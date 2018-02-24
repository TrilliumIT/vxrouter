package host

import (
	"fmt"
	"net"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/vishvananda/netlink"

	"github.com/TrilliumIT/iputil"
	"github.com/TrilliumIT/vxrouter"
	"github.com/TrilliumIT/vxrouter/macvlan"
	"github.com/TrilliumIT/vxrouter/vxlan"
)

var (
	rwm              = make(map[string]*sync.RWMutex)
	rwmLock          sync.Mutex
	routeProto       = vxrouter.GetEnvIntWithDefault(vxrouter.EnvPrefix+"ROUTE_PROTO", "", vxrouter.DefaultRouteProto)
	reqAddrSleepTime = vxrouter.GetEnvDurWithDefault(vxrouter.EnvPrefix+"REQ_ADDR_SLEEP", "", vxrouter.DefaultReqAddrSleepTime)
)

// Interface holds a vxlan and a host macvlan interface used for the gateway interface on a container network
type Interface struct {
	name string
	vxl  *vxlan.Vxlan
	mvl  *macvlan.Macvlan
	log  *log.Entry
	l    *sync.RWMutex
}

// GetOrCreateInterface creates required host interfaces if they don't exist, or gets them if they already do
func GetOrCreateInterface(name string, gateway *net.IPNet, opts map[string]string) (*Interface, error) {
	log := log.WithField("Interface", name)
	log.Debug("GetOrCreateInterface()")
	hi, _ := getInterface(name)
	hi.log = log

	if hi.vxl != nil && hi.mvl != nil && hi.mvl.HasAddress(gateway) {
		return hi, nil
	}
	hi.l.Lock()
	defer hi.l.Unlock()
	hi, _ = getInterface(name)

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

// GetInterface gets host interfaces by name
func GetInterface(name string) (*Interface, error) {
	hi, err := getInterface(name)
	if err != nil {
		return nil, err
	}
	return hi, err
}

func getInterface(name string) (*Interface, error) {
	log := log.WithField("Interface", name)
	log.Debug("getInterface")

	hi := &Interface{
		name: name,
		log:  log,
	}

	rwmLock.Lock()

	if _, ok := rwm[name]; !ok {
		rwm[name] = &sync.RWMutex{}
	}

	hi.l = rwm[name]

	rwmLock.Unlock()

	var err error
	hi.vxl, err = vxlan.FromName(name)
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

// Lock locks the host interface
func (hi *Interface) Lock() {
	hi.l.Lock()
}

// Unlock unlocks the host interface
func (hi *Interface) Unlock() {
	hi.l.Unlock()
}

// CreateMacvlan creates container macvlan interfaces
func (hi *Interface) CreateMacvlan(name string) error {
	hi.log.WithField("Macvlan", name).Debug("CreateMacvlan")
	_, err := hi.vxl.CreateMacvlan(name)
	return err
}

// DeleteMacvlan deletes a container macvlan interface
func (hi *Interface) DeleteMacvlan(name string) error {
	hi.log.WithField("Macvlan", name).Debug("DeleteMacvlan")
	return hi.vxl.DeleteMacvlan(name)
}

// Delete deletes the host interface, only if there are no additional slave devices attached to the vxlan
// this function assumes no containers are running on this macvlan, as slaves in other namespaces
// will not show up
// Caller is responsible for locking/unlocking the host interface before calling delete
func (hi *Interface) Delete() error {
	hi.log.Debug("Delete")

	// if there are any other slaves, don't delete
	slaves, err := hi.vxl.GetSlaveDevices()
	if err != nil {
		hi.log.WithError(err).Debug("failed to get slaves from vxlan")
		return err
	}
	for _, slave := range slaves {
		if slave.Attrs().Index == hi.mvl.GetIndex() {
			continue
		}
		hi.log.Debug("other slave devices still exist on this vxlan")
		return nil
	}

	// if there are any other routes, don't delete
	routes, err := netlink.RouteListFiltered(netlink.FAMILY_ALL, &netlink.Route{LinkIndex: hi.mvl.GetIndex(), Protocol: routeProto}, netlink.RT_FILTER_OIF|netlink.RT_FILTER_PROTOCOL)
	if err != nil {
		hi.log.WithError(err).Error("failed to get routes")
		return err
	}
	for _, r := range routes {
		hi.log.WithField("r.Dst", r.Dst.String()).Debug("other routes found on this device, not deleting")
		return nil
	}

	rwmLock.Lock()
	defer rwmLock.Unlock()
	delete(rwm, hi.name)

	return hi.vxl.Delete()
}

func (hi *Interface) getSubnet() (*net.IPNet, error) {
	gws, err := hi.mvl.GetAddresses()
	if err != nil {
		return nil, err
	}
	for _, gw := range gws {
		return &net.IPNet{IP: iputil.FirstAddr(gw), Mask: gw.Mask}, nil
	}

	return nil, fmt.Errorf("did not find any addresses on the macvlan")
}

// SelectAddress returns an available IP or the requested IP (if available) or an error on timeout
func (hi *Interface) SelectAddress(reqAddress net.IP, propTime, respTime time.Duration, xf, xl int) (*net.IPNet, error) {
	log := hi.log
	if reqAddress != nil {
		log = log.WithField("reqAddress", reqAddress.String())
	}
	log.Debug("SelectAddress()")

	hi.l.RLock()
	defer hi.l.RUnlock()

	var ip *net.IPNet
	var err error

	var sleepTime time.Duration
	if reqAddress != nil {
		sleepTime = reqAddrSleepTime
	}

	stop := time.Now().Add(respTime)
	for time.Now().Before(stop) {
		ip, err = hi.selectAddress(reqAddress, propTime, xf, xl)
		if err != nil {
			log.WithError(err).Error("failed to select address")
			return nil, err
		}
		if ip != nil {
			break
		}
		time.Sleep(sleepTime)
	}

	if ip == nil {
		err = fmt.Errorf("timeout expired while waiting for address")
		log.WithError(err).Error()
		return nil, err
	}

	return ip, nil
}

// selectAddress returns an available random IP on this network, or the requested IP
// if it's available. This function may return (nil, nil) if it selects an unavailable address
// the intention is for the caller to continue calling in a loop until an address is returned
// this way the caller can implement their own timeout logic
func (hi *Interface) selectAddress(reqAddress net.IP, propTime time.Duration, xf, xl int) (*net.IPNet, error) {
	sn, err := hi.getSubnet()
	if err != nil {
		return nil, err
	}

	addrInSubnet, addrOnly := getIPNets(reqAddress, sn)

	if reqAddress != nil && !sn.Contains(reqAddress) {
		return nil, fmt.Errorf("requested address was not in this host interface's subnet")
	}

	// keep looking for a random address until one is found
	if reqAddress == nil {
		addrOnly.IP = iputil.RandAddrWithExclude(sn, xf, xl)
		addrInSubnet.IP = addrOnly.IP
	}
	numRoutes, err := numRoutesTo(addrOnly)
	if err != nil {
		log.WithError(err).Errorf("failed to count routes")
		return nil, err
	}
	if numRoutes > 0 {
		return nil, nil
	}

	log := log.WithField("ip", addrOnly.IP.String())

	// add host route to routing table
	log.Debug("adding route to")
	err = netlink.RouteAdd(&netlink.Route{
		LinkIndex: hi.mvl.GetIndex(),
		Dst:       addrOnly,
		Protocol:  routeProto,
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

// DelRoute deletes the /32 or /128 to the passed address
func (hi *Interface) DelRoute(ip net.IP) error {
	sn, err := hi.getSubnet()
	if err != nil {
		return err
	}

	_, addrOnly := getIPNets(ip, sn)

	return netlink.RouteDel(&netlink.Route{
		LinkIndex: hi.mvl.GetIndex(),
		Dst:       addrOnly,
		Protocol:  routeProto,
	})
}
