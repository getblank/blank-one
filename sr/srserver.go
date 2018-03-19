package sr

import (
	"errors"
	"strings"

	"github.com/getblank/blank-sr/config"
	"github.com/getblank/blank-sr/localstorage"
	"github.com/getblank/blank-sr/registry"
	"github.com/getblank/blank-sr/sync"
	"github.com/getblank/wango"

	"github.com/getblank/blank-one/sessions"
)

// Errors
var (
	ErrInvalidArguments = errors.New("invalid arguments")
	eventHandler        = func(string, []string, interface{}) {}
)

func Init(wamp *wango.Wango, eh func(string, []string, interface{})) {
	eventHandler = eh
	wamp.RegisterSubHandler("registry", registryHandler, nil, nil)
	wamp.RegisterSubHandler("config", configHandler, nil, nil)
	wamp.RegisterSubHandler("sessions", subSessionsHandler, nil, nil)
	wamp.RegisterSubHandler("events", nil, nil, nil)
	wamp.RegisterSubHandler("users", nil, nil, nil)

	wamp.RegisterRPCHandler("register", registerHandler)
	wamp.RegisterRPCHandler("publish", publishHandler)

	wamp.RegisterRPCHandler("sync.lock", syncLockHandler)
	wamp.RegisterRPCHandler("sync.unlock", syncUnlockHandler)
	wamp.RegisterRPCHandler("sync.once", syncOnceHandler)

	wamp.RegisterRPCHandler("localStorage.getItem", localStorageGetItemHandler)
	wamp.RegisterRPCHandler("localStorage.setItem", localStorageSetItemHandler)
	wamp.RegisterRPCHandler("localStorage.removeItem", localStorageRemoveItemHandler)
	wamp.RegisterRPCHandler("localStorage.clear", localStorageClearHandler)
}

func registryHandler(c *wango.Conn, uri string, args ...interface{}) (interface{}, error) {
	services := registry.GetAll()
	return services, nil
}

func subSessionsHandler(c *wango.Conn, uri string, args ...interface{}) (interface{}, error) {
	all := sessions.All()
	return map[string]interface{}{"event": "init", "data": all}, nil
}

func configHandler(c *wango.Conn, uri string, args ...interface{}) (interface{}, error) {
	conf := config.Get()
	return conf, nil
}

// args: uri string, event interface{}, subscribers array of connIDs
// This data will be transferred sent as event on "events" topic
func publishHandler(c *wango.Conn, _ string, args ...interface{}) (interface{}, error) {
	uri, ok := args[0].(string)
	if !ok {
		return nil, ErrInvalidArguments
	}

	subs, ok := args[2].([]interface{})
	if !ok {
		return nil, ErrInvalidArguments
	}

	subscribers := make([]string, 0, len(subs))
	for i, v := range subs {
		if s, ok := v.(string); ok {
			subscribers[i] = s
		} else {
			return nil, ErrInvalidArguments
		}
	}

	eventHandler(uri, subscribers, args[1])
	return nil, nil
}

func registerHandler(c *wango.Conn, uri string, args ...interface{}) (interface{}, error) {
	if args == nil {
		return nil, ErrInvalidArguments
	}

	mes, ok := args[0].(map[string]interface{})
	if !ok {
		return nil, errors.New("Invalid register message")
	}

	_type, ok := mes["type"]
	if !ok {
		return nil, errors.New("Invalid register message. No type")
	}

	typ, ok := _type.(string)
	if !ok || typ == "" {
		return nil, errors.New("Invalid register message. No type")
	}

	remoteAddr := strings.Split(c.RemoteAddr(), ":")[0]
	if remoteAddr == "[" {
		remoteAddr = "127.0.0.1"
	}

	switch typ {
	case registry.TypeFileStore:
		remoteAddr = "http://" + remoteAddr
	default:
		remoteAddr = "ws://" + remoteAddr
	}

	var port string
	if _port, ok := mes["port"]; ok {
		port, ok = _port.(string)
	}

	var commonJS string
	if _commonJS, ok := mes["commonJS"]; ok {
		commonJS, ok = _commonJS.(string)
	}

	registry.Register(typ, remoteAddr, port, c.ID(), commonJS)

	return nil, nil
}

func localStorageGetItemHandler(c *wango.Conn, uri string, args ...interface{}) (interface{}, error) {
	if len(args) == 0 {
		return nil, ErrInvalidArguments
	}
	id, ok := args[0].(string)
	if !ok {
		return nil, ErrInvalidArguments
	}

	return localstorage.GetItem(id), nil
}

func localStorageSetItemHandler(c *wango.Conn, _ string, args ...interface{}) (interface{}, error) {
	if len(args) < 2 {
		return nil, ErrInvalidArguments
	}

	id, ok := args[0].(string)
	if !ok {
		return nil, ErrInvalidArguments
	}

	item, ok := args[1].(string)
	if !ok {
		return nil, ErrInvalidArguments
	}

	return localstorage.SetItem(id, item), nil
}

func localStorageRemoveItemHandler(c *wango.Conn, _ string, args ...interface{}) (interface{}, error) {
	if len(args) == 0 {
		return nil, ErrInvalidArguments
	}

	id, ok := args[0].(string)
	if !ok {
		return nil, ErrInvalidArguments
	}

	localstorage.RemoveItem(id)
	return nil, nil
}

func localStorageClearHandler(c *wango.Conn, _ string, args ...interface{}) (interface{}, error) {
	localstorage.Clear()

	return nil, nil
}

func syncLockHandler(c *wango.Conn, _ string, args ...interface{}) (interface{}, error) {
	if len(args) == 0 {
		return nil, ErrInvalidArguments
	}

	id, ok := args[0].(string)
	if !ok {
		return nil, ErrInvalidArguments
	}

	sync.Lock(c.ID(), id)

	return nil, nil
}

func syncOnceHandler(c *wango.Conn, _ string, args ...interface{}) (interface{}, error) {
	if len(args) == 0 {
		return nil, ErrInvalidArguments
	}

	id, ok := args[0].(string)
	if !ok {
		return nil, ErrInvalidArguments
	}

	return nil, sync.Once(id)
}

func syncUnlockHandler(c *wango.Conn, _ string, args ...interface{}) (interface{}, error) {
	if len(args) == 0 {
		return nil, ErrInvalidArguments
	}

	id, ok := args[0].(string)
	if !ok {
		return nil, ErrInvalidArguments
	}

	sync.Unlock(c.ID(), id)

	return nil, nil
}
