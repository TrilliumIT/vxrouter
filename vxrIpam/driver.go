package vxrIpam

import (
	"net"
	"time"

	"github.com/TrilliumIT/iputil"
	"github.com/docker/go-plugins-helpers/ipam"
	"github.com/vishvananda/netlink"

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
	return &ipam.RequestPoolResponse{
		PoolID: r.Pool,
		Pool:   r.Pool,
	}, nil
}

func (d *Driver) ReleasePool(r *ipam.ReleasePoolRequest) error {
	d.log.WithField("r", r).Debug("ReleasePoolRequest()")
	return nil
}

func (d *Driver) RequestAddress(r *ipam.RequestAddressRequest) (*ipam.RequestAddressResponse, error) {
	d.log.WithField("r", r).Debug("RequestAddress()")
	_, subnet, err := net.ParseCIDR(r.PoolID)
	if err != nil {
		d.log.WithError(err).Error("Error parsing pool id subnet")
	}

	addr := &net.IPNet{
		IP:   net.ParseIP(r.Address),
		Mask: subnet.Mask,
	}

	if r.Options["RequestAddressType"] == "com.docker.network.gateway" {
		return &ipam.RequestAddressResponse{
			Address: addr.String(),
		}, nil
	}

	_, ml := addr.Mask.Size()
	addr.Mask = net.CIDRMask(ml, ml)
	routes := []netlink.Route{{}}
	for len(routes) > 0 {
		if addr.IP == nil {
			addr.IP = iputil.RandAddr(subnet)
		}
		routes, err = netlink.RouteListFiltered(0, &netlink.Route{Dst: addr}, netlink.RT_FILTER_DST)
		if err != nil {
			d.log.WithError(err).Error("failed to get routes")
			return nil, err
		}
	}
	err = netlink.RouteAdd(&netlink.Route{
		Dst: addr,
		Gw:  net.ParseIP("127.0.0.1"),
	})
	if err != nil {
		d.log.WithError(err).Error("failed to add route")
		return nil, err
	}

	addr.Mask = subnet.Mask

	return &ipam.RequestAddressResponse{
		Address: addr.String(),
	}, nil
}

func (d *Driver) ReleaseAddress(r *ipam.ReleaseAddressRequest) error {
	d.log.WithField("r", r).Debug("ReleaseAddress()")
	return nil
}
