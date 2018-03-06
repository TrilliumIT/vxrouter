package host

import (
	"net"

	log "github.com/Sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

func getIPNets(address net.IP, subnet *net.IPNet) (*net.IPNet, *net.IPNet) {
	var sna, a *net.IPNet

	//address in big subnet
	if subnet != nil {
		sna = &net.IPNet{
			IP:   address,
			Mask: subnet.Mask,
		}
	}

	//address as host route (like /32 or /128)
	a = &net.IPNet{IP: address}
	if a.IP.To4() != nil {
		a.Mask = net.CIDRMask(32, 32)
	} else {
		a.Mask = net.CIDRMask(128, 128)
	}

	return sna, a
}

func numRoutesTo(ipnet *net.IPNet) (int, error) {
	routes, err := netlink.RouteListFiltered(0, &netlink.Route{Dst: ipnet}, netlink.RT_FILTER_DST)
	if err != nil {
		log.WithError(err).Error("failed to get routes")
		return -1, err
	}
	return len(routes), nil
}

// VxroutesTo return sthe number of vxrouter routes to a specific IP
func VxroutesTo(ip net.IP) (int, error) {
	_, a := getIPNets(ip, nil)
	routes, err := netlink.RouteListFiltered(0, &netlink.Route{Dst: a, Protocol: routeProto}, netlink.RT_FILTER_DST|netlink.RT_FILTER_PROTOCOL)
	if err != nil {
		log.WithError(err).Error("failed to get routes")
		return -1, err
	}
	return len(routes), nil
}

// AllVxRoutes returns a list of IPNets which there are vxrouer routes to
func AllVxRoutes() ([]*net.IPNet, error) {
	ret := []*net.IPNet{}
	routes, err := netlink.RouteListFiltered(0, &netlink.Route{Protocol: routeProto}, netlink.RT_FILTER_PROTOCOL)
	if err != nil {
		log.WithError(err).Error("failed to get routes")
		return ret, err
	}

	for _, r := range routes {
		ret = append(ret, r.Dst)
	}
	return ret, nil
}
