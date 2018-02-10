package vxrNet

import (
	"fmt"
	"net"

	log "github.com/Sirupsen/logrus"
	apinet "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-plugins-helpers/network"
	"golang.org/x/net/context"

	"github.com/TrilliumIT/docker-vxrouter/hostInterface"
)

type Driver struct {
	scope  string
	client *client.Client
}

func (d *Driver) GetCapabilities() (*network.CapabilitiesResponse, error) {
	log.Debugf("vxrNet.GetCapabilites()")
	cap := &network.CapabilitiesResponse{
		Scope:             d.scope,
		ConnectivityScope: "",
	}
	return cap, nil
}

func (d *Driver) CreateNetwork(r *network.CreateNetworkRequest) error {
	log.WithField("r", r).Debugf("vxrNet.CreateNetwork()")
	return nil
}

func (d *Driver) AllocateNetwork(r *network.AllocateNetworkRequest) (*network.AllocateNetworkResponse, error) {
	log.WithField("r", r).Debugf("vxrNet.AllocateNetwork()")
	return &network.AllocateNetworkResponse{}, nil
}

func (d *Driver) DeleteNetwork(r *network.DeleteNetworkRequest) error {
	log.WithField("r", r).Debugf("vxrNet.DeleteNetwork()")
	return nil
}

func (d *Driver) FreeNetwork(r *network.FreeNetworkRequest) error {
	log.WithField("r", r).Debugf("vxrNet.FreeNetwork()")
	return nil
}

func (d *Driver) CreateEndpoint(r *network.CreateEndpointRequest) (*network.CreateEndpointResponse, error) {
	log.WithField("r", r).Debugf("vxrNet.CreateEndpoint()")
	return &network.CreateEndpointResponse{}, nil
}

func (d *Driver) DeleteEndpoint(r *network.DeleteEndpointRequest) error {
	log.WithField("r", r).Debugf("vxrNet.DeleteEndpoint()")
	return nil
}

func (d *Driver) EndpointInfo(r *network.InfoRequest) (*network.InfoResponse, error) {
	log.WithField("r", r).Debugf("vxrNet.EndpointInfo()")
	return &network.InfoResponse{}, nil
}

func (d *Driver) Join(r *network.JoinRequest) (*network.JoinResponse, error) {
	log.WithField("r", r).Debugf("vxrNet.Join()")
	nr, err := d.client.NetworkInspect(context.Background(), r.NetworkID)
	if err != nil {
		log.WithError(err).Errorf("failed to inspect network %v", r.NetworkID)
		return nil, err
	}

	if nr.Driver != "vxrNet" {
		err := fmt.Errorf("network %v is not a vxrNet", r.NetworkID)
		return nil, err
	}

	gw, err := gatewayFromIPAMConfigs(nr.IPAM.Config)
	if err != nil {
		log.WithError(err).Errorf("failed to get gateway cidr from ipam config")
		return nil, err
	}

	hi, err := hostInterface.GetOrCreateHostInterface(nr.Name, gw, nr.Options)
	if err != nil {
		log.WithError(err).Errorf("failed to create HostInterface")
		return nil, err
	}

	mvlName := "cmvl_" + r.EndpointID[:7]
	err = hi.CreateMacvlan(mvlName)
	if err != nil {
		log.WithError(err).Errorf("failed to create macvlan for container")
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

func (d *Driver) Leave(r *network.LeaveRequest) error {
	log.WithField("r", r).Debugf("vxrNet.Leave()")

	//TODO: get hi

	//mvlName := "cmvl_" + r.EndpointID[:7]
	//return hi.DeleteMacvlan(name)

	return nil
}

func (d *Driver) DiscoverNew(r *network.DiscoveryNotification) error {
	log.WithField("r", r).Debugf("vxrNet.DiscoverNew()")
	return nil
}

func (d *Driver) DiscoverDelete(r *network.DiscoveryNotification) error {
	log.WithField("r", r).Debugf("vxrNet.DiscoverDelete()")
	return nil
}

func (d *Driver) ProgramExternalConnectivity(r *network.ProgramExternalConnectivityRequest) error {
	log.WithField("r", r).Debugf("vxrNet.ProgramExternalConnectivity()")
	return nil
}

func (d *Driver) RevokeExternalConnectivity(r *network.RevokeExternalConnectivityRequest) error {
	log.WithField("r", r).Debugf("vxrNet.RevokeExternalConnectivity()")
	return nil
}

func NewDriver(scope string, client *client.Client) (*Driver, error) {
	d := &Driver{
		scope,
		client,
	}
	return d, nil
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
