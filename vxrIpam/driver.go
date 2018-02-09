package vxrIpam

import (
	"time"

	"github.com/docker/go-plugins-helpers/ipam"

	log "github.com/Sirupsen/logrus"
)

type Driver struct {
	propTime     time.Duration
	respTime     time.Duration
	excludeFirst int
	excludeLast  int
}

func (d *Driver) GetCapabilities() (*ipam.CapabilitiesResponse, error) {
	log.Debugf("vxrIpam.GetCapabilites()")
	return &ipam.CapabilitiesResponse{}, nil
}

func (d *Driver) GetDefaultAddressSpaces() (*ipam.AddressSpacesResponse, error) {
	log.Debugf("vxrIpam.GetDefaultAddressSpaces()")
	return &ipam.AddressSpacesResponse{}, nil
}

func (d *Driver) RequestPool(r *ipam.RequestPoolRequest) (*ipam.RequestPoolResponse, error) {
	log.WithField("r", r).Debugf("vxrIpam.RequestPool()")
	return &ipam.RequestPoolResponse{}, nil
}

func (d *Driver) ReleasePool(r *ipam.ReleasePoolRequest) error {
	log.WithField("r", r).Debugf("vxrIpam.ReleasePoolRequest()")
	return nil
}

func (d *Driver) RequestAddress(r *ipam.RequestAddressRequest) (*ipam.RequestAddressResponse, error) {
	log.WithField("r", r).Debugf("vxrIpam.RequestAddress()")
	return &ipam.RequestAddressResponse{}, nil
}

func (d *Driver) ReleaseAddress(r *ipam.ReleaseAddressRequest) error {
	log.WithField("r", r).Debugf("vxrIpam.ReleaseAddress()")
	return nil
}

func NewDriver(propTime, respTime time.Duration, excludeFirst, excludeLast int) (*Driver, error) {
	d := &Driver{
		propTime,
		respTime,
		excludeFirst,
		excludeLast,
	}
	return d, nil
}
