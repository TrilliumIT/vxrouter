package ipam

import (
	"fmt"

	gphipam "github.com/docker/go-plugins-helpers/ipam"
)

const (
	DriverName = "vxrIpam"
)

type Driver struct{}

func NewDriver() (*Driver, error) {
	return &Driver{}, nil
}

func (d *Driver) GetCapabilities() (*gphipam.CapabilitiesResponse, error) {
	return &gphipam.CapabilitiesResponse{}, nil
}

func (d *Driver) GetDefaultAddressSpaces() (*gphipam.AddressSpacesResponse, error) {
	return &gphipam.AddressSpacesResponse{}, nil
}

func (d *Driver) RequestPool(r *gphipam.RequestPoolRequest) (*gphipam.RequestPoolResponse, error) {
	if r.Pool == "" {
		return nil, fmt.Errorf("This driver does not support automatic address pools.")
	}

	rpr := &gphipam.RequestPoolResponse{
		PoolID: DriverName + "_" + r.Pool,
		Pool:   r.Pool,
	}

	return rpr, nil
}

func (d *Driver) ReleasePool(r *gphipam.ReleasePoolRequest) error {
	return nil
}

func (d *Driver) RequestAddress(r *gphipam.RequestAddressRequest) (*gphipam.RequestAddressResponse, error) {
	return &gphipam.RequestAddressResponse{}, nil
}

func (d *Driver) ReleaseAddress(r *gphipam.ReleaseAddressRequest) error {
	return nil
}
