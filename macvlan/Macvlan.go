package macvlan

import (
	"fmt"
	"net"

	log "github.com/Sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

type Macvlan struct {
	nl  *netlink.Macvlan
	log *log.Entry
}

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

	return &Macvlan{nl, log}, nil
}

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

func (m *Macvlan) Equals(m2 *Macvlan) bool {
	return m.nl.Attrs().Index == m2.nl.Attrs().Index
}

func (m *Macvlan) AddAddress(addr *net.IPNet) error {
	m.log.Debug("AddAddress")
	return netlink.AddrAdd(m.nl, &netlink.Addr{IPNet: addr})
}

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

func (m *Macvlan) GetParentIndex() int {
	m.log.Debug("GetParentIndex")
	return m.nl.Attrs().ParentIndex
}
