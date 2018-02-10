package hostInterface

import (
	"net"

	log "github.com/Sirupsen/logrus"

	"github.com/TrilliumIT/docker-vxrouter/macvlan"
	"github.com/TrilliumIT/docker-vxrouter/vxlan"
)

type HostInterface struct {
	name string
	vxl  *vxlan.Vxlan
	mvl  *macvlan.Macvlan
	log  *log.Entry
}

func NewHostInterface(name string, gateway *net.IPNet, opts map[string]string) (*HostInterface, error) {
	log := log.WithField("HostInterface", name)
	log.Debug("NewHostInterface")
	vxl, err := vxlan.NewVxlan(name, opts)
	if err != nil {
		log.WithError(err).Debug("failed to create vxlan")
		return nil, err
	}

	mvl, err := vxl.CreateMacvlan("hmvl_" + name)
	if err != nil {
		log.WithError(err).Debug("failed to create host macvlan")
		err2 := vxl.Delete()
		if err2 != nil {
			log.WithError(err).WithError(err2).Debug("failed to delete vxlan")
			return nil, err2
		}
		return nil, err
	}

	err = mvl.AddAddress(gateway)
	if err != nil {
		log.WithError(err).Debug("failed to add address to macvlan")
		//implicitly deletes macvlan
		err2 := vxl.Delete()
		if err2 != nil {
			log.WithError(err).WithError(err2).Debug("failed to delete vxlan")
			return nil, err2
		}
		return nil, err
	}

	hi := &HostInterface{
		name,
		vxl,
		mvl,
		log,
	}

	return hi, nil
}

func GetHostInterface(name string) (*HostInterface, error) {
	log := log.WithField("HostInterface", name)
	log.Debug("GetHostInterface")

	vxl, err := vxlan.GetVxlan(name)
	if err != nil {
		log.Debug("failed to get vxlan interface")
		return nil, err
	}

	mvl, err := macvlan.GetMacvlan("hmvl_" + name)
	if err != nil {
		log.Debug("failed to get macvlan interface")
		return nil, err
	}

	hi := &HostInterface{
		name,
		vxl,
		mvl,
		log,
	}

	return hi, nil
}

func GetOrCreateHostInterface(name string, gateway *net.IPNet, opts map[string]string) (*HostInterface, error) {
	log := log.WithField("HostInterface", name)
	log.Debug("GetOrCreateHostInterface")
	hi, err := GetHostInterface(name)
	if err == nil {
		return hi, nil
	}

	return NewHostInterface(name, gateway, opts)
}

func (hi *HostInterface) CreateMacvlan(name string) error {
	hi.log.WithField("Macvlan", name).Debug("CreateMacvlan")
	_, err := hi.vxl.CreateMacvlan(name)
	return err
}

func (hi *HostInterface) DeleteMacvlan(name string) error {
	hi.log.WithField("Macvlan", name).Debug("DeleteMacvlan")
	return hi.vxl.DeleteMacvlan(name)
}

func (hi *HostInterface) isEmpty() bool {
	return true
}
