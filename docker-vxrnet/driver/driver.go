package driver

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/go-plugins-helpers/network"
	"github.com/vishvananda/netlink"
	"golang.org/x/net/context"

	"github.com/TrilliumIT/iputil"
	"github.com/TrilliumIT/vxrouter/hostInterface"
)

const (
	envPrefix  = "VXR_"
	DriverName = "vxrNet"
)

// Driver is a vxrouter network driver
type Driver struct {
	scope       string
	propTime    time.Duration
	respTime    time.Duration
	client      *client.Client
	log         *log.Entry
	nrCache     map[string]*types.NetworkResource
	nrCacheLock *sync.RWMutex
}

// NewDriver creates a new Driver
func NewDriver(scope string, propTime, respTime time.Duration, client *client.Client) (*Driver, error) {
	d := &Driver{
		scope,
		propTime,
		respTime,
		client,
		log.WithField("driver", DriverName),
		make(map[string]*types.NetworkResource),
		&sync.RWMutex{},
	}
	return d, nil
}

// GetCapabilities is called on driver initialization
func (d *Driver) GetCapabilities() (*network.CapabilitiesResponse, error) {
	d.log.Debug("GetCapabilities()")
	cap := &network.CapabilitiesResponse{
		Scope:             d.scope,
		ConnectivityScope: "",
	}
	return cap, nil
}

// CreateNetwork is called on docker network create
func (d *Driver) CreateNetwork(r *network.CreateNetworkRequest) error {
	d.log.WithField("r", r).Debug("CreateNetwork()")

	//Even though we are stateless
	//Validate required options on create to notify the user

	opts, ok := r.Options["com.docker.network.generic"].(map[string]interface{})
	if !ok {
		err := fmt.Errorf("did not retrieve the options array for the network")
		log.WithError(err).Error()
		return err
	}

	//make sure gateway option was specified
	gw, ok := opts["gateway"]
	if !ok {
		err := fmt.Errorf("cannot create a network without a CIDR gateway (-o gateway=<address>/<mask>)")
		d.log.WithError(err).Error()
		return err
	}

	//make sure gateway option is a CIDR
	_, _, err := net.ParseCIDR(gw.(string))
	if err != nil {
		d.log.WithError(err).WithField("gw", gw).Error("failed to parse gateway")
		return err
	}

	vxlID, ok := opts["vxlanid"]
	if !ok {
		err := fmt.Errorf("cannot create a network without a vxlanid (-o vxlanid=<0-16777215>)")
		d.log.WithError(err).Error()
		return err
	}

	vid, err := strconv.Atoi(vxlID.(string))
	if err != nil {
		d.log.WithError(err).WithField("vxlanid", vxlID.(string)).Errorf("failed to parse vxlanid")
		return err
	}
	if vid < 0 || vid > 16777215 {
		err := fmt.Errorf("vxlanid (%v) is out of range", vid)
		d.log.WithError(err).Error()
		return err
	}

	return nil
}

// AllocateNetwork is never called
func (d *Driver) AllocateNetwork(r *network.AllocateNetworkRequest) (*network.AllocateNetworkResponse, error) {
	d.log.WithField("r", r).Debug("AllocateNetwork()")
	return &network.AllocateNetworkResponse{}, nil
}

// DeleteNetwork is called on docker network rm
func (d *Driver) DeleteNetwork(r *network.DeleteNetworkRequest) error {
	d.log.WithField("r", r).Debug("DeleteNetwork()")
	return nil
}

// FreeNetwork is never called
func (d *Driver) FreeNetwork(r *network.FreeNetworkRequest) error {
	d.log.WithField("r", r).Debug("FreeNetwork()")
	return nil
}

