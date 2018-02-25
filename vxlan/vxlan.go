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
	name string
	log  *log.Entry
}

func new(name string) *Vxlan {
	log := log.WithField("Vxlan", name)
	log.WithField("Func", "new()").Debug()
	return &Vxlan{name, log}
}

func (v *Vxlan) nl() (*netlink.Vxlan, error) { // nolint: dupl
	log := v.log.WithField("Func", "nl()")
	log.Debug()

	link, err := netlink.LinkByName(v.name)
	if err != nil {
		log.WithError(err).Debug("failed to get link by name")
		return nil, err
	}

	return checkNl(link)
}

func checkNl(link netlink.Link) (*netlink.Vxlan, error) {
	if nl, ok := link.(*netlink.Vxlan); ok {
		return nl, nil
	}

	return nil, fmt.Errorf("link is not a vxlan")
}

func parseIP(s string) (net.IP, error) {
	r := net.ParseIP(s)
	var err error
	if r == nil {
		err = fmt.Errorf("failed to parse ip")
	}
	return r, err
}

// ParseVxlanID converts a string to a int to validate a vxlan id
func ParseVxlanID(v string) (int, error) {
	vid, err := strconv.Atoi(v)
	if err != nil {
		var v64 int64
		v64, err = strconv.ParseInt(v, 0, 0)
		vid = int(v64)
	}

	if vid < 0 || vid > 16777215 {
		err = fmt.Errorf("vxlanid is out of range")
	}
	return vid, err
}

func linkIndexByName(name string) (int, error) {
	var i int
	dev, err := netlink.LinkByName(name)
	if err == nil {
		i = dev.Attrs().Index
	}
	return i, err

}

func applyOpts(nl *netlink.Vxlan, opts map[string]string) (bool, error) {
	var ok bool
	keys := [...]string{"vxlanmtu", "vxlanhardwareaddr", "vxlantxqlen", "vxlanid", "vtepdev", "srcaddr", "group", "ttl", "tos", "learning", "proxy", "rsc", "l2miss", "l3miss", "noage", "gbp", "age", "limit", "port", "portlow", "porthigh", "vxlanhardwareaddr", "vxlanmtu"}

	for _, k := range keys {
		if _, ok = opts[k]; !ok && os.Getenv(envPrefix+k) != "" {
			opts[k] = os.Getenv(envPrefix + k)
		}
	}

	var changed bool
	var err error

	// Parse interface options
	for k, v := range opts {
		var o, n string
		log := log.WithField(k, v) // nolint: vetshadow
		switch strings.ToLower(k) {
		case "vxlanmtu": // don't check for change, can be changed after up
			nl.LinkAttrs.MTU, err = strconv.Atoi(v)
		case "vxlanhardwareaddr": // don't check for change, can be changed after up
			nl.LinkAttrs.HardwareAddr, err = net.ParseMAC(v)
		case "vxlantxqlen":
			o = strconv.Itoa(nl.LinkAttrs.TxQLen)
			nl.LinkAttrs.TxQLen, err = strconv.Atoi(v)
			n = strconv.Itoa(nl.LinkAttrs.TxQLen)
		case "vxlanid":
			o = strconv.Itoa(nl.VxlanId)
			nl.VxlanId, err = ParseVxlanID(v)
			n = strconv.Itoa(nl.VxlanId)
		case "vtepdev":
			o = strconv.Itoa(nl.VtepDevIndex)
			nl.VtepDevIndex, err = linkIndexByName(v)
			n = strconv.Itoa(nl.VtepDevIndex)
		case "srcaddr":
			o = nl.SrcAddr.String()
			nl.SrcAddr, err = parseIP(v)
			n = nl.SrcAddr.String()
		case "group":
			o = nl.Group.String()
			nl.Group = net.ParseIP(v)
			n = nl.Group.String()
		case "ttl":
			o = strconv.Itoa(nl.TTL)
			nl.TTL, err = strconv.Atoi(v)
			n = strconv.Itoa(nl.TTL)
		case "tos":
			o = strconv.Itoa(nl.TOS)
			nl.TOS, err = strconv.Atoi(v)
			n = strconv.Itoa(nl.TOS)
		case "learning":
			o = strconv.FormatBool(nl.Learning)
			nl.Learning, err = strconv.ParseBool(v)
			n = strconv.FormatBool(nl.Learning)
		case "proxy":
			o = strconv.FormatBool(nl.Proxy)
			nl.Proxy, err = strconv.ParseBool(v)
			n = strconv.FormatBool(nl.Proxy)
		case "rsc":
			o = strconv.FormatBool(nl.RSC)
			nl.RSC, err = strconv.ParseBool(v)
			n = strconv.FormatBool(nl.RSC)
		case "l2miss":
			o = strconv.FormatBool(nl.L2miss)
			nl.L2miss, err = strconv.ParseBool(v)
			n = strconv.FormatBool(nl.L2miss)
		case "l3miss":
			o = strconv.FormatBool(nl.L3miss)
			nl.L3miss, err = strconv.ParseBool(v)
			n = strconv.FormatBool(nl.L3miss)
		case "noage":
			o = strconv.FormatBool(nl.NoAge)
			nl.NoAge, err = strconv.ParseBool(v)
			n = strconv.FormatBool(nl.NoAge)
		case "gbp":
			o = strconv.FormatBool(nl.GBP)
			nl.GBP, err = strconv.ParseBool(v)
			n = strconv.FormatBool(nl.GBP)
		case "age":
			o = strconv.Itoa(nl.Age)
			nl.Age, err = strconv.Atoi(v)
			n = strconv.Itoa(nl.Age)
		case "limit":
			o = strconv.Itoa(nl.Limit)
			nl.Limit, err = strconv.Atoi(v)
			n = strconv.Itoa(nl.Limit)
		case "port":
			o = strconv.Itoa(nl.Port)
			nl.Port, err = strconv.Atoi(v)
			n = strconv.Itoa(nl.Port)
		case "portlow":
			o = strconv.Itoa(nl.PortLow)
			nl.PortLow, err = strconv.Atoi(v)
			n = strconv.Itoa(nl.PortLow)
		case "porthigh":
			o = strconv.Itoa(nl.PortHigh)
			nl.PortHigh, err = strconv.Atoi(v)
			n = strconv.Itoa(nl.PortHigh)
		}
		if err != nil {
			log.WithError(err).Debug()
			return changed, err
		}
		if o != n {
			changed = true
		}
	}

	return changed, nil
}

