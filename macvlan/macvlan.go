package macvlan

import (
	"fmt"
	"net"

	log "github.com/Sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

// Macvlan is a macvlan interface, for either a host or a container
type Macvlan struct {
	name string
	log  *log.Entry
}

func new(name string) *Macvlan {
	log := log.WithField("Macvlan", name)
	log.WithField("Func", "new()").Debug()
	return &Macvlan{name, log}
}

func (m *Macvlan) nl() (*netlink.Macvlan, error) {
	log := m.log.WithField("Func", "nl()")
	log.Debug()

	link, err := netlink.LinkByName(m.name)
	if err != nil {
		log.WithError(err).Debug("failed to get link by name")
		return nil, err
	}

	return checkNl(link)
}

func checkNl(link netlink.Link) (*netlink.Macvlan, error) {
	if nl, ok := link.(*netlink.Macvlan); ok {
		return nl, nil
	}

	return nil, fmt.Errorf("link is not a macvlan")
}

// NewMacvlan creates a macvlan interface, under the parent interface index
func NewMacvlan(name string, parent int) (*Macvlan, error) {
	m := new(name)
	log := m.log.WithField("Func", "NewMacvlan()")
	log.Debug()

	// Create a macvlan link
	nl := &netlink.Macvlan{
		LinkAttrs: netlink.LinkAttrs{
			Name:        name,
			ParentIndex: parent,
		},
		Mode: netlink.MACVLAN_MODE_BRIDGE,
	}
	if err := netlink.LinkAdd(nl); err != nil {
		log.WithError(err).Debug("error adding link")

		// Just in case add failed due to add succeeding from another thread
		var err2 error
		nl, err2 = m.nl()
		if err2 != nil { // add and get failed, return first error
			return nil, err
		}
	}

	if err := netlink.LinkSetUp(nl); err != nil {
		log.WithError(err).Debug("failed to bring up macvlan")
		return nil, err
	}
	log.Debug("Brought up macvlan")

	return m, nil
}

// FromName returns a Macvlan from an interface name
// nolint: dupl
func FromName(name string) (*Macvlan, error) {
	m := new(name)
	log := m.log.WithField("Func", "FromName()")
	log.Debug()

	_, err := m.nl()
	if err != nil {
		return nil, err
	}
	return m, nil
}

// FromLink returns a Macvlan from an interface link
// nolint: dupl
func FromLink(link netlink.Link) (*Macvlan, error) {
	m := new(link.Attrs().Name)
	log := m.log.WithField("Func", "FromLink()")
	log.Debug()

	_, err := checkNl(link)
	if err != nil {
		log.WithError(err).Debug()
		return nil, err
	}
	return m, nil
}

// AddAddress adds an ip address to a Macvlan interface
func (m *Macvlan) AddAddress(addr *net.IPNet) error {
	log := m.log.WithField("Func", "AddAddress()")
	log.Debug()

	nl, err := m.nl()
	if err != nil {
		log.WithError(err).Debug()
		return err
	}
	return netlink.AddrAdd(nl, &netlink.Addr{IPNet: addr})
}

// Delete deletes a Macvlan interface
func (m *Macvlan) Delete() error {
	log := m.log.WithField("Func", "Delete()")
	log.Debug()

	nl, err := m.nl()
	if err != nil {
		log.WithError(err).Debug("link doesn't exist, nothing to delete")
		return nil
	}

	// verify a parent interface isn't being deleted
	if nl.Attrs().ParentIndex == 0 {
		err := fmt.Errorf("interface is not a slave")
		log.WithError(err).Debug()
		return err
	}

	// delete the macvlan slave device
	return netlink.LinkDel(nl)
}

// GetAddresses returns IP Addresses on a Macvlan interface
func (m *Macvlan) GetAddresses() ([]*net.IPNet, error) {
	log := m.log.WithField("Func", "GetAddresses()")
	log.Debug()

	nl, err := m.nl()
	if err != nil {
		log.WithError(err).Debug()
		return nil, err
	}

	addrs, err := netlink.AddrList(nl, 0)
	if err != nil {
		log.WithError(err).Debug()
		return nil, err
	}
	r := []*net.IPNet{}
	for _, a := range addrs {
		r = append(r, a.IPNet)
	}
	return r, nil
}

// HasAddress returns true if addr is bound to the Macvlan interface
func (m *Macvlan) HasAddress(addr *net.IPNet) bool {
	log := m.log.WithField("Func", "HasAddress()")
	log.Debug()

	addrs, err := m.GetAddresses()
	if err != nil {
		log.WithError(err).Debug()
	}

	for _, a := range addrs {
		if a.IP.Equal(addr.IP) && a.Mask.String() == addr.Mask.String() {
			return true
		}
	}

	return false
}

// GetParentIndex returns the index of the parent interface
func (m *Macvlan) GetParentIndex() int {
	log := m.log.WithField("Func", "GetParentIndex()")
	log.Debug()

	nl, err := m.nl()
	if err != nil {
		log.WithError(err).Debug()
		return 0
	}
	return nl.Attrs().ParentIndex
}

// GetIndex returns the index of the interface
func (m *Macvlan) GetIndex() int {
	log := m.log.WithField("Func", "GetParentIndex()")
	log.Debug()

	nl, err := m.nl()
	if err != nil {
		log.WithError(err).Debug()
		return 0
	}
	return nl.Attrs().Index
}
