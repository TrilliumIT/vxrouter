package macvlan

import (
	"fmt"
	"net"

	log "github.com/Sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

type Macvlan struct {
	nl *netlink.Macvlan
}

func NewMacvlan(name string, parent int) (*Macvlan, error) {
	log.Debugf("Creating new macvlan: %v", name)
	// Create a macvlan link
	nl := &netlink.Macvlan{
		LinkAttrs: netlink.LinkAttrs{
			Name:        name,
			ParentIndex: parent,
		},
		Mode: netlink.MACVLAN_MODE_BRIDGE,
	}
	if err := netlink.LinkAdd(nl); err != nil {
		log.WithError(err).Errorf("Error adding link: %v", err)
		return nil, err
	}

	return &Macvlan{nl}, nil
}

func GetMacvlan(name string) (*Macvlan, error) {
	link, err := netlink.LinkByName(name)
	if err != nil {
		log.WithError(err).Errorf("failed to get macvlan link by name %v", name)
		return nil, err
	}

	if nl, ok := link.(*netlink.Macvlan); ok {
		return &Macvlan{nl}, nil
	}

	return nil, fmt.Errorf("link %v was not a macvlan", name)
}

func (m *Macvlan) AddAddress(addr *net.IPNet) error {
	return netlink.AddrAdd(m.nl, &netlink.Addr{IPNet: addr})
}

func (m *Macvlan) Delete() error {
	name := m.nl.LinkAttrs.Name
	log.Debugf("deleting macvlan: %s", name)

	// verify a parent interface isn't being deleted
	if m.nl.Attrs().ParentIndex == 0 {
		err := fmt.Errorf("interface (%v) does not appear to be a slave interface", name)
		log.WithError(err).Error()
		return err
	}

	// delete the macvlan slave device
	return netlink.LinkDel(m.nl)
}
