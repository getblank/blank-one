package intranet

import (
	"sync"

	"github.com/getblank/blank-router/taskq"
	"github.com/getblank/blank-sr/config"
)

var onEventHandler = func(string, interface{}, []string) {}

func Init() {
	runServer()
}

// OnEvent sets intranet event handler
func OnEvent(fn func(string, interface{}, []string)) {
	onEventHandler = fn
}

func srEventHandler(uri string, subscribers []string, event interface{}) {
	if len(subscribers) == 0 {
		return
	}

	onEventHandler(uri, event, subscribers)
}

var storeMigrationVersion = map[string]map[int]struct{}{}
var storeMigrationLocker sync.Mutex

func isStoreVersionProcessed(storeName string, version int) bool {
	storeMigrationLocker.Lock()
	defer storeMigrationLocker.Unlock()

	if storeMigrationVersion[storeName] == nil {
		return false
	}

	_, ok := storeMigrationVersion[storeName][version]
	return ok
}

func setProcessedStoreVersion(storeName string, version int) {
	storeMigrationLocker.Lock()
	defer storeMigrationLocker.Unlock()

	if storeMigrationVersion[storeName] == nil {
		storeMigrationVersion[storeName] = map[int]struct{}{}
	}

	storeMigrationVersion[storeName][version] = struct{}{}
}

func runMigrationScripts(conf map[string]config.Store) {
	for storeName, storeDesc := range conf {
		if storeDesc.StoreLifeCycle.Migration == nil {
			continue
		}

		if isStoreVersionProcessed(storeName, storeDesc.Version) {
			continue
		}

		log.Infof("Will run migration scripts for store %s if needed", storeName)
		t := &taskq.Task{
			Type:   taskq.StoreLifeCycle,
			UserID: "system",
			Store:  storeName,
			Arguments: map[string]interface{}{
				"event": "migration",
			},
		}

		res, err := taskq.PushAndGetResult(t, 0)
		if err != nil {
			log.Errorf("Migration scripts for store %s completed with error: %v", storeName, err)
			continue
		}

		setProcessedStoreVersion(storeName, storeDesc.Version)
		log.Infof("Migration scripts for store %s completed with result: %v", storeName, res)
	}
}
