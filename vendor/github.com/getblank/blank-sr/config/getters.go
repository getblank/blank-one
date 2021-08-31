package config

import (
	"github.com/jinzhu/copier"
)

func Get() map[string]Store {
	confLocker.RLock()
	defer confLocker.RUnlock()
	res := map[string]Store{}
	for k, _v := range config {
		v := Store{}
		copier.Copy(&v, &_v)
		res[k] = v
	}
	return res
}