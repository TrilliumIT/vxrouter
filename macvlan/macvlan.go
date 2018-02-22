package macvlan

import (
	"fmt"
	"net"

	log "github.com/Sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

// Macvlan is a macvlan interface, for either a host or a container
type Macvlan struct {
	nl  *netlink.Macvlan
	log *log.Entry
}

// NewMacvlan creates a macvlan interface, under the parent interface index
func NewMacvlan(name string, parent int) (*Macvlan, error) {
	log := log.WithField("Macvlan", name)
	log.Debug("NewMacvlan")
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
		return nil, err
	}

	if err := netlink.LinkSetUp(nl); err != nil {
		log.WithError(err).Debug("failed to bring up macvlan")
		return nil, err
	}
	log.Debug("Brought up macvlan")

	return &Macvlan{nl, log}, nil
}

// FromName returns a Macvlan from an interface name
// nolint: dupl
func FromName(name string) (*Macvlan, error) {
	log := log.WithField("Macvlan", name)
	log.Debug("FromName")
	link, err := netlink.LinkByName(name)
	if err != nil {
		log.WithError(err).Debug("failed to get link by name")
		return nil, err
	}

	return FromLink(link)
}

// FromIndex returns a Macvlan from an interface index
// nolint: dupl
func FromIndex(index int) (*Macvlan, error) {
	log := log.WithField("Macvlan", index)
	log.Debug("FromIndex")
	link, err := netlink.LinkByIndex(index)
	if err != nil {
		log.WithError(err).Debug("failed to get link by index")
		return nil, err
	}

	return FromLink(link)
}

// FromLink returns a Macvlan from an interface index
// nolint dupl
func FromLink(link netlink.Link) (*Macvlan, error) {
	log := log.WithField("Macvlan", link.Attrs().Name)
	log.Debug("FromLink")
	if nl, ok := link.(*netlink.Macvlan); ok {
		return &Macvlan{nl, log}, nil
	}

	err := fmt.Errorf("link is not a macvlan")
	log.WithError(err).Debug()
	return nil, err
}

// Equals determines if m and m2 are the same interface
func (m *Macvlan) Equals(m2 *Macvlan) bool {
	return m.nl.Attrs().Index == m2.nl.Attrs().Index
}

// AddAddress adds an ip address to a Macvlan interface
func (m *Macvlan) AddAddress(addr *net.IPNet) error {
	m.log.Debug("AddAddress")
	return netlink.AddrAdd(m.nl, &netlink.Addr{IPNet: addr})
}

// Delete deletes a Macvlan interface
func (m *Macvlan) Delete() error {
	m.log.Debug("Delete")

	// verify a parent interface isn't being deleted
	if m.nl.Attrs().ParentIndex == 0 {
		err := fmt.Errorf("interface is not a slave")
		m.log.WithError(err).Error()
		return err
	}

	// delete the macvlan slave device
	return netlink.LinkDel(m.nl)
}

// GetAddresses returns IP Addresses on a Macvlan interface
func (m *Macvlan) GetAddresses() ([]*net.IPNet, error) {
	addrs, err := netlink.AddrList(m.nl, 0)
	if err != nil {
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
	addrs, err := m.GetAddresses()
	if err != nil {
		log.WithError(err).Warn("err getting macvlan addresses")
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
	m.log.Debug("GetParentIndex")
	return m.nl.Attrs().ParentIndex
}

// GetIndex returns the index of the interface
func (m *Macvlan) GetIndex() int {
	m.log.Debug("GetIndex")
	return m.nl.Attrs().Index
}
