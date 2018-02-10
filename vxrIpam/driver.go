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
	log          *log.Entry
}

func NewDriver(propTime, respTime time.Duration, excludeFirst, excludeLast int) (*Driver, error) {
	d := &Driver{
		propTime,
		respTime,
		excludeFirst,
		excludeLast,
		log.WithField("driver", "vxrIpam"),
	}
	return d, nil
}

func (d *Driver) GetCapabilities() (*ipam.CapabilitiesResponse, error) {
	d.log.Debug("GetCapabilites()")
	return &ipam.CapabilitiesResponse{}, nil
}

func (d *Driver) GetDefaultAddressSpaces() (*ipam.AddressSpacesResponse, error) {
	d.log.Debug("GetDefaultAddressSpaces()")
	return &ipam.AddressSpacesResponse{}, nil
}

func (d *Driver) RequestPool(r *ipam.RequestPoolRequest) (*ipam.RequestPoolResponse, error) {
	d.log.WithField("r", r).Debug("RequestPool()")
	return &ipam.RequestPoolResponse{}, nil
}

func (d *Driver) ReleasePool(r *ipam.ReleasePoolRequest) error {
	d.log.WithField("r", r).Debug("ReleasePoolRequest()")
	return nil
}

func (d *Driver) RequestAddress(r *ipam.RequestAddressRequest) (*ipam.RequestAddressResponse, error) {
	d.log.WithField("r", r).Debug("RequestAddress()")
	return &ipam.RequestAddressResponse{}, nil
}

func (d *Driver) ReleaseAddress(r *ipam.ReleaseAddressRequest) error {
	d.log.WithField("r", r).Debug("ReleaseAddress()")
	return nil
}
