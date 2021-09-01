package intranet

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/getblank/wango"
	"github.com/go-chi/chi"
	"golang.org/x/net/websocket"

	"github.com/getblank/blank-router/berrors"
	"github.com/getblank/blank-router/taskq"
	"github.com/getblank/blank-sr/config"
	"github.com/getblank/blank-sr/registry"

	"github.com/getblank/blank-one/appconfig"
	"github.com/getblank/blank-one/logging"
	"github.com/getblank/blank-one/sessions"
	"github.com/getblank/blank-one/sr"
)

const (
	getTaskURI   = "get"
	doneTaskURI  = "done"
	errorTaskURI = "error"
	publishURI   = "publish"
	cronRunURI   = "cron.run"
	uriSubStores = "com.stores"

	rpcSessionNew        = "session.new"
	rpcSessionCheck      = "session.check"
	rpcSessionDelete     = "session.delete"
	rpcSessionUserUpdate = "session.user-update"
)

var (
	wampServer           = wango.New()
	log                  = logging.Logger()
	taskWatchChan        = make(chan taskKeeper, 1000)
	workerConnectChan    = make(chan string)
	workerDisconnectChan = make(chan string)
	listeningPort        = "2345"
)

type taskKeeper struct {
	workerID string
	taskID   uint64
	done     bool
}

func taskGetHandler(c *wango.Conn, uri string, args ...interface{}) (interface{}, error) {
	log.Debugf("Get task request from client \"%s\"", c.ID())
	t := taskq.Shift()
	log.Debugf("Shifted task id: \"%d\" type: \"%s\" for client \"%s\"", t.ID, t.Type, c.ID())
	if c.Connected() {
		taskWatchChan <- taskKeeper{c.ID(), t.ID, false}
		return t, nil
	}

	taskq.UnShift(t)
	log.Debugf("Shifted task id: \"%d\" type: \"%s\" returned to the queue because client \"%s\" already disconnected", t.ID, t.Type, c.ID())

	return nil, nil
}

func taskDoneHandler(c *wango.Conn, uri string, args ...interface{}) (interface{}, error) {
	if len(args) < 2 {
		log.Warn("Invalid task.done RPC")
		return nil, berrors.ErrInvalidArguments
	}

	id, ok := args[0].(float64)
	if !ok {
		log.Warnf("Invalid task.id %v in task.done RPC", args[0])
		return nil, berrors.ErrInvalidArguments
	}

	log.Debugf("Task done id: \"%d\" from client \"%s\"", int(id), c.ID())

	result := taskq.Result{
		ID:     uint64(id),
		Result: args[1],
	}

	taskq.Done(result)
	taskWatchChan <- taskKeeper{c.ID(), uint64(id), true}

	return nil, nil
}

func taskErrorHandler(c *wango.Conn, uri string, args ...interface{}) (interface{}, error) {
	if len(args) < 2 {
		log.Warn("Invalid task.error RPC")
		return nil, berrors.ErrInvalidArguments
	}

	id, ok := args[0].(float64)
	if !ok {
		log.Warnf("Invalid task.id %v in task.done RPC", args[0])
		return nil, berrors.ErrInvalidArguments
	}

	err, ok := args[1].(string)
	if !ok {
		log.Warn("Invalid description in task.error RPC")
		return nil, berrors.ErrInvalidArguments
	}

	log.Debugf("Task error id: \"%d\" err: \"%s\" from client \"%s\"", int(id), err, c.ID())
	result := taskq.Result{
		ID:  uint64(id),
		Err: err,
	}

	taskq.Done(result)
	taskWatchChan <- taskKeeper{c.ID(), uint64(id), true}

	return nil, nil
}

func cronRunHandler(c *wango.Conn, uri string, args ...interface{}) (interface{}, error) {
	if len(args) < 2 {
		log.Warn("Invalid cron.run RPC")
		return nil, berrors.ErrInvalidArguments
	}

	storeName, ok := args[0].(string)
	if !ok {
		log.Warn("Invalid storeName when cron.run RPC")
		return nil, berrors.ErrInvalidArguments
	}

	index, ok := args[1].(float64)
	if !ok {
		log.Warn("Invalid task index when cron.run RPC")
		return nil, berrors.ErrInvalidArguments
	}

	t := taskq.Task{
		Type:   taskq.ScheduledScript,
		Store:  storeName,
		UserID: "system",
		Arguments: map[string]interface{}{
			"taskIndex": index,
		},
	}

	resChan := taskq.Push(&t)

	res := <-resChan
	if res.Err != "" {
		return nil, errors.New(res.Err)
	}

	return res.Result, nil
}

func internalOpenCallback(c *wango.Conn) {
	log.Infof("Connected client to TQ: '%s'", c.ID())
	workerConnectChan <- c.ID()
}

func internalCloseCallback(c *wango.Conn) {
	log.Infof("Disconnected client from TQ: '%s'", c.ID())
	workerDisconnectChan <- c.ID()
}

// args: uri string, event interface{}, subscribers array of connIDs
// This data will be transferred sent as event on "events" topic
func publishHandler(c *wango.Conn, _uri string, args ...interface{}) (interface{}, error) {
	if len(args) < 2 {
		return nil, berrors.ErrInvalidArguments
	}

	uri, ok := args[0].(string)
	if !ok {
		return nil, berrors.ErrInvalidArguments
	}

	wampServer.Publish(uri, args[1])

	if len(args) < 3 {
		onEventHandler(uri, args[1], nil)
		return nil, nil
	}

	_subscribers, ok := args[2].([]interface{})
	if !ok {
		return nil, berrors.ErrInvalidArguments
	}

	subscribers := make([]string, len(_subscribers))
	for k, v := range _subscribers {
		connID, ok := v.(string)
		if !ok {
			log.Warn("ConnID is not a string in 'publish' RPC call")
			continue
		}
		subscribers[k] = connID
	}

	onEventHandler(uri, args[1], subscribers)

	return nil, nil
}

