package ipam

import (
	"fmt"
	"net"
	"strings"

	log "github.com/Sirupsen/logrus"
	gphipam "github.com/docker/go-plugins-helpers/ipam"

	"github.com/TrilliumIT/vxrouter/docker/core"
)

const (
	// DriverName is the name of the driver
	DriverName = "vxrIpam"
)

// Driver is the driver ipam type
type Driver struct {
	core *core.Core
}

// NewDriver creates new ipam driver
func NewDriver(core *core.Core) (*Driver, error) {
	return &Driver{core}, nil
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
	rar := &gphipam.RequestAddressResponse{}
	if r.Address != "" {
		_, sn, err := net.ParseCIDR()
		if err != nil {
			return nil, fmt.Errorf("failed to parse subnet")
		}
		gwn := &net.IPNet{IP: net.ParseIP(r.Address), Mask: sn.Mask}
		rar.Address = gwn.String()
		return rar, nil
	}
	return nil, nil
}

// ReleaseAddress does nothing
func (d *Driver) ReleaseAddress(r *gphipam.ReleaseAddressRequest) error {
	return nil
}