// New creates a new vxlan interface
func New(vxlanName string, opts map[string]string) (*Vxlan, error) {
	return newVxlan(vxlanName, opts, true)
}

func newVxlan(vxlanName string, opts map[string]string, retry bool) (*Vxlan, error) {
	v := new(vxlanName)
	log := v.log.WithField("Func", "NewVxlan()")
	log.Debug()

	new := false
	nl, err := v.nl()

	if err != nil {
		new = true
		nl = &netlink.Vxlan{
			LinkAttrs: netlink.LinkAttrs{
				Name: vxlanName,
			},
		}
	}

	var changed bool
	changed, err = applyOpts(nl, opts)
	if err != nil {
		log.WithError(err).Debug()
		return nil, err
	}

	if !new && changed {
		err = fmt.Errorf("vxlan interface already exists with wrong attributes")
		log.WithError(err).Debug()
		return nil, err
	}

	if new {
		err = netlink.LinkAdd(nl)
		if err != nil {
			if retry { // try again, in case another thread already brought it up
				log.WithError(err).Debug("retrying")
				return newVxlan(vxlanName, opts, false)
			}
			log.WithError(err).Debug("not retrying")
			return nil, err
		}
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
			if hardwareAddr.String() == nl.HardwareAddr.String() {
				break
			}
			err = netlink.LinkSetHardwareAddr(nl, hardwareAddr)
		case "vxlanmtu":
			var mtu int
			mtu, err = strconv.Atoi(v)
			if err != nil {
				break
			}
			if mtu == nl.MTU {
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

	return v, nil
}

// FromName gets a vxlan interface by name
func FromName(name string) (*Vxlan, error) {
	v := new(name)
	log := v.log.WithField("Func", "FromName()")
	log.Debug()

	_, err := v.nl()
	if err != nil {
		log.WithError(err).Debug()
	}

	return v, nil
}

// CreateMacvlan creates a macvlan as a slave to v
func (v *Vxlan) CreateMacvlan(name string) (*macvlan.Macvlan, error) {
	log := v.log.WithField("Func", "CreateMacvlan()")
	log.Debug()

	nl, err := v.nl()
	if err != nil {
		log.WithError(err).Debug()
		return nil, err
	}

	return macvlan.New(name, nl.LinkAttrs.Index)
}

// DeleteMacvlan deletes the slave macvlan interface by name
func (v *Vxlan) DeleteMacvlan(name string) error {
	log := v.log.WithField("Func", "DeleteMacvlan()")
	log.Debug()

	nl, err := v.nl()
	if err != nil {
		log.WithError(err).Debug()
		return err
	}

	mvl, err := macvlan.FromName(name)
	if err != nil {
		log.WithError(err).Debug()
		return err
	}

	if nl.Index != mvl.GetParentIndex() {
		return fmt.Errorf("macvlan is not a child of this vxlan")
	}

	return mvl.Delete()
}

// Delete deletes the vxlan interface.
// Any child macvlans will automatically be deleted by the kernel.
func (v *Vxlan) Delete() error {
	log := v.log.WithField("Func", "Delete()")
	log.Debug()

	nl, err := v.nl()
	if err != nil {
		log.WithError(err).Debug("link doesn't exist, nothing to delete")
		return nil
	}

	return netlink.LinkDel(nl)
}

// GetMacVlans returns all slave macvlan interfaces
func (v *Vxlan) GetMacVlans() ([]*macvlan.Macvlan, error) {
	log := v.log.WithField("Func", "GetMacVlans()")
	log.Debug()

	r := []*macvlan.Macvlan{}

	allSlaves, err := v.GetSlaveDevices()
	if err != nil {
		return r, err
	}

	var mvl *macvlan.Macvlan
	for _, link := range allSlaves {
		mvl, err = macvlan.FromLink(link)
		if err != nil {
			continue
		}
		r = append(r, mvl)
	}
	return r, nil
}

// GetSlaveDevices gets all slave devices, including macvlans, but possibly others
func (v *Vxlan) GetSlaveDevices() ([]netlink.Link, error) {
	log := v.log.WithField("Func", "GetSlaveDevices()")
	log.Debug()

	nl, err := v.nl()
	if err != nil {
		log.WithError(err).Debug()
		return nil, err
	}

	r := []netlink.Link{}

	allLinks, err := netlink.LinkList()
	if err != nil {
		log.WithError(err).Debug("failed to get all links")
		return r, err
	}

	for _, link := range allLinks {
		if link.Attrs().MasterIndex != nl.Attrs().Index {
			continue
		}
		r = append(r, link)
	}
	return r, nil
}
