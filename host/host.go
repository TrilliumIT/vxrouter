package host

import (
	"fmt"
	"net"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/vishvananda/netlink"

	"github.com/TrilliumIT/iputil"
	"github.com/TrilliumIT/vxrouter/macvlan"
	"github.com/TrilliumIT/vxrouter/vxlan"
)

// HostInterface holds a vxlan and a host macvlan interface used for the gateway interface on a container network
type HostInterface struct {
	name string
	vxl  *vxlan.Vxlan
	mvl  *macvlan.Macvlan
	log  *log.Entry
}

// GetOrCreateHostInterface creates required host interfaces if they don't exist, or gets them if they already do
func GetOrCreateHostInterface(name string, gateway *net.IPNet, opts map[string]string) (*HostInterface, error) {
	log := log.WithField("HostInterface", name)
	log.Debug("GetOrCreateHostInterface")
	hi, _ := getHostInterface(name)
	log = hi.log

	var err error
	if hi.vxl == nil {
		hi.vxl, err = vxlan.NewVxlan(name, opts)
		if err != nil {
			log.WithError(err).Debug("failed to create vxlan")
			return nil, err
		}
	}

	if hi.mvl == nil {
		hi.mvl, err = hi.vxl.CreateMacvlan("hmvl_" + name)
		if err != nil {
			err2 := hi.Delete()
			if err2 != nil {
				log.WithError(err).WithError(err2).Debug("failed to delete vxlan")
				return nil, err2
			}
			return nil, err
		}
	}

	if hi.mvl.HasAddress(gateway) {
		return hi, nil
	}

	err = hi.mvl.AddAddress(gateway)
	if err != nil {
		log.WithError(err).Debug("failed to add address to macvlan")
		//implicitly deletes macvlan
		err2 := hi.Delete()
		if err2 != nil {
			log.WithError(err).WithError(err2).Debug("failed to delete vxlan")
			return nil, err2
		}
		return nil, err
	}

	return hi, nil
}

// GetHostInterface gets host interfaces by name
func GetHostInterface(name string) (*HostInterface, error) {
	hi, err := getHostInterface(name)
	if err != nil {
		return nil, err
	}
	return hi, err
}

func getHostInterface(name string) (*HostInterface, error) {
	log := log.WithField("HostInterface", name)
	log.Debug("GetHostInterface")

	hi := &HostInterface{
		name: name,
		log:  log,
	}

	var err error
	hi.vxl, err = vxlan.GetVxlan(name)
	if err != nil {
		log.Debug("failed to get vxlan interface")
		return hi, err
	}

	hi.mvl, err = macvlan.FromName("hmvl_" + name)
	if err != nil {
		log.Debug("failed to get macvlan interface")
	}

	return hi, err
}

// CreateMacvlan creates container macvlan interfaces
func (hi *HostInterface) CreateMacvlan(name string) error {
	hi.log.WithField("Macvlan", name).Debug("CreateMacvlan")
	_, err := hi.vxl.CreateMacvlan(name)
	return err
}

// DeleteMacvlan deletes a container macvlan interface
func (hi *HostInterface) DeleteMacvlan(name string) error {
	hi.log.WithField("Macvlan", name).Debug("DeleteMacvlan")
	return hi.vxl.DeleteMacvlan(name)
}

// Delete deletes the host interface, only if there are no additional slave devices attached to the vxlan
func (hi *HostInterface) Delete() error {
	hi.log.Debug("Delete")

	slaves, err := hi.vxl.GetSlaveDevices()
	if err != nil {
		hi.log.WithError(err).Debug("failed to get slaves from vxlan")
		return err
	}

	var mvl *macvlan.Macvlan
	for _, slave := range slaves {
		// err != nil implies this slave is not a macvlan device, something was added manually, we better not delete the interface
		// Only slave interface that should be here is hi.mvl
		if mvl, err = macvlan.FromLink(slave); err == nil && mvl.Equals(hi.mvl) {
			continue
		}
		err = fmt.Errorf("other slave devices still exist on this vxlan")
		return err
	}

	return hi.vxl.Delete()
}

func numRoutesTo(ipnet *net.IPNet) (int, error) {
	routes, err := netlink.RouteListFiltered(0, &netlink.Route{Dst: ipnet}, netlink.RT_FILTER_DST)
	if err != nil {
		log.WithError(err).Error("failed to get routes")
		return -1, err
	}
	return len(routes), nil
}

func (hi *HostInterface) getConnectionInfo() (net.IP, *net.IPNet, error) {
	gws, err := hi.mvl.GetAddresses()
	if err != nil {
		return nil, nil, err
	}
	for _, gw := range gws {
		return gw.IP, &net.IPNet{IP: iputil.FirstAddr(gw), Mask: gw.Mask}, nil
	}

	return nil, nil, fmt.Errorf("did not find any addresses on the macvlan")
}

//this function may return (nil, nil) if it selects an unavailable address
//the intention is for the caller to continue calling in a loop until an address is returned
//this way the caller can implement their own timeout logic
func (hi *HostInterface) SelectAddress(reqAddress net.IP, propTime time.Duration, xf, xl int) (*net.IPNet, error) {
	gw, sn, err := hi.getConnectionInfo()
	if err != nil {
		return nil, err
	}

	addrInSubnet, addrOnly := getIPNets(reqAddress, sn)
	log := hi.log.WithField("ip", addrOnly.IP.String())

	if reqAddress != nil && !sn.Contains(reqAddress) {
		return nil, fmt.Errorf("requested address was not in this host interface's subnet")
	}

	// keep looking for a random address until one is found
	numRoutes := 1
	if reqAddress == nil {
		addrOnly.IP = iputil.RandAddrWithExclude(sn, xf, xl)
		addrInSubnet.IP = addrOnly.IP
	}
	numRoutes, err = numRoutesTo(addrOnly)
	if err != nil {
		log.WithError(err).Errorf("failed to count routes")
		return nil, err
	}
	if numRoutes > 0 {
		return nil, nil
	}

	// add host route to routing table
	log.Debug("adding route to")
	err = netlink.RouteAdd(&netlink.Route{
		Dst: addrOnly,
		Gw:  gw,
	})
	if err != nil {
		log.WithError(err).Error("failed to add route")
		return nil, err
	}

	//wait for at least estimated route propagation time
	time.Sleep(propTime)

	//check that we are still the only route
	numRoutes, err = numRoutesTo(addrOnly)
	if err != nil {
		log.WithError(err).Error("failed to count routes")
		return nil, err
	}

	if numRoutes == 1 {
		return addrInSubnet, nil
	}

	log.Info("someone else grabbed ip first")

	err = hi.DelRoute(addrOnly.IP)
	if err != nil {
		log.WithError(err).Error("failed to delete dup route")
		return nil, err
	}

	return nil, nil
}

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

func (hi *HostInterface) DelRoute(ip net.IP) error {
	gw, sn, err := hi.getConnectionInfo()
	if err != nil {
		return err
	}

	_, addrOnly := getIPNets(ip, sn)

	return netlink.RouteDel(&netlink.Route{
		Dst: addrOnly,
		Gw:  gw,
	})
}
