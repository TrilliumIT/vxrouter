package host

import (
	"net"

	log "github.com/Sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

func getIPNets(address net.IP, subnet *net.IPNet) (*net.IPNet, *net.IPNet) {
	//address in big subnet
	sna := &net.IPNet{
		IP:   address,
		Mask: subnet.Mask,
	}

	//address as host route (like /32 or /128)
	_, ml := subnet.Mask.Size()
	a := &net.IPNet{
		IP:   sna.IP,
		Mask: net.CIDRMask(ml, ml),
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
