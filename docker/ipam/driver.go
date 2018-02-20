package ipam

import (
	"fmt"
	"net"
	"strings"

	log "github.com/Sirupsen/logrus"
	gphipam "github.com/docker/go-plugins-helpers/ipam"

	"github.com/TrilliumIT/vxrouter/docker/client"
)

const (
	// DriverName is the name of the driver
	DriverName = "vxrIpam"
)

// Driver is the driver ipam type
type Driver struct {
	client *client.Client
}

// NewDriver creates new ipam driver
func NewDriver(client *client.Client) (*Driver, error) {
	return &Driver{client}, nil
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
	pool := poolFromID(r.PoolID)
	nr, err := d.client.GetNetworkResourceByPool(pool)
	if err != nil {
		log.WithError(err).WithField("pool", pool).Error("failed to get network resource")
		return nil, err
	}

	gw, err := d.getGateway(r.NetworkID)
	if err != nil {
		log.WithError(err).Error("failed to get gateway")
		return nil, err
	}

	//exclude network and (normal) broadcast addresses by default
	xf := getEnvIntWithDefault(envPrefix+"excludefirst", nr.Options["excludefirst"], 1)
	xl := getEnvIntWithDefault(envPrefix+"excludelast", nr.Options["excludelast"], 1)

	hi, err := host.GetOrCreateInterface(nr.Name, gw, nr.Options)
	if err != nil {
		d.log.WithError(err).WithField("NetworkID", r.NetworkID).Error("failed to get or create host interface")
		return nil, err
	}

	rip, _, _ := net.ParseCIDR(r.Interface.Address) //nolint errcheck
	var ip *net.IPNet
	stop := time.Now().Add(d.respTime)
	for time.Now().Before(stop) {
		ip, err = hi.SelectAddress(rip, d.propTime, xf, xl)
		if err != nil {
			d.log.WithError(err).Error("failed to select address")
			return nil, err
		}
		if ip != nil {
			break
		}
		if rip != nil {
			time.Sleep(10 * time.Millisecond)
		}
	}

	if ip == nil {
		err = fmt.Errorf("timeout expired while waiting for address")
		d.log.WithError(err).Error()
		return nil, err
	}

	if rip != nil {
		return nil, nil
	}
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

func poolFromID(poolid string) string {
	return strings.TrimPrefix(poolid, ipamDriverName+"_")
}
