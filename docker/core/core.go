package core

import (
	"fmt"
	"net"
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
	dockerTimeout     = 5 * time.Second
)

func toCtx() context.Context {
	c, _ := context.WithTimeout(context.Background(), dockerTimeout)
	return c
}

// Core is a wrapper for docker client type things
type Core struct {
	dc       *client.Client
	propTime time.Duration
	respTime time.Duration
	getNr    chan *getNr
	delNr    chan string
	putNr    chan *types.NetworkResource
}

type getNr struct {
	s  string
	rc chan<- *types.NetworkResource
}

// NewCore creates a new client
func NewCore(propTime, respTime time.Duration) (*Core, error) {
	dc, err := client.NewEnvClient()
	if err != nil {
		return nil, err
	}

	c := &Core{
		dc:       dc,
		propTime: propTime,
		respTime: respTime,
		getNr:    make(chan *getNr),
		delNr:    make(chan string),
		putNr:    make(chan *types.NetworkResource),
	}

	go nrCacheLoop(c.getNr, c.delNr, c.putNr)
	return c, nil
}

func nrCacheLoop(getNr <-chan *getNr, delNr <-chan string, putNr <-chan *types.NetworkResource) {
	nrCache := make(map[string]*types.NetworkResource)
	for {
		select {
		case rc := <-getNr:
			rc.rc <- nrCache[rc.s]
		case dn := <-delNr:
			nr := nrCache[dn]
			if nr == nil {
				break
			}
			delete(nrCache, nr.ID)
			pool, err := poolFromNR(nr)
			if err != nil {
				log.Debug("failed to get pool from network resource, not deleting")
				break
			}
			delete(nrCache, pool)
		case nr := <-putNr:
			nrCache[nr.ID] = nr
			pool, err := poolFromNR(nr)
			if err != nil {
				log.Debug("failed to get pool from network resource, not caching")
				break
			}
			nrCache[pool] = nr
		}
	}
}

func (c *Core) getNrFromCache(s string) *types.NetworkResource {
	rc := make(chan *types.NetworkResource)
	c.getNr <- &getNr{s, rc}
	return <-rc
}

// GetContainers gets a list of docker containers
func (c *Core) GetContainers() ([]types.Container, error) {
	return c.dc.ContainerList(toCtx(), types.ContainerListOptions{})
}

// GetNetworkResourceByID gets a network resource by ID (checks cache first)
func (c *Core) GetNetworkResourceByID(id string) (*types.NetworkResource, error) {
	log := log.WithField("net_id", id)
	log.Debug("GetNetworkResourceByID()")

	nr := c.getNrFromCache(id)
	if nr != nil {
		return nr, nil
	}

	//netid wasn't in cache, fetch from docker inspect
	nnr, err := c.dc.NetworkInspect(toCtx(), id)
	if err != nil {
		log.WithError(err).Error("failed to inspect network")
		return nil, err
	}
	nr = &nnr

	c.putNr <- nr

	return nr, nil
}

// GetNetworkResourceByPool gets a network resource by it's subnet
func (c *Core) GetNetworkResourceByPool(pool string) (*types.NetworkResource, error) {
	log := log.WithField("pool", pool)
	log.Debug("getNetworkResourceByPool")

	nr := c.getNrFromCache(pool)
	if nr != nil {
		return nr, nil
	}

	flts := filters.NewArgs()
	flts.Add("driver", networkDriverName)
	nl, err := c.dc.NetworkList(toCtx(), types.NetworkListOptions{Filters: flts})
	if err != nil {
		log.WithError(err).Error("failed to list networks")
		return nil, err
	}

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
	pool := poolFromID(poolid)
	c.delNr <- pool
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

	gw, err := GatewayFromNR(nr)
	if err != nil {
		log.WithError(err).Error("failed to get gateway")
		return nil, err
	}

	//exclude network and (normal) broadcast addresses by default
	xf := vxrouter.GetEnvIntWithDefault(envPrefix+"excludefirst", nr.Options["excludefirst"], 1)
	xl := vxrouter.GetEnvIntWithDefault(envPrefix+"excludelast", nr.Options["excludelast"], 1)

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
	return GatewayFromNR(nr)
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
	return GatewayFromNR(nr)
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

	gw, err := GatewayFromNR(nr)
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
