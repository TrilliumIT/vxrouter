package vxrNet

import (
	"github.com/docker/go-plugins-helpers/network"

	log "github.com/Sirupsen/logrus"
)

type vxrNet struct{}

func (vi *vxrNet) GetCapabilities() (*network.CapabilitiesResponse, error) {
	log.Debugf("vxrNet.GetCapabilites()")
	return &network.CapabilitiesResponse{}, nil
}

func (vi *vxrNet) CreateNetwork(r *network.CreateNetworkRequest) error {
	log.Debugf("vxrNet.CreateNetwork()")
	return nil
}

func (vi *vxrNet) AllocateNetwork(r *network.AllocateNetworkRequest) (*network.AllocateNetworkResponse, error) {
	log.Debugf("vxrNet.AllocateNetwork()")
	return &network.AllocateNetworkResponse{}, nil
}

func (vi *vxrNet) DeleteNetwork(r *network.DeleteNetworkRequest) error {
	log.Debugf("vxrNet.DeleteNetwork()")
	return nil
}

func (vi *vxrNet) FreeNetwork(r *network.FreeNetworkRequest) error {
	log.Debugf("vxrNet.FreeNetwork()")
	return nil
}

func (vi *vxrNet) CreateEndpoint(r *network.CreateEndpointRequest) (*network.CreateEndpointResponse, error) {
	log.Debugf("vxrNet.ReleaseAddress()")
	return &network.CreateEndpointResponse{}, nil
}

func (vi *vxrNet) DeleteEndpoint(r *network.DeleteEndpointRequest) error {
	log.Debugf("vxrNet.DeleteEndpoint()")
	return nil
}

func (vi *vxrNet) EndpointInfo(r *network.InfoRequest) (*network.InfoResponse, error) {
	log.Debugf("vxrNet.EndpointInfo()")
	return &network.InfoResponse{}, nil
}

func (vi *vxrNet) Join(r *network.JoinRequest) (*network.JoinResponse, error) {
	log.Debugf("vxrNet.Join()")
	return &network.JoinResponse{}, nil
}

func (vi *vxrNet) Leave(r *network.LeaveRequest) error {
	log.Debugf("vxrNet.Leave()")
	return nil
}

func (vi *vxrNet) DiscoverNew(r *network.DiscoveryNotification) error {
	log.Debugf("vxrNet.DiscoverNew()")
	return nil
}

func (vi *vxrNet) DiscoverDelete(r *network.DiscoveryNotification) error {
	log.Debugf("vxrNet.DiscoverDelete()")
	return nil
}

func (vi *vxrNet) ProgramExternalConnectivity(r *network.ProgramExternalConnectivityRequest) error {
	log.Debugf("vxrNet.ProgramExternalConnectivity()")
	return nil
}

func (vi *vxrNet) RevokeExternalConnectivity(r *network.RevokeExternalConnectivityRequest) error {
	log.Debugf("vxrNet.RevokeExternalConnectivity()")
	return nil
}
