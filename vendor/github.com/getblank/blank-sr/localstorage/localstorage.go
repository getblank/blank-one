package localstorage

import (
	log "github.com/sirupsen/logrus"

	"github.com/getblank/blank-sr/bdb"
)

var (
	bucket = "_localStorage"
	db     = bdb.DB{}
)

// GetItem returns item from localstorage or nil of it was not find
func GetItem(id string) interface{} {
	item, err := db.Get(bucket, id)
	if err != nil {
		return nil
	}
	return string(item)
}

// SetItem sets item in localStorage to provided value
func SetItem(id, value string) error {
	return db.Save(bucket, id, []byte(value))
}

// RemoveItem removes item from localStorage
func RemoveItem(id string) {
	db.Delete(bucket, id)
	return
}

// Clear removes all items from localStorage
func Clear() {
	go clear()
}

func clear() {
	err := db.DeleteBucket(bucket)
	if err != nil {
		if err.Error() != "bucket not found" {
			log.WithError(err).Error("Can't clear localStorage")
		}
	}
}
