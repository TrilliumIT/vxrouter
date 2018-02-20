package network

import (
	"fmt"
	"net"
	"strconv"
	"sync"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types"
	gphnet "github.com/docker/go-plugins-helpers/network"

	"github.com/TrilliumIT/vxrouter/docker/core"
	"github.com/TrilliumIT/vxrouter/host"
)

const (
	// DriverName is the docker plugin name of the driver
	DriverName = "vxrNet"
)

// Driver is a vxrouter network driver
type Driver struct {
	scope       string
	core        *core.Core
	log         *log.Entry
	nrCache     map[string]*types.NetworkResource
	nrCacheLock *sync.RWMutex
}

// NewDriver creates a new Driver
func NewDriver(scope string, core *core.Core) (*Driver, error) {
	d := &Driver{
		scope,
		core,
		log.WithField("driver", DriverName),
		make(map[string]*types.NetworkResource),
		&sync.RWMutex{},
	}
	return d, nil
}

// GetCapabilities is called on driver initialization
func (d *Driver) GetCapabilities() (*gphnet.CapabilitiesResponse, error) {
	d.log.Debug("GetCapabilities()")
	cap := &gphnet.CapabilitiesResponse{
		Scope:             d.scope,
		ConnectivityScope: "",
	}
	return cap, nil
}

// CreateNetwork is called on docker network create
func (d *Driver) CreateNetwork(r *gphnet.CreateNetworkRequest) error {
	d.log.WithField("r", r).Debug("CreateNetwork()")

	hasGW := false
	for _, v4 := range append(r.IPv4Data, r.IPv6Data...) {
		if v4.Gateway != "" {
			hasGW = true
			break
		}
	}

	if !hasGW {
		err := fmt.Errorf("gateway not found in IPAMData")
		d.log.WithError(err).Error()
		return err
	}

	opts, ok := r.Options["com.docker.network.generic"].(map[string]interface{})
	if !ok {
		err := fmt.Errorf("did not retrieve the options array for the network")
		d.log.WithError(err).Error()
		return err
	}

	vxlID, ok := opts["vxlanid"]
	if !ok {
		err := fmt.Errorf("cannot create a network without a vxlanid (-o vxlanid=<0-16777215>)")
		d.log.WithError(err).Error()
		return err
	}

	vid, err := strconv.Atoi(vxlID.(string))
	if err != nil {
		d.log.WithError(err).WithField("vxlanid", vxlID.(string)).Errorf("failed to parse vxlanid")
		return err
	}
	if vid < 0 || vid > 16777215 {
		err = fmt.Errorf("vxlanid is out of range")
		d.log.WithField("vxlanid", vid).WithError(err).Error()
		return err
	}

	return nil
}

// AllocateNetwork is never called
func (d *Driver) AllocateNetwork(r *gphnet.AllocateNetworkRequest) (*gphnet.AllocateNetworkResponse, error) {
	d.log.WithField("r", r).Debug("AllocateNetwork()")
	return &gphnet.AllocateNetworkResponse{}, nil
}

// DeleteNetwork is called on docker network rm
func (d *Driver) DeleteNetwork(r *gphnet.DeleteNetworkRequest) error {
	d.log.WithField("r", r).Debug("DeleteNetwork()")
	return nil
}

// FreeNetwork is never called
func (d *Driver) FreeNetwork(r *gphnet.FreeNetworkRequest) error {
	d.log.WithField("r", r).Debug("FreeNetwork()")
	return nil
}

// CreateEndpoint is called after IPAM has assigned an address, before Join is called
func (d *Driver) CreateEndpoint(r *gphnet.CreateEndpointRequest) (*gphnet.CreateEndpointResponse, error) {
	d.log.WithField("r", r).Debug("CreateEndpoint()")

	return &gphnet.CreateEndpointResponse{}, nil
}

