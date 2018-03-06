package core

import (
	log "github.com/Sirupsen/logrus"

	"context"
	"net"

	"github.com/TrilliumIT/vxrouter/host"
	"github.com/docker/docker/api/types"
)

// Reconcile adds missing routes and deletes orphaned routes
func (c *Core) Reconcile() {
	log := log.WithField("func", "Reconcile()")

	// This is possibly racy, if a container starts up after containers are listed
	// I might delete it's routes
	es, err := c.getContainerIPsAndSubnets()
	if err != nil {
		log.WithError(err).Error("Error getting container IPs")
		return
	}

	// Make sure all containers are connected
	for ip, subnet := range es {
		var connected bool
		connected, err = c.connectIfNotConnected(ip, subnet)
		if err != nil {
			log.WithError(err).Error("Error connecting container")
		}
		if connected {
			log.WithField("ip", ip).Debug("added missing route")
		}
	}

	// remove errant routes
	nets, err := host.AllVxRoutes()
	if err != nil {
		log.WithError(err).Error("Error getting routes")
		return
	}

	for _, n := range nets {
		if _, ok := es[n.IP.String()]; ok {
			continue
		}
		log.WithField("IP", n.IP.String()).Debug("Deleting orphaned Route")
		err = c.deleteRoute(n.IP)
		if err != nil {
			log.WithError(err).Error("error deleting orphaned route")
		}
	}

	es2, err := c.getContainerIPsAndSubnets()
	if err != nil {
		log.WithError(err).Error("Error getting final container IPs")
		return
	}

	if !ipListsEqual(es, es2) {
		// Container IPs changed while running reconcile, we better run it again
		c.Reconcile()
	}
}

func ipListsEqual(m map[string]string, m2 map[string]string) bool {
	if len(m) != len(m2) {
		return false
	}

	for k := range m {
		if m[k] != m2[k] {
			return false
		}
	}

	return true
}

func (c *Core) getContainerIPsAndSubnets() (map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dockerTimeout)
	defer cancel()

	ctrs, err := c.dc.ContainerList(ctx, types.ContainerListOptions{})
	if err != nil {
		return nil, err
	}

	ret := make(map[string]string)
	for _, ctr := range ctrs {
		for _, es := range ctr.NetworkSettings.Networks {
			if es.IPAMConfig == nil {
				continue
			}
			// This is necessary because docker is stupid, this could be
			// "10.1.141.01" for example
			ip := net.ParseIP(es.IPAMConfig.IPv4Address)
			if ip != nil {
				ret[ip.String()] = es.NetworkID
				log.WithField("Container", ctr.Names[0]).WithField("net", es.NetworkID).
					WithField("ip", ip.String()).Debug("Appending to es list")
			}

			ip = net.ParseIP(es.IPAddress)
			if ip != nil {
				ret[ip.String()] = es.NetworkID
				log.WithField("Container", ctr.Names[0]).WithField("net", es.NetworkID).
					WithField("ip", ip.String()).Debug("Appending to es list")
			}
		}
	}
	return ret, nil
}
