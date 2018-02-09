package vxlan

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/vishvananda/netlink"

	"github.com/TrilliumIT/docker-vxrouter/macvlan"
)

const (
	ENVVAR_PREFIX = "VXR_"
)

type Vxlan struct {
	nl *netlink.Vxlan
}

func NewVxlan(vxlanName string, opts map[string]string) (*Vxlan, error) {
	var ok bool
	keys := [...]string{"vxlanmtu", "vxlanhardwareaddr", "vxlantxqlen", "vxlanid", "vtepdev", "srcaddr", "group", "ttl", "tos", "learning", "proxy", "rsc", "l2miss", "l3miss", "noage", "gbp", "age", "limit", "port", "portlow", "porthigh", "vxlanhardwareaddr", "vxlanmtu"}

	for _, k := range keys {
		if _, ok = opts[k]; !ok && os.Getenv(ENVVAR_PREFIX+k) != "" {
			opts[k] = os.Getenv(ENVVAR_PREFIX + k)
		}
	}

	nl := &netlink.Vxlan{
		LinkAttrs: netlink.LinkAttrs{
			Name: vxlanName,
		},
	}

	// Parse interface options
	for k, v := range opts {
		switch strings.ToLower(k) {
		case "vxlanmtu":
			MTU, err := strconv.Atoi(v)
			if err != nil {
				log.Errorf("Error converting MTU to int: %v", err)
				return nil, err
			}
			nl.LinkAttrs.MTU = MTU
		case "vxlanhardwareaddr":
			HardwareAddr, err := net.ParseMAC(v)
			if err != nil {
				log.Errorf("Error parsing mac: %v", err)
				return nil, err
			}
			nl.LinkAttrs.HardwareAddr = HardwareAddr
		case "vxlantxqlen":
			TxQLen, err := strconv.Atoi(v)
			if err != nil {
				log.Errorf("Error converting TxQLen to int: %v", err)
				return nil, err
			}
			nl.LinkAttrs.TxQLen = TxQLen
		case "vxlanid":
			VxlanID, err := strconv.ParseInt(v, 0, 32)
			if err != nil {
				log.Errorf("Error converting VxlanId to int: %v", err)
				return nil, err
			}
			nl.VxlanId = int(VxlanID)
		case "vtepdev":
			vtepDev, err := netlink.LinkByName(v)
			if err != nil {
				log.Errorf("Error getting VtepDev interface: %v", err)
				return nil, err
			}
			nl.VtepDevIndex = vtepDev.Attrs().Index
		case "srcaddr":
			nl.SrcAddr = net.ParseIP(v)
			if nl.SrcAddr == nil {
				err := fmt.Errorf("Failed to parse SrcAddr")
				log.WithError(err).Error()
				return nil, err
			}
		case "group":
			nl.Group = net.ParseIP(v)
			if nl.Group == nil {
				err := fmt.Errorf("Failed to parse Group")
				log.WithError(err).Error()
				return nil, err
			}
		case "ttl":
			TTL, err := strconv.Atoi(v)
			if err != nil {
				log.Errorf("Error converting TTL to int: %v", err)
				return nil, err
			}
			nl.TTL = TTL
		case "tos":
			TOS, err := strconv.Atoi(v)
			if err != nil {
				log.Errorf("Error converting TOS to int: %v", err)
				return nil, err
			}
			nl.TOS = TOS
		case "learning":
			Learning, err := strconv.ParseBool(v)
			if err != nil {
				log.Errorf("Error converting Learning to bool: %v", err)
				return nil, err
			}
			nl.Learning = Learning
		case "proxy":
			Proxy, err := strconv.ParseBool(v)
			if err != nil {
				log.Errorf("Error converting Proxy to bool: %v", err)
				return nil, err
			}
			nl.Proxy = Proxy
		case "rsc":
			RSC, err := strconv.ParseBool(v)
			if err != nil {
				log.Errorf("Error converting RSC to bool: %v", err)
				return nil, err
			}
			nl.RSC = RSC
		case "l2miss":
			L2miss, err := strconv.ParseBool(v)
			if err != nil {
				log.Errorf("Error converting L2miss to bool: %v", err)
				return nil, err
			}
			nl.L2miss = L2miss
		case "l3miss":
			L3miss, err := strconv.ParseBool(v)
			if err != nil {
				log.Errorf("Error converting L3miss to bool: %v", err)
				return nil, err
			}
			nl.L3miss = L3miss
		case "noage":
			NoAge, err := strconv.ParseBool(v)
			if err != nil {
				log.Errorf("Error converting NoAge to bool: %v", err)
				return nil, err
			}
			nl.NoAge = NoAge
		case "gbp":
			GBP, err := strconv.ParseBool(v)
			if err != nil {
				log.Errorf("Error converting GBP to bool: %v", err)
				return nil, err
			}
			nl.GBP = GBP
		case "age":
			Age, err := strconv.Atoi(v)
			if err != nil {
				log.Errorf("Error converting Age to int: %v", err)
				return nil, err
			}
			nl.Age = Age
		case "limit":
			Limit, err := strconv.Atoi(v)
			if err != nil {
				log.Errorf("Error converting Limit to int: %v", err)
				return nil, err
			}
			nl.Limit = Limit
		case "port":
			Port, err := strconv.Atoi(v)
			if err != nil {
				log.Errorf("Error converting Port to int: %v", err)
				return nil, err
			}
			nl.Port = Port
		case "portlow":
			PortLow, err := strconv.Atoi(v)
			if err != nil {
				log.Errorf("Error converting PortLow to int: %v", err)
				return nil, err
			}
			nl.PortLow = PortLow
		case "porthigh":
			PortHigh, err := strconv.Atoi(v)
			if err != nil {
				log.Errorf("Error converting PortHigh to int: %v", err)
				return nil, err
			}
			nl.PortHigh = PortHigh
		}
	}

	err := netlink.LinkAdd(nl)
	if err != nil {
		log.Errorf("Error adding vxlan interface: %v", err)
		return nil, err
	}

	// Parse interface options
	for k, v := range opts {
		switch strings.ToLower(k) {
		case "vxlanhardwareaddr":
			hardwareAddr, err := net.ParseMAC(v)
			if err != nil {
				log.Errorf("Error parsing mac address: %v", err)
				return nil, err
			}
			err = netlink.LinkSetHardwareAddr(nl, hardwareAddr)
			if err != nil {
				log.Errorf("Error setting mac address: %v", err)
				return nil, err
			}
		case "vxlanmtu":
			mtu, err := strconv.Atoi(v)
			if err != nil {
				log.Errorf("Error converting MTU to int: %v", err)
				return nil, err
			}
			err = netlink.LinkSetMTU(nl, mtu)
			if err != nil {
				log.Errorf("Error setting MTU: %v", err)
				return nil, err
			}
		}
	}

	// bring interfaces up
	err = netlink.LinkSetUp(nl)
	if err != nil {
		log.Errorf("Error bringing up vxlan: %v", err)
		return nil, err
	}

	return &Vxlan{nl}, nil
}

func GetVxlan(name string) (*Vxlan, error) {
	log.Debugf("vxlan.GetVxlan(%v)", name)

	link, err := netlink.LinkByName(name)
	if err != nil {
		return nil, err
	}

	if nl, ok := link.(*netlink.Vxlan); ok {
		return &Vxlan{nl}, nil
	}

	return nil, fmt.Errorf("link %v was not a vxlan", name)
}

func (v *Vxlan) CreateMacvlan(name string) (*macvlan.Macvlan, error) {
	return macvlan.NewMacvlan(name, v.nl.LinkAttrs.Index)
}

func (v *Vxlan) DeleteMacvlan(name string) error {
	//TODO: validate parent index

	mvl, err := macvlan.GetMacvlan(name)
	if err != nil {
		return err
	}

	if v.nl.Index != mvl.GetParentIndex() {
		return fmt.Errorf("tried to delete a macvlan for which I'm not the parent")
	}

	return mvl.Delete()
}

//host macvlan ind address are implicitely deleted when calling this
func (v *Vxlan) Delete() error {
	name := v.nl.LinkAttrs.Name
	log.Debugf("deleting vxlan: %s", name)

	return netlink.LinkDel(v.nl)
}
