package scheduler

import (
	"sync"

	"github.com/robfig/cron"

	"github.com/getblank/blank-router/taskq"
	"github.com/getblank/blank-sr/config"

	"github.com/getblank/blank-one/logging"
)

var (
	storeSchedulers = map[string]*cron.Cron{}
	locker          sync.RWMutex
	runningTasks    = map[string]map[int]struct{}{}

	log = logging.Logger()
)

func onConfigUpdate(c map[string]config.Store) {
	for storeName, conf := range c {
		updateScheduler(storeName, conf.Tasks)
	}
}

func updateScheduler(storeName string, tasks []*config.Task) {
	locker.Lock()
	defer locker.Unlock()

	if currentScheduler, ok := storeSchedulers[storeName]; ok {
		currentScheduler.Stop()
		delete(storeSchedulers, storeName)
	}

	if len(tasks) == 0 {
		return
	}

	runningTasks[storeName] = map[int]struct{}{}

	c := cron.New()
	for i := range tasks {
		t := tasks[i]
		index := i
		err := c.AddFunc(t.Schedule, func() {
			runTask(storeName, index, t.AllowConcurrent)
		})

		if err != nil {
			log.Errorf("Can't add sheduled task for store: %s, taskIndex: %d, error: %v", storeName, i, err)
			continue
		}

		log.Debugf("Scheduled task added to cron for store: %s, taskIndex: %d", storeName, i)
	}

	c.Start()
	storeSchedulers[storeName] = c
}

func runTask(storeName string, index int, allowConcurrent bool) {
	log.Debugf("Time to run scheduled task for store: %s, taskIndex: %d", storeName, index)

	if !canRunTask(storeName, index, allowConcurrent) {
		log.Debugf("Concurrent is not allowed. Prev task is not completed for store: %s, taskIndex: %d", storeName, index)
		return
	}

	if !checkAndMarkTaskRunning(storeName, index) {
		log.Debugf("Concurrent is not allowed. Can't start task because prev task is not completed for store: %s, taskIndex: %d", storeName, index)
		return
	}

	log.Debugf("Starting scheduled task for store: %s, taskIndex: %d", storeName, index)
	t := taskq.Task{
		Type:   taskq.ScheduledScript,
		Store:  storeName,
		UserID: "system",
		Arguments: map[string]interface{}{
			"taskIndex": index,
		},
	}

	res, err := taskq.PushAndGetResult(&t, 0)

	markTaskCompleted(storeName, index)
	if err != nil {
		log.Debugf("Scheduled task completed with error for store: %s, taskIndex: %d, error: %v", storeName, index, err)
		return
	}

	log.Debugf("Scheduled task completed for store: %s, taskIndex: %d, result: %v", storeName, index, res)
}

func canRunTask(storeName string, index int, allowConcurrent bool) bool {
	if allowConcurrent {
		return true
	}

	locker.RLock()
	_, ok := runningTasks[storeName][index]
	locker.RUnlock()

	return !ok
}

// if returns true, task can be run
func checkAndMarkTaskRunning(storeName string, index int) bool {
	locker.Lock()
	defer locker.Unlock()

	_, ok := runningTasks[storeName][index]
	if ok {
		return false
	}

	runningTasks[storeName][index] = struct{}{}
	return true
}

func markTaskCompleted(storeName string, index int) {
	locker.Lock()
	delete(runningTasks[storeName], index)
	locker.Unlock()
}

func init() {
	config.OnUpdate(onConfigUpdate)
}
