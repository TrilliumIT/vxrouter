package core

import (
	"fmt"
	"net"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"golang.org/x/net/context"

	"github.com/TrilliumIT/vxrouter"
	"github.com/TrilliumIT/vxrouter/host"
)

const (
	networkDriverName = vxrouter.NetworkDriver
	ipamDriverName    = vxrouter.IpamDriver
	envPrefix         = vxrouter.EnvPrefix
)

// Core is a wrapper for docker client type things
type Core struct {
	dc          *client.Client
	propTime    time.Duration
	respTime    time.Duration
	nrByID      map[string]*types.NetworkResource
	nrByPool    map[string]*types.NetworkResource
	nrCacheLock *sync.RWMutex
}

// NewCore creates a new client
func NewCore(propTime, respTime time.Duration) (*Core, error) {
	dc, err := client.NewEnvClient()
	if err != nil {
		return nil, err
	}

	c := &Core{
		dc:          dc,
		propTime:    propTime,
		respTime:    respTime,
		nrByID:      make(map[string]*types.NetworkResource),
		nrByPool:    make(map[string]*types.NetworkResource),
		nrCacheLock: &sync.RWMutex{},
	}

	return c, nil
}

// GetContainers gets a list of docker containers
func (c *Core) GetContainers() ([]types.Container, error) {
	return c.dc.ContainerList(context.Background(), types.ContainerListOptions{})
}

// GetNetworkResourceByID gets a network resource by ID (checks cache first)
func (c *Core) GetNetworkResourceByID(id string) (*types.NetworkResource, error) {
	log := log.WithField("net_id", id)
	log.Debug("GetNetworkResourceByID()")

	//first check local cache with a read-only mutex
	c.nrCacheLock.RLock()

	if nr, ok := c.nrByID[id]; ok {
		c.nrCacheLock.RUnlock()
		return nr, nil
	}
	c.nrCacheLock.RUnlock()

	//netid wasn't in cache, fetch from docker inspect
	nr, err := c.dc.NetworkInspect(context.Background(), id)
	if err != nil {
		log.WithError(err).Error("failed to inspect network")
		return nil, err
	}

	//add nr pointer to both caches
	c.cacheNetworkResource(&nr)

	return &nr, nil
}

// GetNetworkResourceByPool gets a network resource by it's subnet
func (c *Core) GetNetworkResourceByPool(pool string) (*types.NetworkResource, error) {
	log := log.WithField("pool", pool)
	log.Debug("getNetworkResourceByPool")

	//not sure of the performance implications of sharing a read lock between
	//both caches, but we want them in lock step anyway, so likely a non-issue
	c.nrCacheLock.RLock()

	if nr, ok := c.nrByPool[pool]; ok {
		c.nrCacheLock.RUnlock()
		return nr, nil
	}
	c.nrCacheLock.RUnlock()

	flts := filters.NewArgs()
	flts.Add("driver", networkDriverName)
	nl, err := c.dc.NetworkList(context.Background(), types.NetworkListOptions{Filters: flts})
	if err != nil {
		log.WithError(err).Error("failed to list networks")
		return nil, err
	}

	var nr *types.NetworkResource
	for _, n := range nl {
		nr, err = c.GetNetworkResourceByID(n.ID)
		if err != nil {
			continue
		}
		tp, _ := poolFromNR(nr) // nolint: errcheck
		if tp == pool {
			return nr, nil
		}
	}

	return nil, fmt.Errorf("network resource not found")
}

// Uncache uncaches the network resources
func (c *Core) Uncache(poolid string) {
	c.nrCacheLock.Lock()
	defer c.nrCacheLock.Unlock()
	if _, ok := c.nrByPool[poolid]; !ok {
		return
	}
	delete(c.nrByID, c.nrByPool[poolid].ID)
	delete(c.nrByPool, poolid)
}

func (c *Core) cacheNetworkResource(nr *types.NetworkResource) {
	log := log.WithField("nr.Name", nr.Name)
	log.Debug("cacheNetworkResource()")

	c.nrCacheLock.Lock()
	defer c.nrCacheLock.Unlock()

	pool, err := poolFromNR(nr)
	if err != nil {
		log.Debug("failed to get pool from network resource, not caching")
		return
	}

	c.nrByID[nr.ID] = nr
	c.nrByPool[pool] = nr
}

// ConnectAndGetAddress connects the host to the network for the
// passed in pool, and returns either an available random or the
// requested address if it's available
func (c *Core) ConnectAndGetAddress(addr, poolid string) (*net.IPNet, error) {
	log := log.WithField("addr", addr)
	log = log.WithField("poolid", poolid)
	log.Debug("ConnectAndGetAddress()")

	pool := poolFromID(poolid)
	nr, err := c.GetNetworkResourceByPool(pool)
	if err != nil {
		log.WithError(err).Error("failed to get network resource")
		return nil, err
	}

	gw, err := c.GetGatewayByNetID(nr.ID)
	if err != nil {
		log.WithError(err).Error("failed to get gateway")
		return nil, err
	}

	//exclude network and (normal) broadcast addresses by default
	xf := getEnvIntWithDefault(envPrefix+"excludefirst", nr.Options["excludefirst"], 1)
	xl := getEnvIntWithDefault(envPrefix+"excludelast", nr.Options["excludelast"], 1)

	hi, err := host.GetOrCreateInterface(nr.Name, gw, nr.Options)
	if err != nil {
		log.WithError(err).Error("failed to get or create host interface")
		return nil, err
	}

	rip := net.ParseIP(addr)
	return hi.SelectAddress(rip, c.propTime, c.respTime, xf, xl)
}

