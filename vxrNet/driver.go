package vxrNet

import (
	"fmt"
	"net"
	"sync"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types"
	apinet "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-plugins-helpers/network"
	"golang.org/x/net/context"

	"github.com/TrilliumIT/docker-vxrouter/hostInterface"
)

// Driver is a vxrouter network driver
type Driver struct {
	scope       string
	client      *client.Client
	log         *log.Entry
	nrCache     map[string]*types.NetworkResource
	nrCacheLock *sync.Mutex
}

// NewDriver creates a new Driver
func NewDriver(scope string, client *client.Client) (*Driver, error) {
	d := &Driver{
		scope,
		client,
		log.WithField("driver", "vxrNet"),
		make(map[string]*types.NetworkResource),
		&sync.Mutex{},
	}
	return d, nil
}

// GetCapabilities is called on driver initialization
func (d *Driver) GetCapabilities() (*network.CapabilitiesResponse, error) {
	d.log.Debug("GetCapabilities()")
	cap := &network.CapabilitiesResponse{
		Scope:             d.scope,
		ConnectivityScope: "",
	}
	return cap, nil
}

// CreateNetwork is called on docker network create
func (d *Driver) CreateNetwork(r *network.CreateNetworkRequest) error {
	d.log.WithField("r", r).Debug("CreateNetwork()")
	return nil
}

// AllocateNetwork is never called
func (d *Driver) AllocateNetwork(r *network.AllocateNetworkRequest) (*network.AllocateNetworkResponse, error) {
	d.log.WithField("r", r).Debug("AllocateNetwork()")
	return &network.AllocateNetworkResponse{}, nil
}

// DeleteNetwork is called on docker network rm
func (d *Driver) DeleteNetwork(r *network.DeleteNetworkRequest) error {
	d.log.WithField("r", r).Debug("DeleteNetwork()")
	return nil
}

// FreeNetwork is never called
func (d *Driver) FreeNetwork(r *network.FreeNetworkRequest) error {
	d.log.WithField("r", r).Debug("FreeNetwork()")
	return nil
}

// CreateEndpoint is called after IPAM has assigned an address, before Join is called
func (d *Driver) CreateEndpoint(r *network.CreateEndpointRequest) (*network.CreateEndpointResponse, error) {
	d.log.WithField("r", r).Debug("CreateEndpoint()")
	return &network.CreateEndpointResponse{}, nil
}

// DeleteEndpoint is called after Leave, before IPAM release address
func (d *Driver) DeleteEndpoint(r *network.DeleteEndpointRequest) error {
	d.log.WithField("r", r).Debug("DeleteEndpoint()")
	nr, err := d.getNetworkResource(r.NetworkID)
	if err != nil {
		d.log.WithError(err).WithField("NetworkID", r.NetworkID).Error("failed to get network resource")
		return err
	}

	hi, err := hostInterface.GetHostInterface(nr.Name)
	if err != nil {
		return err
	}

	mvlName := "cmvl_" + r.EndpointID[:7]
	err = hi.DeleteMacvlan(mvlName)
	if err != nil {
		d.log.WithError(err).Error("failed to delete macvlan for container")
		return err
	}

	containers, err := d.client.ContainerList(context.Background(), types.ContainerListOptions{})
	if err != nil {
		d.log.WithError(err).Error("failed to list containers")
		return err
	}

	for _, c := range containers {
		if ns, ok := c.NetworkSettings.Networks[nr.Name]; ok {
			if ns.EndpointID != r.EndpointID {
				d.log.Debug("other containers are still running on this network")
				return nil
			}
		}
	}

	return hi.Delete()
}

// EndpointInfo is called on inspect... maybe?
func (d *Driver) EndpointInfo(r *network.InfoRequest) (*network.InfoResponse, error) {
	d.log.WithField("r", r).Debug("EndpointInfo()")
	return &network.InfoResponse{}, nil
}

// Join is the last thing called before the nic is put into the container namespace
func (d *Driver) Join(r *network.JoinRequest) (*network.JoinResponse, error) {
	hi, err := d.ConnectHost(r.NetworkID)
	if err != nil {
		return nil, err
	}

	mvlName := "cmvl_" + r.EndpointID[:7]
	err = hi.CreateMacvlan(mvlName)
	if err != nil {
		d.log.WithError(err).Error("failed to create macvlan for container")
		return nil, err
	}

	jr := &network.JoinResponse{
		InterfaceName: network.InterfaceName{
			SrcName:   mvlName,
			DstPrefix: "eth",
		},
	}

	return jr, nil
}