// DeleteEndpoint is called after Leave
func (d *Driver) DeleteEndpoint(r *gphnet.DeleteEndpointRequest) error {
	d.log.WithField("r", r).Debug("DeleteEndpoint()")

	//TODO: move to core func

	nr, err := d.core.GetNetworkResourceByID(r.NetworkID)
	if err != nil {
		d.log.WithError(err).WithField("NetworkID", r.NetworkID).Error("failed to get network resource")
		return err
	}

	hi, err := host.GetInterface(nr.Name)
	if err != nil {
		return err
	}

	mvlName := "cmvl_" + r.EndpointID[:7]
	err = hi.DeleteMacvlan(mvlName)
	if err != nil {
		d.log.WithError(err).Error("failed to delete macvlan for container")
		return err
	}

	containers, err := d.core.GetContainers()
	if err != nil {
		d.log.WithError(err).Error("failed to list containers")
		return err
	}

	delHi := true
	for _, c := range containers {
		ns, ok := c.NetworkSettings.Networks[nr.Name]
		if !ok {
			continue
		}

		if ns.EndpointID != r.EndpointID {
			d.log.Debug("other containers are still running on this network")
			delHi = false
			continue
		}

		if err := hi.DelRoute(net.ParseIP(ns.IPAddress)); err != nil {
			d.log.WithError(err).Debug("failed to delete route")
			return err
		}
		if !delHi {
			break
		}
	}

	if delHi {
		return hi.Delete()
	}

	return nil
}

// EndpointInfo is called on inspect... maybe?
func (d *Driver) EndpointInfo(r *gphnet.InfoRequest) (*gphnet.InfoResponse, error) {
	d.log.WithField("r", r).Debug("EndpointInfo()")
	return &gphnet.InfoResponse{}, nil
}

// Join is the last thing called before the nic is put into the container namespace
func (d *Driver) Join(r *gphnet.JoinRequest) (*gphnet.JoinResponse, error) {
	nr, err := d.client.GetNetworkResourceByID(r.NetworkID)
	if err != nil {
		d.log.WithError(err).WithField("NetworkID", r.NetworkID).Error("failed to get network resource")
		return nil, err
	}

	hi, err := host.GetInterface(nr.Name)
	if err != nil {
		return nil, err
	}

	mvlName := "cmvl_" + r.EndpointID[:7]
	err = hi.CreateMacvlan(mvlName)
	if err != nil {
		d.log.WithError(err).Error("failed to create macvlan for container")
		return nil, err
	}

	gw, err := d.getGateway(r.NetworkID)
	if err != nil {
		d.log.WithError(err).Error("failed to get gateway")
		return nil, err
	}

	jr := &gphnet.JoinResponse{
		InterfaceName: gphnet.InterfaceName{
			SrcName:   mvlName,
			DstPrefix: "eth",
		},
		Gateway: gw.IP.String(),
	}

	return jr, nil
}

// Leave is the first thing called on container stop
func (d *Driver) Leave(r *gphnet.LeaveRequest) error {
	d.log.WithField("r", r).Debug("Leave()")
	return nil
}

// DiscoverNew is not implemented by this driver
func (d *Driver) DiscoverNew(r *gphnet.DiscoveryNotification) error {
	d.log.WithField("r", r).Debug("DiscoverNew()")
	return nil
}

// DiscoverDelete is not implemented by this driver
func (d *Driver) DiscoverDelete(r *gphnet.DiscoveryNotification) error {
	d.log.WithField("r", r).Debug("DiscoverDelete()")
	return nil
}

// ProgramExternalConnectivity is not implemented by this driver
func (d *Driver) ProgramExternalConnectivity(r *gphnet.ProgramExternalConnectivityRequest) error {
	d.log.WithField("r", r).Debug("ProgramExternalConnectivity()")
	return nil
}

// RevokeExternalConnectivity is not implemented by this driver
func (d *Driver) RevokeExternalConnectivity(r *gphnet.RevokeExternalConnectivityRequest) error {
	d.log.WithField("r", r).Debug("RevokeExternalConnectivity()")

	return nil
}
