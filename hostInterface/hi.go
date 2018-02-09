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
}

func NewHostInterface(name string, gateway *net.IPNet, opts map[string]string) (*HostInterface, error) {
	vxl, err := vxlan.NewVxlan(name, opts)
	if err != nil {
		log.WithError(err).Errorf("failed to create vxlan for HostInterface")
		return nil, err
	}

	mvl, err := vxl.CreateMacvlan("hmvl_" + name)
	if err != nil {
		log.WithError(err).Errorf("failed to add macvlan to vxlan")
		err2 := vxl.Delete()
		if err2 != nil {
			log.WithError(err2).Errorf("your kernel is effed up, bro")
			return nil, err2
		}
		return nil, err
	}

	err = mvl.AddAddress(gateway)
	if err != nil {
		log.WithError(err).Errorf("failed to add address to macvlan")
		//implicitly deletes macvlan
		err2 := vxl.Delete()
		if err2 != nil {
			log.WithError(err2).Errorf("your kernel is effed up, bro")
			return nil, err2
		}
		return nil, err
	}

	hi := &HostInterface{
		name,
		vxl,
		mvl,
	}

	return hi, nil
}

func GetHostInterface(name string) (*HostInterface, error) {
	vxl, err := vxlan.GetVxlan(name)
	if err != nil {
		log.WithError(err).Errorf("failed to get vxlan link by name %v", name)
		return nil, err
	}
	mvl, err := macvlan.GetMacvlan("hmvl_" + name)
	if err != nil {
		log.WithError(err).Errorf("failed to get macvlan link by name %v", "hmvl_"+name)
		return nil, err
	}

	hi := &HostInterface{
		name,
		vxl,
		mvl,
	}

	return hi, nil
}

func (hi *HostInterface) CreateMacvlan(name string) error {
	_, err := hi.vxl.CreateMacvlan(name)
	return err
}

func (hi *HostInterface) isEmpty() bool {
	return true
}
