package vxlan

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/vishvananda/netlink"

	"github.com/TrilliumIT/vxrouter"
	"github.com/TrilliumIT/vxrouter/macvlan"
)

const (
	envPrefix = vxrouter.EnvPrefix
)

// Vxlan is a vxlan interface
type Vxlan struct {
	nl  *netlink.Vxlan
	log *log.Entry
}

func parseIP(s string) (net.IP, error) {
	r := net.ParseIP(s)
	var err error
	if r == nil {
		err = fmt.Errorf("failed to parse ip")
	}
	return r, err
}

func parseInt(v string) (int, error) {
	i, err := strconv.ParseInt(v, 0, 32)
	return int(i), err
}

func linkIndexByName(name string) (int, error) {
	var i int
	dev, err := netlink.LinkByName(name)
	if err == nil {
		i = dev.Attrs().Index
	}
	return i, err

}

// NewVxlan creates a new vxlan interface
func NewVxlan(vxlanName string, opts map[string]string) (*Vxlan, error) {
	log := log.WithField("Vxlan", vxlanName)
	var ok bool
	keys := [...]string{"vxlanmtu", "vxlanhardwareaddr", "vxlantxqlen", "vxlanid", "vtepdev", "srcaddr", "group", "ttl", "tos", "learning", "proxy", "rsc", "l2miss", "l3miss", "noage", "gbp", "age", "limit", "port", "portlow", "porthigh", "vxlanhardwareaddr", "vxlanmtu"}

	for _, k := range keys {
		if _, ok = opts[k]; !ok && os.Getenv(envPrefix+k) != "" {
			opts[k] = os.Getenv(envPrefix + k)
		}
	}

	nl := &netlink.Vxlan{
		LinkAttrs: netlink.LinkAttrs{
			Name: vxlanName,
		},
	}

	// Parse interface options
	var err error
	for k, v := range opts {
		log := log.WithField(k, v) // nolint: vetshadow
		switch strings.ToLower(k) {
		case "vxlanmtu":
			nl.LinkAttrs.MTU, err = strconv.Atoi(v)
		case "vxlanhardwareaddr":
			nl.LinkAttrs.HardwareAddr, err = net.ParseMAC(v)
		case "vxlantxqlen":
			nl.LinkAttrs.TxQLen, err = strconv.Atoi(v)
		case "vxlanid":
			nl.VxlanId, err = parseInt(v)
		case "vtepdev":
			nl.VtepDevIndex, err = linkIndexByName(v)
		case "srcaddr":
			nl.SrcAddr, err = parseIP(v)
		case "group":
			nl.Group = net.ParseIP(v)
		case "ttl":
			nl.TTL, err = strconv.Atoi(v)
		case "tos":
			nl.TOS, err = strconv.Atoi(v)
		case "learning":
			nl.Learning, err = strconv.ParseBool(v)
		case "proxy":
			nl.Proxy, err = strconv.ParseBool(v)
		case "rsc":
			nl.RSC, err = strconv.ParseBool(v)
		case "l2miss":
			nl.L2miss, err = strconv.ParseBool(v)
		case "l3miss":
			nl.L3miss, err = strconv.ParseBool(v)
		case "noage":
			nl.NoAge, err = strconv.ParseBool(v)
		case "gbp":
			nl.GBP, err = strconv.ParseBool(v)
		case "age":
			nl.Age, err = strconv.Atoi(v)
		case "limit":
			nl.Limit, err = strconv.Atoi(v)
		case "port":
			nl.Port, err = strconv.Atoi(v)
		case "portlow":
			nl.PortLow, err = strconv.Atoi(v)
		case "porthigh":
			nl.PortHigh, err = strconv.Atoi(v)
		}
		if err != nil {
			log.WithError(err).Debug()
			return nil, err
		}
	}

	err = netlink.LinkAdd(nl)
	if err != nil {
		log.Errorf("Error adding vxlan interface: %v", err)
		return nil, err
	}

	// Parse interface options
	for k, v := range opts {
		log := log.WithField(k, v) // nolint: vetshadow
		switch strings.ToLower(k) {
		case "vxlanhardwareaddr":
			var hardwareAddr net.HardwareAddr
			hardwareAddr, err = net.ParseMAC(v)
			if err != nil {
				break
			}
			err = netlink.LinkSetHardwareAddr(nl, hardwareAddr)
		case "vxlanmtu":
			var mtu int
			mtu, err = strconv.Atoi(v)
			if err != nil {
				break
			}
			err = netlink.LinkSetMTU(nl, mtu)
		}
		if err != nil {
			log.WithError(err).Debug()
			return nil, err
		}
	}

	// bring interfaces up
	err = netlink.LinkSetUp(nl)
	if err != nil {
		log.WithError(err).Debug("failed to bring up vxlan")
		return nil, err
	}

	return &Vxlan{nl, log}, nil
}

// FromName gets a vxlan interface by name
func FromName(name string) (*Vxlan, error) {
	log := log.WithField("Vxlan", name)
	log.Debug("GetVxlan")

	link, err := netlink.LinkByName(name)
	if err != nil {
		log.WithError(err).Debug("error getting link by name")
		return nil, err
	}

	if nl, ok := link.(*netlink.Vxlan); ok {
		return &Vxlan{nl, log}, nil
	}

	log.Debug("link is not a vxlan")
	return nil, fmt.Errorf("link is not a vxlan")
}

// CreateMacvlan creates a macvlan as a slave to v
func (v *Vxlan) CreateMacvlan(name string) (*macvlan.Macvlan, error) {
	v.log.Debug("CreateMacVlan")
	return macvlan.NewMacvlan(name, v.nl.LinkAttrs.Index)
}

// DeleteMacvlan deletes the slave macvlan interface by name
func (v *Vxlan) DeleteMacvlan(name string) error {
	v.log.Debug("DeleteMacvlan")

	mvl, err := macvlan.FromName(name)
	if err != nil {
		return err
	}

	if v.nl.Index != mvl.GetParentIndex() {
		return fmt.Errorf("macvlan is not a child of this vxlan")
	}

	return mvl.Delete()
}

// Delete deletes the vxlan interface.
// Any child macvlans will automatically be deleted by the kernel.
func (v *Vxlan) Delete() error {
	v.log.Debug("Delete")
	return netlink.LinkDel(v.nl)
}

// GetMacVlans returns all slave macvlan interfaces
func (v *Vxlan) GetMacVlans() ([]*macvlan.Macvlan, error) {
	v.log.Debug("GetMacVlans")
	r := []*macvlan.Macvlan{}

	allSlaves, err := v.GetSlaveDevices()
	if err != nil {
		return r, err
	}

	for _, link := range allSlaves {
		mvl, err := macvlan.FromLink(link)
		if err != nil {
			continue
		}
		r = append(r, mvl)
	}
	return r, nil
}

// GetSlaveDevices gets all slave devices, including macvlans, but possibly others
func (v *Vxlan) GetSlaveDevices() ([]netlink.Link, error) {
	v.log.Debug("GetSlaveDevices")
	r := []netlink.Link{}

	allLinks, err := netlink.LinkList()
	if err != nil {
		log.WithError(err).Debug("failed to get all links")
		return r, err
	}

	for _, link := range allLinks {
		if link.Attrs().MasterIndex != v.nl.Attrs().Index {
			continue
		}
		r = append(r, link)
	}
	return r, nil
}
