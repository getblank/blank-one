package config

import (
	"fmt"
	"os"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	log "github.com/sirupsen/logrus"
)

const cfgCollection = "blankCustomStores"

// MongoCFGProvider is a ConfigProvider implementation with storing config in MongoDB.
type MongoCFGProvider struct {
	url string
	db  string
}

// Name implements Name method of ConfigProvider.
func (p *MongoCFGProvider) Name() string {
	return "MongoCFGProvider"
}

// Get implements Get method of ConfigProvider.
func (p *MongoCFGProvider) Get() (map[string]Store, error) {
	log.Info("Start loading config from MongoDB")
	session, err := mgo.Dial(p.url)
	if err != nil {
		return nil, err
	}

	defer session.Close()
	c := session.DB(p.db).C(collectionName())
	bson.SetJSONTagFallback(true)
	bson.SetRespectNilValues(true)

	var stores []*Store
	if err := c.Find(nil).All(&stores); err != nil {
		return nil, err
	}

	res := map[string]Store{}
	for _, s := range stores {
		res[s.ID] = *s
	}

	log.Infof("Loading config from MongoDB completed, loaded %d stores", len(res))

	return res, nil
}

// RegisterMongoCFGProvider registers MongoCFGProvider.
func RegisterMongoCFGProvider() {
	dbName := "blank"
	if v := os.Getenv("MONGO_PORT_27017_DB_NAME"); len(v) > 0 {
		dbName = v
	}
	dbAddr := "localhost"
	if v := os.Getenv("MONGO_PORT_27017_TCP_ADDR"); len(v) > 0 {
		dbAddr = v
	}
	dbPort := "27017"
	if v := os.Getenv("MONGO_PORT_27017_TCP_PORT"); len(v) > 0 {
		dbPort = v
	}

	url := fmt.Sprintf("mongodb://%s:%s", dbAddr, dbPort)
	p := &MongoCFGProvider{url, dbName}
	RegisterConfigProvider(p)
}

func collectionName() string {
	if v := os.Getenv("BLANK_CUSTOM_STORES_COLLECTION"); len(v) > 0 {
		return v
	}

	return cfgCollection
}