// GetGatewayByNetID loops over the IPAMConfig array, combine gw and sn into a cidr
func (c *Core) GetGatewayByNetID(netid string) (*net.IPNet, error) {
	log := log.WithField("netid", netid)
	log.Debug("GetGatewayByNetID()")

	nr, err := c.GetNetworkResourceByID(netid)
	if err != nil {
		log.WithError(err).WithField("NetworkID", netid).Error("failed to get network resource")
		return nil, err
	}

	for _, ic := range nr.IPAM.Config {
		gws := ic.Gateway
		sns := ic.Subnet
		if gws != "" && sns != "" {
			gw := net.ParseIP(gws)
			if gw == nil {
				err = fmt.Errorf("failed to parse gateway from ipam config")
				return nil, err
			}
			var sn *net.IPNet
			_, sn, err = net.ParseCIDR(sns)
			return &net.IPNet{IP: gw, Mask: sn.Mask}, err
		}
	}

	return nil, fmt.Errorf("no gateway with subnet found in ipam config")
}

// GetGatewayByPoolID return the gateway by the poolid (I don't think I'm calling this anymore)
func (c *Core) GetGatewayByPoolID(poolid string) (*net.IPNet, error) {
	log := log.WithField("poolid", poolid)
	log.Debug("GetGatewayByNetID()")

	pool := poolFromID(poolid)
	nr, err := c.GetNetworkResourceByPool(pool)
	if err != nil {
		return nil, err
	}
	return c.GetGatewayByNetID(nr.ID)
}

// CreateContainerInterface creates the macvlan to be put into a container namespace
// returns the name of the interface
func (c *Core) CreateContainerInterface(netid, endpointid string) (string, error) {
	log := log.WithField("netid", netid)
	log = log.WithField("endpointid", endpointid)
	log.Debug("CreateContainerInterface()")

	nr, err := c.GetNetworkResourceByID(netid)
	if err != nil {
		log.WithError(err).WithField("netid", netid).Error("failed to get network resource")
		return "", err
	}

	gw, err := c.GetGatewayByNetID(nr.ID)
	if err != nil {
		log.WithError(err).Error("failed to get gateway")
		return "", err
	}

	hi, err := host.GetOrCreateInterface(nr.Name, gw, nr.Options)
	if err != nil {
		return "", err
	}

	mvlName := "cmvl_" + endpointid[:7]
	err = hi.CreateMacvlan(mvlName)
	if err != nil {
		log.WithError(err).Error("failed to create macvlan for container")
		return "", err
	}

	return mvlName, nil
}

// DeleteContainerInterface deletes the container interface, removes the route
// and deletes the host interface if it's the last container
func (c *Core) DeleteContainerInterface(netid, endpointid string) error {
	log := log.WithField("netid", netid)
	log = log.WithField("endpointid", endpointid)
	log.Debug("CreateContainerInterface()")

	nr, err := c.GetNetworkResourceByID(netid)
	if err != nil {
		log.WithError(err).WithField("NetworkID", netid).Error("failed to get network resource")
		return err
	}

	hi, err := host.GetInterface(nr.Name)
	if err != nil {
		return err
	}

	mvlName := "cmvl_" + endpointid[:7]
	err = hi.DeleteMacvlan(mvlName)
	if err != nil {
		log.WithError(err).Error("failed to delete macvlan for container")
		return err
	}

	return nil
}

// CheckAndDeleteInterface checks the host interface for running containers, and if non, deletes it
func (c *Core) CheckAndDeleteInterface(hi *host.Interface, netName, address string) {
	hi.Lock()
	defer hi.Unlock()

	containers, err := c.GetContainers()
	if err != nil {
		log.WithError(err).Error("failed to list containers")
		return
	}

	for _, c := range containers {
		ns, ok := c.NetworkSettings.Networks[netName]
		if !ok {
			continue
		}

		if ns.IPAddress != address {
			log.Debug("other containers are still running on this network")
			return
		}
	}

	if err = hi.Delete(); err != nil {
		log.WithError(err).Error("failed to delete host interface")
	}
}

// DeleteRoute deletes a route... who'd have thought?
func (c *Core) DeleteRoute(address, poolid string) error {
	pool := poolFromID(poolid)
	nr, err := c.GetNetworkResourceByPool(pool)
	if err != nil {
		return err
	}

	hi, err := host.GetInterface(nr.Name)
	if err != nil {
		return err
	}

	err = hi.DelRoute(net.ParseIP(address))
	if err != nil {
		return err
	}

	go c.CheckAndDeleteInterface(hi, nr.Name, address)

	return nil
}
