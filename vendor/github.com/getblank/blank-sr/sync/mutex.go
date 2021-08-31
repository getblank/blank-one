package sync

import (
	"sync"

	log "github.com/sirupsen/logrus"
)

var (
	lockers         = map[string]*locker{}
	mutexLocker     sync.Mutex
	owners2Lockers  = map[string][]string{}
	lockersCounters = map[string]int{}
)

type locker struct {
	ch chan struct{}
}

// Lock create new locker for provided id if it is not exists or takes existing, then locks it
func Lock(owner, id string) {
	log.Debugf("mutex.Lock id: %s REQUEST for owner %s", id, owner)
	mutexLocker.Lock()
	m, ok := lockers[id]
	if !ok {
		m = new(locker)
		m.ch = make(chan struct{}, 1)
		lockers[id] = m
	}
	if _, ok := owners2Lockers[owner]; !ok {
		owners2Lockers[owner] = []string{}
	}
	owners2Lockers[owner] = append(owners2Lockers[owner], id)
	lockersCounters[id]++
	mutexLocker.Unlock()

	m.lock()
	log.Debugf("mutex.Lock id: %s LOCKED for owner %s", id, owner)
}

// Unlock takes existing locker from map and unlocks it
func Unlock(owner, id string) {
	log.Debugf("mutex.Unlock id: %s REQUEST for owner %s", id, owner)
	mutexLocker.Lock()
	defer mutexLocker.Unlock()
	m, ok := lockers[id]
	if !ok {
		log.Debugf("mutex.Unlock id: %s NOT FOUND for owner %s", id, owner)
		return
	}
	m.unlock()
	lockersCounters[id]--
	if lockersCounters[id] == 0 {
		delete(lockers, id)
		delete(lockersCounters, id)
	}
	for i := len(owners2Lockers[owner]) - 1; i >= 0; i-- {
		_id := owners2Lockers[owner][i]
		if id == _id {
			owners2Lockers[owner] = append(owners2Lockers[owner][:i], owners2Lockers[owner][i+1:]...)
		}
		if len(owners2Lockers[owner]) == 0 {
			delete(owners2Lockers, owner)
		}
	}
	log.Debugf("mutex.Unlock id: %s UNLOCKED for owner %s", id, owner)
}

// UnlockForOwner unlocks all lockers locked by owner
func UnlockForOwner(owner string) {
	log.Debugf("mutex.UnlockForOwner REQUEST for owner %s", owner)
	mutexLocker.Lock()
	defer mutexLocker.Unlock()
	if locks, ok := owners2Lockers[owner]; ok {
		for _, id := range locks {
			lockers[id].unlock()
			lockersCounters[id]--
		}
		delete(owners2Lockers, owner)
	}
	log.Debugf("mutex.UnlockForOwner UNLOCKED ALL for owner %s", owner)
}

func (l *locker) lock() {
	l.ch <- struct{}{}
}

func (l *locker) unlock() {
	if len(l.ch) == 0 {
		log.Warn("attempt to unlock no locked locker")
	}
	<-l.ch
}
