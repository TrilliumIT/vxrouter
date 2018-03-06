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

// Core is a wrapper for docker client type things
type Core struct {
	dc       *client.Client
	propTime time.Duration
	respTime time.Duration
	getNr    chan *getNr
	delNr    chan string
	putNr    chan *types.NetworkResource
}

// New creates a new client
func New(propTime, respTime time.Duration) (*Core, error) {
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

// getNetworkResourceByID gets a network resource by ID (checks cache first)
func (c *Core) getNetworkResourceByID(id string) (*types.NetworkResource, error) {
	log := log.WithField("net_id", id)
	log.Debug("GetNetworkResourceByID()")

	nr := c.getNrFromCache(id)
	if nr != nil {
		return nr, nil
	}

	//netid wasn't in cache, fetch from docker inspect
	ctx, cancel := context.WithTimeout(context.Background(), dockerTimeout)
	defer cancel()
	nnr, err := c.dc.NetworkInspect(ctx, id)
	if err != nil {
		log.WithError(err).Error("failed to inspect network")
		return nil, err
	}
	nr = &nnr

	c.putNrInCache(nr)

	return nr, nil
}

// getNetworkResourceByPool gets a network resource by it's subnet
func (c *Core) getNetworkResourceByPool(pool string) (*types.NetworkResource, error) {
	log := log.WithField("pool", pool)
	log.Debug("getNetworkResourceByPool")

	nr := c.getNrFromCache(pool)
	if nr != nil {
		return nr, nil
	}

	flts := filters.NewArgs()
	flts.Add("driver", networkDriverName)
	ctx, cancel := context.WithTimeout(context.Background(), dockerTimeout)
	defer cancel()
	nl, err := c.dc.NetworkList(ctx, types.NetworkListOptions{Filters: flts})
	if err != nil {
		log.WithError(err).Error("failed to list networks")
		return nil, err
	}

	for _, n := range nl {
		nr, err = c.getNetworkResourceByID(n.ID)
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
	c.delNrInCache(pool)
}

func (c *Core) connectIfNotConnected(addr, nrID string) error {
	ip := net.ParseIP(addr)
	numRoutes, err := host.VxroutesTo(ip)
	if err != nil {
		return err
	}
	if numRoutes > 0 {
		return nil
	}
	nr, err := c.getNetworkResourceByID(nrID)
	if err != nil {
		return err
	}
	_, err = c.connectAndGetAddress(ip, nr)
	return err
}

// ConnectAndGetAddress connects the host to the network for the
// passed in pool, and returns either an available random or the
// requested address if it's available
func (c *Core) ConnectAndGetAddress(addr, poolid string) (*net.IPNet, error) {
	log := log.WithField("addr", addr)
	log = log.WithField("poolid", poolid)
	log.Debug("ConnectAndGetAddress()")

	pool := poolFromID(poolid)
	nr, err := c.getNetworkResourceByPool(pool)
	if err != nil {
		log.WithError(err).Error("failed to get network resource")
		return nil, err
	}

	ip := net.ParseIP(addr)

	return c.connectAndGetAddress(ip, nr)
}

func (c *Core) connectAndGetAddress(addr net.IP, nr *types.NetworkResource) (*net.IPNet, error) {
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

	return hi.SelectAddress(addr, c.propTime, c.respTime, xf, xl)
}

// GetGatewayByNetID loops over the IPAMConfig array, combine gw and sn into a cidr
func (c *Core) GetGatewayByNetID(netid string) (*net.IPNet, error) {
	log := log.WithField("netid", netid)
	log.Debug("GetGatewayByNetID()")

	nr, err := c.getNetworkResourceByID(netid)
	if err != nil {
		log.WithError(err).WithField("NetworkID", netid).Error("failed to get network resource")
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

	nr, err := c.getNetworkResourceByID(netid)
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

	nr, err := c.getNetworkResourceByID(netid)
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

// DeleteRoute deletes a route... who'd have thought?
func (c *Core) DeleteRoute(address string) error {
	return c.deleteRoute(net.ParseIP(address))
}

func (c *Core) deleteRoute(addr net.IP) error {
	hi, err := host.GetInterfaceFromDestinationAddress(addr)
	if err != nil {
		return err
	}

	err = hi.DelRoute(addr)
	if err != nil {
		return err
	}

	go func() {
		if err = hi.Delete(); err != nil {
			log.WithError(err).Error("error while deleting host interface")
		}
	}()

	return nil
}
