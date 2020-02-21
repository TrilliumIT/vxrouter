package ipam

import (
	"fmt"

	log "github.com/sirupsen/logrus"
	gphipam "github.com/docker/go-plugins-helpers/ipam"

	"github.com/TrilliumIT/vxrouter"
	"github.com/TrilliumIT/vxrouter/docker/core"
)

const (
	// DriverName is the name of the driver
	DriverName = vxrouter.IpamDriver
)

// Driver is the driver ipam type
type Driver struct {
	core *core.Core
	log  *log.Entry
}

// NewDriver creates new ipam driver
func NewDriver(core *core.Core) (*Driver, error) {
	return &Driver{core, log.WithField("driver", DriverName)}, nil
}

// GetCapabilities does nothing
func (d *Driver) GetCapabilities() (*gphipam.CapabilitiesResponse, error) {
	d.log.Debug("GetCapabilities()")
	return &gphipam.CapabilitiesResponse{}, nil
}

// GetDefaultAddressSpaces does nothing
func (d *Driver) GetDefaultAddressSpaces() (*gphipam.AddressSpacesResponse, error) {
	d.log.Debug("GetDefaultAddressSpaces()")
	return &gphipam.AddressSpacesResponse{}, nil
}

// RequestPool reflects the pool back to the caller
func (d *Driver) RequestPool(r *gphipam.RequestPoolRequest) (*gphipam.RequestPoolResponse, error) {
	d.log.WithField("r", r).Debug("RequestPool()")

	if r.Pool == "" {
		return nil, fmt.Errorf("this driver does not support automatic address pools")
	}

	rpr := &gphipam.RequestPoolResponse{
		PoolID: DriverName + "/" + r.Pool,
		Pool:   r.Pool,
	}

	return rpr, nil
}

// ReleasePool clears the network resource cache from core
func (d *Driver) ReleasePool(r *gphipam.ReleasePoolRequest) error {
	d.log.WithField("r", r).Debug("ReleasePool()")
	d.core.Uncache(r.PoolID)
	return nil
}

// RequestAddress calls the core function to connect and get an available address
func (d *Driver) RequestAddress(r *gphipam.RequestAddressRequest) (*gphipam.RequestAddressResponse, error) {
	d.log.WithField("r", r).Debug("RequestAddress()")

	// Always respond with the gateway address if specified
	// This is called on network create, and network create will fail if this returns an error
	if r.Options["RequestAddressType"] == "com.docker.network.gateway" && r.Address != "" {
		r, err := core.IPNetFromReqInfo(r.PoolID, r.Address)
		if err != nil {
			return nil, err
		}
		return &gphipam.RequestAddressResponse{
			Address: r.String(),
		}, nil
	}

	addr, err := d.core.ConnectAndGetAddress(r.Address, r.PoolID)
	if err != nil {
		log.WithField("r.Address", r.Address).WithField("r.PoolID", r.PoolID).Error("failed to get address")
		return nil, err
	}

	rar := &gphipam.RequestAddressResponse{
		Address: addr.String(),
	}

	return rar, nil
}

// ReleaseAddress does nothing
func (d *Driver) ReleaseAddress(r *gphipam.ReleaseAddressRequest) error {
	d.log.WithField("r", r).Debug("ReleaseAddress()")

	return d.core.DeleteRoute(r.Address)
}
