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

// Init register SR handlers to wamp server
func Init(wamp *wango.Wango, eh func(string, []string, interface{})) {
	eventHandler = eh
	if err := wamp.RegisterSubHandler("registry", registryHandler, nil, nil); err != nil {
		panic(err)
	}
	if err := wamp.RegisterSubHandler("config", configHandler, nil, nil); err != nil {
		panic(err)
	}
	if err := wamp.RegisterSubHandler("sessions", subSessionsHandler, nil, nil); err != nil {
		panic(err)
	}
	if err := wamp.RegisterSubHandler("events", nil, nil, nil); err != nil {
		panic(err)
	}
	if err := wamp.RegisterSubHandler("users", nil, nil, nil); err != nil {
		panic(err)
	}

	if err := wamp.RegisterRPCHandler("register", registerHandler); err != nil {
		panic(err)
	}
	if err := wamp.RegisterRPCHandler("sync.lock", syncLockHandler); err != nil {
		panic(err)
	}
	if err := wamp.RegisterRPCHandler("sync.unlock", syncUnlockHandler); err != nil {
		panic(err)
	}
	if err := wamp.RegisterRPCHandler("sync.once", syncOnceHandler); err != nil {
		panic(err)
	}

	if err := wamp.RegisterRPCHandler("localStorage.getItem", localStorageGetItemHandler); err != nil {
		panic(err)
	}
	if err := wamp.RegisterRPCHandler("localStorage.setItem", localStorageSetItemHandler); err != nil {
		panic(err)
	}
	if err := wamp.RegisterRPCHandler("localStorage.removeItem", localStorageRemoveItemHandler); err != nil {
		panic(err)
	}
	if err := wamp.RegisterRPCHandler("localStorage.clear", localStorageClearHandler); err != nil {
		panic(err)
	}
}

// FSAddress returns File Storage address if exists or empty string
func FSAddress() string {
	return registry.FSAddress()
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
	if !ok || len(typ) == 0 {
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
		port, _ = _port.(string)
	}

	var commonJS string
	if _commonJS, ok := mes["commonJS"]; ok {
		commonJS, _ = _commonJS.(string)
	}

	return registry.Register(typ, remoteAddr, port, c.ID(), commonJS)
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
