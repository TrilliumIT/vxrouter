package host

import (
	"sync"
	"time"
)

var (
	getHlc chan *getHlReq
	delHlc chan string
)

func init() {
	getHlc = make(chan *getHlReq)
	delHlc = make(chan string)
	go hiLockLoop()
}

// This is a special rwlock which does not cause rlock to block if a lock is waiting.
// it allows new rlocks to be acquired, a full lock must wait for all of them to complete before it can run
type hiLock struct {
	l  sync.RWMutex
	wg sync.WaitGroup
}

func (hl *hiLock) lock() {
	hl.wg.Wait()
	hl.l.Lock()

	// In case wg.add called after we acquired a lock, that rlock is waiting, and new rlocks are being blocked.
	// Try to wait again to check. If this second wait blocks for more than 1ms, unlock and start over.
	c := make(chan struct{})
	go func() {
		hl.wg.Wait()
		close(c)
	}()
	t := time.NewTimer(1 * time.Millisecond)
	select {
	case <-c:
	case <-t.C:
		hl.l.Unlock()
		hl.lock()
	}
	t.Stop()
}

func (hl *hiLock) unlock() {
	hl.l.Unlock()
}

func (hl *hiLock) rlock() {
	hl.wg.Add(1)
	hl.l.RLock()
}

func (hl *hiLock) runlock() {
	hl.wg.Done()
	hl.l.RUnlock()
}

type getHlReq struct {
	s  string
	rc chan<- *hiLock
}

func hiLockLoop() {
	hlCache := make(map[string]*hiLock)
	for {
		select {
		case gh := <-getHlc:
			if _, ok := hlCache[gh.s]; !ok {
				hlCache[gh.s] = &hiLock{}
			}
			gh.rc <- hlCache[gh.s]
		case s := <-delHlc:
			delete(hlCache, s)
		}
	}
}

func getHl(s string) *hiLock {
	rc := make(chan *hiLock)
	getHlc <- &getHlReq{s, rc}
	return <-rc
}

func delHl(s string) {
	delHlc <- s
}
