package vxrIpam

import (
	"github.com/docker/go-plugins-helpers/ipam"

	log "github.com/Sirupsen/logrus"
)

type vxrIpam struct{}

func (vi *vxrIpam) GetCapabilities() (*ipam.CapabilitiesResponse, error) {
	log.Debugf("vxrIpam.GetCapabilites()")
	return &ipam.CapabilitiesResponse{}, nil
}

func (vi *vxrIpam) GetDefaultAddressSpaces() (*ipam.AddressSpacesResponse, error) {
	log.Debugf("vxrIpam.GetDefaultAddressSpaces()")
	return &ipam.AddressSpacesResponse{}, nil
}

func (vi *vxrIpam) RequestPool(r *ipam.RequestPoolRequest) (*ipam.RequestPoolResponse, error) {
	log.Debugf("vxrIpam.RequestPool()")
	return &ipam.RequestPoolResponse{}, nil
}

func (vi *vxrIpam) ReleasePool(r *ipam.ReleasePoolRequest) error {
	log.Debugf("vxrIpam.ReleasePoolRequest()")
	return nil
}

func (vi *vxrIpam) RequestAddress(r *ipam.RequestAddressRequest) (*ipam.RequestAddressResponse, error) {
	log.Debugf("vxrIpam.RequestAddress()")
	return &ipam.RequestAddressResponse{}, nil
}

func (vi *vxrIpam) ReleaseAddress(r *ipam.ReleaseAddressRequest) error {
	log.Debugf("vxrIpam.ReleaseAddress()")
	return nil
}
