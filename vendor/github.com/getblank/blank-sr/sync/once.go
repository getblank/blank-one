package sync

import (
	"errors"
	"sync"
	"time"
)

var (
	oncers      = map[string]struct{}{}
	onceLocker  sync.Mutex
	errExecuted = errors.New("already executed")
	ttl         = time.Minute
)

// Once will return nil only to one caller for id provided in minute.
// Other callers will get error errExecuted
func Once(id string) error {
	return once(id)
}

func once(id string) error {
	onceLocker.Lock()
	defer onceLocker.Unlock()
	if _, ok := oncers[id]; ok {
		return errExecuted
	}
	oncers[id] = makeOncer(id)
	return nil
}

func makeOncer(id string) struct{} {
	time.AfterFunc(ttl, func() {
		onceLocker.Lock()
		delete(oncers, id)
		onceLocker.Unlock()
	})
	return struct{}{}
}