// CreateEndpoint is called after IPAM has assigned an address, before Join is called
func (d *Driver) CreateEndpoint(r *network.CreateEndpointRequest) (*network.CreateEndpointResponse, error) {
	d.log.WithField("r", r).Debug("CreateEndpoint()")

	nr, err := d.getNetworkResource(r.NetworkID)
	if err != nil {
		d.log.WithError(err).WithField("NetworkID", r.NetworkID).Error("failed to get network resource")
		return nil, err
	}

	gw, sn, _ := net.ParseCIDR(nr.Options["gateway"])

	//exclude network and (normal) broadcast addresses by default

	xf := getEnvIntWithDefault(envPrefix+"excludefirst", nr.Options["excludefirst"], 1)
	xl := getEnvIntWithDefault(envPrefix+"excludelast", nr.Options["excludelast"], 1)

	_, err = hostInterface.GetOrCreateHostInterface(nr.Name, &net.IPNet{IP: gw, Mask: sn.Mask}, nr.Options)
	if err != nil {
		d.log.WithError(err).WithField("NetworkID", r.NetworkID).Error("failed to get or create host interface")
		return nil, err
	}

	var ip *net.IPNet
	stop := time.Now().Add(d.respTime)
	for time.Now().Before(stop) {
		ip, err = d.selectAddress(r.Interface.Address, r.NetworkID, gw, sn, xf, xl)
		if err != nil {
			d.log.WithError(err).Error("failed to select address")
			return nil, err
		}
		if ip != nil {
			break
		}
		if r.Interface.Address != "" {
			time.Sleep(10 * time.Millisecond)
		}
	}

	if ip == nil {
		err = fmt.Errorf("timeout expired while waiting for address")
		d.log.WithError(err).Error()
		return nil, err
	}

	cer := &network.CreateEndpointResponse{
		Interface: &network.EndpointInterface{
			Address: ip.String(),
		},
	}
	return cer, nil
}

// DeleteEndpoint is called after Leave
func (d *Driver) DeleteEndpoint(r *network.DeleteEndpointRequest) error {
	d.log.WithField("r", r).Debug("DeleteEndpoint()")

	nr, err := d.getNetworkResource(r.NetworkID)
	if err != nil {
		d.log.WithError(err).WithField("NetworkID", r.NetworkID).Error("failed to get network resource")
		return err
	}

	hi, err := hostInterface.GetHostInterface(nr.Name)
	if err != nil {
		return err
	}

	mvlName := "cmvl_" + r.EndpointID[:7]
	err = hi.DeleteMacvlan(mvlName)
	if err != nil {
		d.log.WithError(err).Error("failed to delete macvlan for container")
		return err
	}

	containers, err := d.client.ContainerList(context.Background(), types.ContainerListOptions{})
	if err != nil {
		d.log.WithError(err).Error("failed to list containers")
		return err
	}

	gw, sn, _ := net.ParseCIDR(nr.Options["gateway"])
	delHi := true
	for _, c := range containers {
		ns, ok := c.NetworkSettings.Networks[nr.Name]
		if !ok {
			continue
		}

		if ns.EndpointID != r.EndpointID {
			d.log.Debug("other containers are still running on this network")
			delHi = false
			continue
		}

		if err := delRoute(ns.IPAddress, gw, sn); err != nil {
			d.log.WithError(err).Debug("failed to delete route")
			return err
		}
		if delHi == false {
			break
		}
	}

	if delHi {
		return hi.Delete()
	}

	return nil
}

func delRoute(ip string, gw net.IP, sn *net.IPNet) error {
	_, addrOnly := getAddresses(ip, sn)

	return netlink.RouteDel(&netlink.Route{
		Dst: addrOnly,
		Gw:  gw,
	})
}

// EndpointInfo is called on inspect... maybe?
func (d *Driver) EndpointInfo(r *network.InfoRequest) (*network.InfoResponse, error) {
	d.log.WithField("r", r).Debug("EndpointInfo()")
	return &network.InfoResponse{}, nil
}

// Join is the last thing called before the nic is put into the container namespace
func (d *Driver) Join(r *network.JoinRequest) (*network.JoinResponse, error) {
	nr, err := d.getNetworkResource(r.NetworkID)
	if err != nil {
		d.log.WithError(err).WithField("NetworkID", r.NetworkID).Error("failed to get network resource")
		return nil, err
	}

	hi, err := hostInterface.GetHostInterface(nr.Name)
	if err != nil {
		return nil, err
	}

	mvlName := "cmvl_" + r.EndpointID[:7]
	err = hi.CreateMacvlan(mvlName)
	if err != nil {
		d.log.WithError(err).Error("failed to create macvlan for container")
		return nil, err
	}

	gw, _, err := net.ParseCIDR(nr.Options["gateway"])

	jr := &network.JoinResponse{
		InterfaceName: network.InterfaceName{
			SrcName:   mvlName,
			DstPrefix: "eth",
		},
		Gateway: gw.String(),
	}

	return jr, nil
}

// Leave is the first thing called on container stop
func (d *Driver) Leave(r *network.LeaveRequest) error {
	d.log.WithField("r", r).Debug("Leave()")
	return nil
}

