package core

import (
	log "github.com/Sirupsen/logrus"

	"context"
	"net"
	"sync"

	"github.com/TrilliumIT/vxrouter/host"
	"github.com/docker/docker/api/types"
)

// Reconcile adds missing routes and deletes orphaned routes
func (c *Core) Reconcile() {
	log := log.WithField("func", "Reconcile()")

	// This is possibly racy, if a container starts up after containers are listed
	// I might delete it's routes
	// To compensate for this, I compare es before and after the run, if it's changed, run again immediately
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

	orphanedInts := make(map[string]*host.Interface)
	for _, n := range nets {
		if _, ok := es[n.IP.String()]; ok {
			continue
		}
		// This MUST only delete the route, not call hi.Delete(), because if the race condition triggered
		// and another container started up, and I just deleted it's route, hi.Delete() will delete the vxlan
		// interface that is the master of the slave container interface. There will be no way to recover except by
		// restarting the container.
		// Store the deleted routes so we can call hi.delete() on them only if es hasn't changed at the end of this function.
		log.WithField("IP", n.IP.String()).Debug("Deleting orphaned Route")
		var hi *host.Interface
		hi, err = c.deleteRoute(n.IP)
		if err != nil {
			log.WithError(err).Error("error deleting orphaned route")
			continue
		}
		orphanedInts[hi.Name()] = hi
	}

	es2, err := c.getContainerIPsAndSubnets()
	if err != nil {
		log.WithError(err).Error("Error getting final container IPs")
		return
	}

	if !ipListsEqual(es, es2) {
		// Container IPs changed while running reconcile, we better run it again
		c.Reconcile()
		return
	}

	// nothing changed, we can call hi.delete on all the orphaned routes
	hiDelWg := sync.WaitGroup{}
	for _, hi := range orphanedInts {
		hiDelWg.Add(1)
		go func(hi *host.Interface) {
			defer hiDelWg.Done()
			if err = hi.Delete(); err != nil {
				log.WithError(err).Error("error while deleting host interface")
			}
		}(hi)
	}

	hiDelWg.Wait()
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
			// This is necessary because docker is stupid, this could be
			// "10.1.141.01" for example
			ip := net.ParseIP(es.IPAddress)
			if ip != nil {
				ret[ip.String()] = es.NetworkID
				log.WithField("Container", ctr.Names[0]).WithField("net", es.NetworkID).
					WithField("ip", ip.String()).Debug("Appending to es list")
			}

			if es.IPAMConfig == nil {
				continue
			}
			ip = net.ParseIP(es.IPAMConfig.IPv4Address)
			if ip != nil {
				ret[ip.String()] = es.NetworkID
				log.WithField("Container", ctr.Names[0]).WithField("net", es.NetworkID).
					WithField("ip", ip.String()).Debug("Appending to es list")
			}
		}
	}
	return ret, nil
}
