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

type Driver struct {
	scope       string
	client      *client.Client
	log         *log.Entry
	nrCache     map[string]*types.NetworkResource
	nrCacheLock *sync.Mutex
}

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

func (d *Driver) GetCapabilities() (*network.CapabilitiesResponse, error) {
	d.log.Debug("GetCapabilities()")
	cap := &network.CapabilitiesResponse{
		Scope:             d.scope,
		ConnectivityScope: "",
	}
	return cap, nil
}

func (d *Driver) CreateNetwork(r *network.CreateNetworkRequest) error {
	d.log.WithField("r", r).Debug("CreateNetwork()")
	return nil
}

func (d *Driver) AllocateNetwork(r *network.AllocateNetworkRequest) (*network.AllocateNetworkResponse, error) {
	d.log.WithField("r", r).Debug("AllocateNetwork()")
	return &network.AllocateNetworkResponse{}, nil
}

func (d *Driver) DeleteNetwork(r *network.DeleteNetworkRequest) error {
	d.log.WithField("r", r).Debug("DeleteNetwork()")
	return nil
}

func (d *Driver) FreeNetwork(r *network.FreeNetworkRequest) error {
	d.log.WithField("r", r).Debug("FreeNetwork()")
	return nil
}

func (d *Driver) CreateEndpoint(r *network.CreateEndpointRequest) (*network.CreateEndpointResponse, error) {
	d.log.WithField("r", r).Debug("CreateEndpoint()")
	return &network.CreateEndpointResponse{}, nil
}

func (d *Driver) DeleteEndpoint(r *network.DeleteEndpointRequest) error {
	d.log.WithField("r", r).Debug("DeleteEndpoint()")
	nr, err := d.getNetworkResource(r.NetworkID)
	if err != nil {
		d.log.WithError(err).WithField("NetworkID", r.NetworkID).Error("failed to get network resource")
		return err
	}

	hi, err := hostInterface.GetHostInterface(nr.Name)

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

func (d *Driver) EndpointInfo(r *network.InfoRequest) (*network.InfoResponse, error) {
	d.log.WithField("r", r).Debug("EndpointInfo()")
	return &network.InfoResponse{}, nil
}

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

func (d *Driver) Leave(r *network.LeaveRequest) error {
	d.log.WithField("r", r).Debug("Leave()")
	return nil
}

func (d *Driver) DiscoverNew(r *network.DiscoveryNotification) error {
	d.log.WithField("r", r).Debug("DiscoverNew()")
	return nil
}

func (d *Driver) DiscoverDelete(r *network.DiscoveryNotification) error {
	d.log.WithField("r", r).Debug("DiscoverDelete()")
	return nil
}

func (d *Driver) ProgramExternalConnectivity(r *network.ProgramExternalConnectivityRequest) error {
	d.log.WithField("r", r).Debug("ProgramExternalConnectivity()")
	return nil
}

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
	d.nrCacheLock.Lock()
	defer d.nrCacheLock.Unlock()
	var err error
	if nr, ok := d.nrCache[id]; ok {
		return nr, nil
	}

	nr, err := d.client.NetworkInspect(context.Background(), id)
	if err != nil {
		d.log.WithError(err).Error("failed to inspect network %v", id)
		return nil, err
	}

	if nr.Driver != "vxrNet" {
		err := fmt.Errorf("network %v is not a vxrNet", id)
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
			log.WithField("subnet", ipc.Subnet).WithField("to", subnet).Debug("Checking for pool")
			if ipc.Subnet == subnet {
				log.Debug("Returnig subnet")
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
			d.getNetworkResource(nr.ID)
		}
	}
	return nil
}

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

func (d *Driver) GetGatewayBySubnet(subnet string) (*net.IPNet, error) {
	nr, err := d.GetNetworkResourceBySubnet(subnet)
	if err != nil {
		return nil, err
	}
	return gatewayFromIPAMConfigs(nr.IPAM.Config)
}
