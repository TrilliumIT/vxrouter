package core

import (
	log "github.com/Sirupsen/logrus"

	"context"

	"github.com/TrilliumIT/vxrouter/host"
	"github.com/docker/docker/api/types"
)

// Reconcile adds missing routes and deletes orphaned routes
func (c *Core) Reconcile() {
	log := log.WithField("func", "DelOrphanedRoutes()")

	// This is possibly racy, if a container starts up after containers are listed
	// I might delete it's routes
	es, err := c.getContainerIPsAndSubnets()
	if err != nil {
		log.WithError(err).Error("Error getting container IPs")
		return
	}

	// Make sure all containers are connected
	for ip, subnet := range es {
		err = c.connectIfNotConnected(ip, subnet)
		if err != nil {
			log.WithError(err).Error("Error connecting container")
		}
	}

	// remove errant routes
	nets, err := host.AllVxRoutes()
	if err != nil {
		log.WithError(err).Error("Error getting routes")
		return
	}

	for _, n := range nets {
		if _, ok := es[n.String()]; ok {
			continue
		}
		err = c.DeleteRoute(n.String())
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
			if es.IPAMConfig.IPv4Address != "" {
				ret[es.IPAMConfig.IPv4Address] = es.NetworkID
			}
		}
	}
	return ret, nil
}
