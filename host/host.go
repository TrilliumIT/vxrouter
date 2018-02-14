package host

import (
	"fmt"
	"net"

	log "github.com/Sirupsen/logrus"

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