// DiscoverNew is not implemented by this driver
func (d *Driver) DiscoverNew(r *network.DiscoveryNotification) error {
	d.log.WithField("r", r).Debug("DiscoverNew()")
	return nil
}

// DiscoverDelete is not implemented by this driver
func (d *Driver) DiscoverDelete(r *network.DiscoveryNotification) error {
	d.log.WithField("r", r).Debug("DiscoverDelete()")
	return nil
}

// ProgramExternalConnectivity is not implemented by this driver
func (d *Driver) ProgramExternalConnectivity(r *network.ProgramExternalConnectivityRequest) error {
	d.log.WithField("r", r).Debug("ProgramExternalConnectivity()")
	return nil
}

// RevokeExternalConnectivity is not implemented by this driver
func (d *Driver) RevokeExternalConnectivity(r *network.RevokeExternalConnectivityRequest) error {
	d.log.WithField("r", r).Debug("RevokeExternalConnectivity()")

	return nil
}

func (d *Driver) getNetworkResource(id string) (*types.NetworkResource, error) {
	log := d.log.WithField("net_id", id)
	log.Debug("getNetworkResource")

	//first check local cache with a read-only mutex
	d.nrCacheLock.RLock()
	var err error
	if nr, ok := d.nrCache[id]; ok {
		d.nrCacheLock.RUnlock()
		return nr, nil
	}
	d.nrCacheLock.RUnlock()

	//netid wasn't in cache, fetch from docker inspect
	d.nrCacheLock.Lock()
	defer d.nrCacheLock.Unlock()
	nr, err := d.client.NetworkInspect(context.Background(), id)
	if err != nil {
		log.WithError(err).Error("failed to inspect network")
		return nil, err
	}

	if nr.Driver != DriverName {
		err := fmt.Errorf("network is not a %v", DriverName)
		return nil, err
	}

	d.nrCache[id] = &nr

	return &nr, nil
}

func getAddresses(address string, sn *net.IPNet) (*net.IPNet, *net.IPNet) {
	sna := &net.IPNet{
		IP:   net.ParseIP(address),
		Mask: sn.Mask,
	}

	_, ml := sn.Mask.Size()
	a := &net.IPNet{
		IP:   sna.IP,
		Mask: net.CIDRMask(ml, ml),
	}

	return sna, a
}

func (d *Driver) numRoutesTo(ipnet *net.IPNet) (int, error) {
	routes, err := netlink.RouteListFiltered(0, &netlink.Route{Dst: ipnet}, netlink.RT_FILTER_DST)
	if err != nil {
		d.log.WithError(err).Error("failed to get routes")
		return -1, err
	}
	return len(routes), nil
}

//this function may return (nil, nil) if it selects an unavailable address
//the intention is for the caller to continue calling in a loop until an address is returned
//this way the caller can implement their own timeout logic
func (d *Driver) selectAddress(reqAddress, netid string, gateway net.IP, subnet *net.IPNet, xf, xl int) (*net.IPNet, error) {
	var err error

	addrInSubnet, addrOnly := getAddresses(reqAddress, subnet)

	log := d.log.WithField("ip", addrOnly.IP.String())

	// keep looking for a random address until one is found
	numRoutes := 1
	if reqAddress == "" {
		addrOnly.IP = iputil.RandAddrWithExclude(subnet, xf, xl)
		addrInSubnet.IP = addrOnly.IP
	}
	numRoutes, err = d.numRoutesTo(addrOnly)
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
		Gw:  gateway,
	})
	if err != nil {
		log.WithError(err).Error("failed to add route")
		return nil, err
	}

	//wait for at least estimated route propagation time
	time.Sleep(d.propTime)

	//check that we are still the only route
	numRoutes, err = d.numRoutesTo(addrOnly)
	if err != nil {
		log.WithError(err).Error("failed to count routes")
		return nil, err
	}

	if numRoutes == 1 {
		return addrInSubnet, nil
	}

	log.Info("someone else grabbed ip first")

	err = delRoute(addrOnly.IP.String(), gateway, subnet)
	if err != nil {
		log.WithError(err).Error("failed to delete dup route")
		return nil, err
	}

	return nil, nil
}

func getEnvIntWithDefault(val, opt string, def int) int {
	e := os.Getenv(val)
	if e == "" {
		e = opt
	}
	if e == "" {
		return def
	}
	ei, err := strconv.Atoi(e)
	if err != nil {
		log.WithField("string", e).WithError(err).Warnf("failed to convert string to int, using default")
		return def
	}
	return ei
}
