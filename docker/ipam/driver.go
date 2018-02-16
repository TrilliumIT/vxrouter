package ipam

import (
	"fmt"

	gphipam "github.com/docker/go-plugins-helpers/ipam"
)

const (
	// DriverName is the name of the driver
	DriverName = "vxrIpam"
)

// Driver is the driver ipam type
type Driver struct{}

// NewDriver creates new ipam driver
func NewDriver() (*Driver, error) {
	return &Driver{}, nil
}

// GetCapabilities does nothing
func (d *Driver) GetCapabilities() (*gphipam.CapabilitiesResponse, error) {
	return &gphipam.CapabilitiesResponse{}, nil
}

// GetDefaultAddressSpaces does nothing
func (d *Driver) GetDefaultAddressSpaces() (*gphipam.AddressSpacesResponse, error) {
	return &gphipam.AddressSpacesResponse{}, nil
}

// RequestPool reflects the pool back to the caller
func (d *Driver) RequestPool(r *gphipam.RequestPoolRequest) (*gphipam.RequestPoolResponse, error) {
	if r.Pool == "" {
		return nil, fmt.Errorf("this driver does not support automatic address pools")
	}

	rpr := &gphipam.RequestPoolResponse{
		PoolID: DriverName + "_" + r.Pool,
		Pool:   r.Pool,
	}

	return rpr, nil
}

// ReleasePool does nothing
func (d *Driver) ReleasePool(r *gphipam.ReleasePoolRequest) error {
	return nil
}

// RequestAddress does nothing
func (d *Driver) RequestAddress(r *gphipam.RequestAddressRequest) (*gphipam.RequestAddressResponse, error) {
	return &gphipam.RequestAddressResponse{}, nil
}

// ReleaseAddress does nothing
func (d *Driver) ReleaseAddress(r *gphipam.ReleaseAddressRequest) error {
	return nil
}
