package core

import (
	log "github.com/Sirupsen/logrus"
	"github.com/docker/docker/api/types"
)

type getNr struct {
	s  string
	rc chan<- *types.NetworkResource
}

func nrCacheLoop(getNr <-chan *getNr, delNr <-chan string, putNr <-chan *types.NetworkResource) {
	nrCache := make(map[string]*types.NetworkResource)
	for {
		select {
		case rc := <-getNr:
			rc.rc <- nrCache[rc.s]
		case dn := <-delNr:
			nr := nrCache[dn]
			if nr == nil {
				break
			}
			delete(nrCache, nr.ID)
			pool, err := poolFromNR(nr)
			if err != nil {
				log.Debug("failed to get pool from network resource, not deleting")
				break
			}
			delete(nrCache, pool)
		case nr := <-putNr:
			nrCache[nr.ID] = nr
			pool, err := poolFromNR(nr)
			if err != nil {
				log.Debug("failed to get pool from network resource, not caching")
				break
			}
			nrCache[pool] = nr
		}
	}
}

func (c *Core) getNrFromCache(s string) *types.NetworkResource {
	rc := make(chan *types.NetworkResource)
	c.getNr <- &getNr{s, rc}
	return <-rc
}

func (c *Core) putNrInCache(nr *types.NetworkResource) {
	c.putNr <- nr
}

func (c *Core) delNrInCache(s string) {
	c.delNr <- s
}