// ConnectHost is not used by docker, it is called by IPAM because IPAM needs to make sure that the host is connected before routes can be added to allocate an address
func (d *Driver) ConnectHost(id string) (*hostInterface.HostInterface, error) {
	log := d.log.WithField("id", id)
	log.Debug("ConnectHost()")

	nr, err := d.getNetworkResource(id)
	if err != nil {
		log.WithError(err).Error("failed to get network resource")
		return nil, err
	}

	gw, err := gatewayFromIPAMConfigs(nr.IPAM.Config)
	if err != nil {
		d.log.WithError(err).Error("failed to get gateway cidr from ipam config")
		return nil, err
	}

	return hostInterface.GetOrCreateHostInterface(nr.Name, gw, nr.Options)
}

// Leave is the first thing called on container stop
func (d *Driver) Leave(r *network.LeaveRequest) error {
	d.log.WithField("r", r).Debug("Leave()")
	return nil
}

// DiscoverNew is not implemented by this driver
func (d *Driver) DiscoverNew(r *network.DiscoveryNotification) error {
	d.log.WithField("r", r).Debug("DiscoverNew()")
	return nil
}

// DiscoverDelete is not implemented by this driver
func (d *Driver) DiscoverDelete(r *network.DiscoveryNotification) error {
	d.log.WithField("r", r).Debug("DiscoverDelete()")
	return nil
}

// ProgramExternalConnectivity is not implemented by this driver
func (d *Driver) ProgramExternalConnectivity(r *network.ProgramExternalConnectivityRequest) error {
	d.log.WithField("r", r).Debug("ProgramExternalConnectivity()")
	return nil
}

// RevokeExternalConnectivity is not implemented by this driver
func (d *Driver) RevokeExternalConnectivity(r *network.RevokeExternalConnectivityRequest) error {
	d.log.WithField("r", r).Debug("RevokeExternalConnectivity()")
	return nil
}

//loop over the IPAMConfig array, combine gw and sn into a cidr
func gatewayFromIPAMConfigs(ics []apinet.IPAMConfig) (*net.IPNet, error) {
	for _, ic := range ics {
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

func (d *Driver) getNetworkResource(id string) (*types.NetworkResource, error) {
	log := d.log.WithField("net_id", id)
	d.nrCacheLock.Lock()
	defer d.nrCacheLock.Unlock()
	var err error
	if nr, ok := d.nrCache[id]; ok {
		return nr, nil
	}

	nr, err := d.client.NetworkInspect(context.Background(), id)
	if err != nil {
		log.WithError(err).Error("failed to inspect network")
		return nil, err
	}

	if nr.Driver != "vxrNet" {
		err := fmt.Errorf("network is not a vxrNet")
		return nil, err
	}

	d.nrCache[id] = &nr

	return &nr, nil
}

func (d *Driver) getNetworkResourceBySubnetFromCache(subnet string) *types.NetworkResource {
	d.nrCacheLock.Lock()
	defer d.nrCacheLock.Unlock()
	for _, nr := range d.nrCache {
		for _, ipc := range nr.IPAM.Config {
			if ipc.Subnet == subnet {
				return nr
			}
		}
	}
	return nil
}

func (d *Driver) cacheAllNetworkResources() error {
	nets, err := d.client.NetworkList(context.Background(), types.NetworkListOptions{})
	if err != nil {
		return err
	}
	for _, nr := range nets {
		if nr.Driver == "vxrNet" {
			if _, err := d.getNetworkResource(nr.ID); err != nil {
				// we don't want to bail on errors here, we're just trying to cache everything we can
				d.log.WithField("net_id", nr.ID).WithError(err).Error("error caching network resource")
			}

		}
	}
	return nil
}

// GetNetworkResourceBySubnet is used by the IPAM driver to find the correct network resource for a given pool
func (d *Driver) GetNetworkResourceBySubnet(subnet string) (*types.NetworkResource, error) {
	nr := d.getNetworkResourceBySubnetFromCache(subnet)
	if nr != nil {
		return nr, nil
	}
	err := d.cacheAllNetworkResources()
	if err != nil {
		return nil, err
	}
	return d.getNetworkResourceBySubnetFromCache(subnet), nil
}

// GetGatewayBySubnet is used by the IPAM driver to find the correct gateway address for a given pool
func (d *Driver) GetGatewayBySubnet(subnet string) (*net.IPNet, error) {
	nr, err := d.GetNetworkResourceBySubnet(subnet)
	if err != nil {
		return nil, err
	}
	return gatewayFromIPAMConfigs(nr.IPAM.Config)
}
