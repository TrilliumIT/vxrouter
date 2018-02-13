package vxrNet

import (
	"net"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types"
	"github.com/vishvananda/netlink"
	"golang.org/x/net/context"
)

func (d *Driver) reconcile() error {
	dnets, err := d.client.NetworkList(context.Background(), types.NetworkListOptions{})
	if err != nil {
		return err
	}
	nets := make(map[string]*types.NetworkResource)
	gws := make(map[string]net.IP)
	for _, dn := range dnets {
		if dn.Driver != DriverName {
			continue
		}
		for _, ipc := range dn.IPAM.Config {
			gws[ipc.Gateway] = net.ParseIP(ipc.Gateway)
		}
		nets[dn.Name] = &dn
	}

	var cRoutes map[string]*netlink.Route
	for _, gw := range gws {
		tcRoutes, err := netlink.RouteListFiltered(0, &netlink.Route{Gw: gw}, netlink.RT_FILTER_GW)
		if err != nil {
			return err
		}
		for _, r := range tcRoutes {
			o, b := r.Dst.Mask.Size()
			if o < b { // filters to /32 routes, ones is not less than bits
				continue
			}
			cRoutes[r.Dst.IP.String()] = &r
		}
	}

	containers, err := d.client.ContainerList(context.Background(), types.ContainerListOptions{})
	if err != nil {
		d.log.WithError(err).Error("failed to list containers")
		return err
	}

	for _, c := range containers {
		for n, nr := range c.NetworkSettings.Networks {
			if _, ok := gws[nr.Gateway]; !ok { // if this network gateway is not me
				continue
			}
			if _, ok := nets[n]; !ok { // this network isn't a vxrnet
				continue
			}
			if _, ok := cRoutes[nr.IPAMConfig.IPv4Address]; ok { // this route already exists
				delete(cRoutes, nr.IPAMConfig.IPv4Address) // delete from the map
				continue
			}

			// At this point, a route needs to be added
			for _, ipc := range nets[n].IPAM.Config {
				_, subnet, err := net.ParseCIDR(ipc.Subnet)
				if err != nil {
					return err
				}

				_, addrOnly := getAddresses(nr.IPAMConfig.IPv4Address, subnet)

				if !subnet.Contains(addrOnly.IP) { // maybe this is the wrong ipam.config, idk
					continue
				}

				err = netlink.RouteAdd(&netlink.Route{
					Dst: addrOnly,
					Gw:  gws[nr.Gateway],
				})
				if err != nil {
					log.WithError(err).Error("failed to add route")
					return err
				}
			}

		}
	}

	// cleanup extra routes
	for _, r := range cRoutes {
		err := netlink.RouteDel(r)
		if err != nil {
			return err
		}
	}

	return nil
}
