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

	"github.com/TrilliumIT/vxrouter/host"
)

const (
	//should really find a better place for these
	//rather than duplicating the driver names
	networkDriverName = "vxrNet"
	ipamDriverName    = "vxrIpam"
	envPrefix         = "VXR_"
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
	log.Debug("getNetworkResourceByID")

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
		tnr, err := c.GetNetworkResourceByID(n.ID)
		if err != nil {
			continue
		}
		tp, _ := poolFromNR(tnr) // nolint errcheck
		if tp == pool {
			nr = tnr
			break
		}
	}

	return nr, nil
}

func (c *Core) cacheNetworkResource(nr *types.NetworkResource) {
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

func (c *Core) RequestAddress(addr, poolid string) (*net.IPNet, error) {
	pool := poolFromID(poolid)
	nr, err := c.GetNetworkResourceByPool(pool)
	if err != nil {
		log.WithError(err).WithField("pool", pool).Error("failed to get network resource")
		return nil, err
	}

	gw, err := c.getGateway(nr.ID)
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

	rip, _, _ := net.ParseCIDR(addr) //nolint errcheck
	var ip *net.IPNet
	stop := time.Now().Add(c.respTime)
	for time.Now().Before(stop) {
		ip, err = hi.SelectAddress(rip, c.propTime, xf, xl)
		if err != nil {
			log.WithError(err).Error("failed to select address")
			return nil, err
		}
		if ip != nil {
			break
		}
		if rip != nil {
			time.Sleep(10 * time.Millisecond)
		}
	}

	if ip == nil {
		err = fmt.Errorf("timeout expired while waiting for address")
		log.WithError(err).Error()
		return nil, err
	}

	return ip, nil
}

//loop over the IPAMConfig array, combine gw and sn into a cidr
func (c *Core) getGateway(networkid string) (*net.IPNet, error) {
	nr, err := c.GetNetworkResourceByID(networkid)
	if err != nil {
		log.WithError(err).WithField("NetworkID", networkid).Error("failed to get network resource")
		return nil, err
	}

	for _, ic := range nr.IPAM.Config {
		gws := ic.Gateway
		sns := ic.Subnet
		if gws != "" && sns != "" {
			gw := net.ParseIP(gws)
			if gw == nil {
				err := fmt.Errorf("failed to parse gateway from ipam config")
				return nil, err
			}
			_, sn, err := net.ParseCIDR(sns)
			return &net.IPNet{IP: gw, Mask: sn.Mask}, err
		}
	}

	return nil, fmt.Errorf("no gateway with subnet found in ipam config")
}
