package registry

import (
	"fmt"
	"sync"

	log "github.com/sirupsen/logrus"
)

var (
	services       = map[string][]Service{}
	locker         sync.RWMutex
	createHandlers = []func(Service){}
	updateHandlers = []func(Service){}
	deleteHandlers = []func(Service){}
)

// Services types consts
const (
	TypeWorker    = "worker"
	TypePBX       = "pbx"
	TypeTaskQueue = "taskQueue"
	TypeCron      = "cron"
	TypeFileStore = "fileStore"
	TypeQueue     = "queue"

	PortWorker    = "1234"
	PortPBX       = "1234"
	PortTaskQueue = "1234"
)

type Service struct {
	Type     string `json:"type"`
	Address  string `json:"address"`
	Port     string `json:"port"`
	CommonJS string `json:"commonJS,omitempty"`
	connID   string
}

type RegisterMessage struct {
	Type string `json:"type"`
}

// FSAddress returns File Storage address if exists or empty string
func FSAddress() string {
	locker.RLock()
	defer locker.RUnlock()

	for k, v := range services {
		if k == TypeFileStore {
			if len(v) == 0 {
				return ""
			}

			return fmt.Sprintf("%s:%s", v[0].Address, v[0].Port)
		}
	}

	return ""
}

// GetAll returns all services from registry
func GetAll() map[string][]Service {
	locker.RLock()
	defer locker.RUnlock()
	all := map[string][]Service{}
	for k, v := range services {
		all[k] = append([]Service{}, v...)
	}

	return all
}

// OnCreate pass handler func, that will call when new service will created
func OnCreate(fn func(Service)) {
	createHandlers = append(createHandlers, fn)
}

// OnUpdate pass handler func, that will call when existing service will created
func OnUpdate(fn func(Service)) {
	updateHandlers = append(updateHandlers, fn)
}

// OnDelete pass handler func, that will call when existing service will deleted
func OnDelete(fn func(Service)) {
	deleteHandlers = append(deleteHandlers, fn)
}

// Register adds new service in registry
func Register(typ, remoteAddr, port, connID, commonJS string) (interface{}, error) {
	if port == "" {
		switch typ {
		case TypeWorker:
			port = PortWorker
		case TypePBX:
			port = PortPBX
		case TypeTaskQueue:
			port = PortTaskQueue
		}
	}

	s := Service{
		Type:     typ,
		Address:  remoteAddr,
		Port:     port,
		CommonJS: commonJS,
		connID:   connID,
	}
	register(s)

	for _, h := range createHandlers {
		h(s)
	}

	log.Infof(`Registered "%s" service at address: "%s" and port: "%s"`, typ, remoteAddr, port)

	return nil, nil
}

// Unregister removes service from registry
func Unregister(id string) {
	unregister(id)
}

func register(service Service) {
	locker.Lock()
	defer locker.Unlock()

	if services[service.Type] == nil {
		services[service.Type] = []Service{}
	}
	services[service.Type] = append(services[service.Type], service)
}

func unregister(id string) {
	locker.Lock()
	defer locker.Unlock()
	for typ, ss := range services {
		for i, _ss := range ss {
			if _ss.connID == id {
				services[typ] = append(ss[:i], ss[i+1:]...)
				for _, h := range deleteHandlers {
					go h(_ss)
				}
				return
			}
		}
	}
}