func subStoresHandler(c *wango.Conn, _uri string, args ...interface{}) (interface{}, error) {
	storeName := strings.TrimPrefix(_uri, uriSubStores)
	t := taskq.Task{
		Store:  storeName,
		Type:   taskq.DbFind,
		UserID: "root",
		Arguments: map[string]interface{}{
			"query": map[string]interface{}{},
			"take":  1,
		},
	}

	_res, err := taskq.PushAndGetResult(&t, 0)
	if err != nil {
		return nil, err
	}

	result := map[string]interface{}{"event": "init", "data": nil}
	res, ok := _res.(map[string]interface{})
	if !ok {
		result["data"] = _res
		return result, nil
	}

	result["data"] = res["items"]

	return result, nil
}

func taskWatcher() {
	workerTasks := map[string]map[uint64]struct{}{}
	errWorkerDisconnectedText := "connection with worker lost"
	for {
		select {
		case t := <-taskWatchChan:
			if t.done {
				delete(workerTasks[t.workerID], t.taskID)
			} else {
				workerTasks[t.workerID][t.taskID] = struct{}{}
			}
		case workerID := <-workerConnectChan:
			workerTasks[workerID] = map[uint64]struct{}{}
		case workerID := <-workerDisconnectChan:
			workerTasksNumber := len(workerTasks[workerID])
			if len(workerTasks[workerID]) == 0 {
				continue
			}
			log.Infof("Worker %s disconnected. Need to close all proccessing tasks by this worker with error. Worker is running %d tasks now.", workerID, workerTasksNumber)
			for taskID := range workerTasks[workerID] {
				result := taskq.Result{
					ID:  taskID,
					Err: errWorkerDisconnectedText,
				}
				taskq.Done(result)
			}
			delete(workerTasks, workerID)
			log.Infof("All workers %s tasks closed.", workerID)
		}
	}
}

func sessionNewHandler(c *wango.Conn, uri string, args ...interface{}) (interface{}, error) {
	if len(args) < 1 {
		log.Warn("Invalid cron.run RPC")
		return nil, berrors.ErrInvalidArguments
	}

	user, ok := args[0].(map[string]interface{})
	if !ok {
		userID, ok := args[0].(string)
		if !ok {
			return nil, berrors.ErrInvalidArguments
		}
		user = map[string]interface{}{"_id": userID}
	}

	return sessions.NewSession(user, "")
}

func checkErrorAndPanic(err error) {
	if err != nil {
		panic(err)
	}
}

func runServer() {
	go taskWatcher()

	wampServer.SetSessionOpenCallback(internalOpenCallback)
	wampServer.SetSessionCloseCallback(internalCloseCallback)

	config.OnUpdate(func(c map[string]config.Store) {
		wampServer.Publish("config", c)
		runMigrationScripts(c)
	})

	checkErrorAndPanic(wampServer.RegisterRPCHandler(getTaskURI, taskGetHandler))
	checkErrorAndPanic(wampServer.RegisterRPCHandler(doneTaskURI, taskDoneHandler))
	checkErrorAndPanic(wampServer.RegisterRPCHandler(errorTaskURI, taskErrorHandler))
	checkErrorAndPanic(wampServer.RegisterRPCHandler(publishURI, publishHandler))
	checkErrorAndPanic(wampServer.RegisterRPCHandler(cronRunURI, cronRunHandler))

	checkErrorAndPanic(wampServer.RegisterRPCHandler(rpcSessionNew, sessionNewHandler))

	checkErrorAndPanic(wampServer.RegisterSubHandler(uriSubStores, subStoresHandler, nil, nil))

	sr.Init(wampServer, srEventHandler)

	s := new(websocket.Server)
	s.Handshake = func(c *websocket.Config, r *http.Request) error {
		return nil
	}

	s.Handler = func(ws *websocket.Conn) {
		wampServer.WampHandler(ws, nil)
	}

	if tqPort := os.Getenv("BLANK_TASK_QUEUE_PORT"); len(tqPort) > 0 {
		listeningPort = tqPort
	}

	wampServer.RegisterSubHandler("assets-update", func(c *wango.Conn, uri string, args ...interface{}) (interface{}, error) {
		return nil, nil
	}, nil, nil)

	r := chi.NewRouter()
	r.Handle("/", s)
	if nodeEnv := os.Getenv("NODE_ENV"); nodeEnv == "DEV" {
		r.Post("/config", appconfig.PostConfigHandler)
		r.Post("/lib/lib.zip", func(w http.ResponseWriter, r *http.Request) {
			appconfig.PostLibHandler(w, r)
			wampServer.Publish("assets-update", "lib.zip")
		})
		r.Post("/assets/assets.zip", appconfig.PostAssetsHandler)
	}

	r.Get("/lib/", libHandler)

	log.Info("TaskQueue will listen for connection on port ", listeningPort)
	if _, err := registry.Register("taskQueue", "ws://127.0.0.1", listeningPort, "0", ""); err != nil {
		log.Fatalf("register taskQ error: %v", err)
	}

	err := http.ListenAndServe(":"+listeningPort, r)
	if err != nil {
		log.Fatalf("ListenAndServe: %v", err)
	}
}

func libHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := w.Write(appconfig.GetLibZip()); err != nil {
		log.Debugf("[libHandler] write error: %v", err)
	}
}
