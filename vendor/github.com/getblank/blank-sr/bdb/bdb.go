package bdb

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"

	"github.com/getblank/blank-sr/berror"

	"github.com/boltdb/bolt"
	log "github.com/sirupsen/logrus"
)

var (
	BoltDB *bolt.DB
	opened bool
	mutex  = &sync.Mutex{}
	dbpath = "./blank.db"
)

type M map[string]interface{}

type DB struct{}

func Opened() bool {
	mutex.Lock()
	defer mutex.Unlock()
	return opened
}

func init() {
	var err error
	BoltDB, err = bolt.Open(dbpath, 0600, nil)
	if err != nil {
		panic("Can't open DB file " + dbpath)
	}
	mutex.Lock()
	defer mutex.Unlock()
	opened = true
	// go boltview.Init(BoltDB)
}

func (DB) Connected() bool {
	return Opened()
}

func (DB) Inc(bucket, key, propPath string, inc float64) (result M, err error) {
	BoltDB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			err = berror.DbNotFound
			return err
		}
		v := b.Get([]byte(key))
		if v == nil {
			err = berror.DbNotFound
			return err
		}
		err = json.Unmarshal(v, &result)
		if err != nil {
			return err
		}
		props := strings.Split(propPath, ".")
		if len(props) == 1 {
			_val, ok := result[propPath]
			if ok && _val != nil {
				val, ok := _val.(float64)
				if !ok {
					err = berror.WrongData
					return err
				}
				val += inc
				result[propPath] = val
			} else {
				result[propPath] = inc
			}
		} else {
			_obj, ok := result[props[0]]
			if !ok {
				err = berror.WrongData
				return err
			}
			obj, ok := _obj.(map[string]interface{})
			if !ok {
				err = berror.WrongData
				return err
			}
			_val, ok := obj[props[1]]
			if ok && _val != nil {
				val, ok := _val.(float64)
				if !ok {
					err = berror.WrongData
					return err
				}
				val += inc
				obj[props[1]] = val
			} else {
				obj[props[1]] = inc
			}
			result[props[0]] = obj
		}
		var encoded []byte
		encoded, err = json.Marshal(result)
		if err != nil {
			return err
		}
		err = b.Put([]byte(key), encoded)
		return err
	})
	return
}

func (DB) Delete(bucket, key string) (err error) {
	BoltDB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			err = berror.DbNotFound
			return err
		}
		err = b.Delete([]byte(key))
		return err
	})
	return
}

func (DB) DeleteNested(key []byte, buckets ...[]byte) (err error) {
	if buckets == nil {
		return errors.New("No buckets provided")
	}
	BoltDB.Update(func(tx *bolt.Tx) error {
		var currentBucket *bolt.Bucket
		var currentBucketName []byte
		for _, _b := range buckets {
			var b *bolt.Bucket
			if currentBucket == nil {
				b = tx.Bucket(_b)
			} else {
				b = currentBucket.Bucket(_b)
			}
			if b == nil {
				err = berror.DbNotFound
				return err
			}
			currentBucket = b
			currentBucketName = _b
		}
		err = currentBucket.Delete(key)
		if err != nil {
			return err
		}
		if currentBucket.Stats().KeyN == 0 && len(buckets) > 1 {
			for i, _b := range buckets {
				if i == len(buckets)-1 {
					break
				}
				var b *bolt.Bucket
				if currentBucket == nil {
					b = tx.Bucket(_b)
				} else {
					b = currentBucket.Bucket(_b)
				}
				if b == nil {
					return nil
				}
				currentBucket = b
			}
			currentBucket.DeleteBucket(currentBucketName)
		}
		return nil
	})
	return
}

func (DB) DeleteBucket(bucket string) (err error) {
	BoltDB.Update(func(tx *bolt.Tx) error {
		err = tx.DeleteBucket([]byte(bucket))
		return err
	})
	return
}

func (DB) Get(bucket, key string) (result []byte, err error) {
	BoltDB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			err = berror.DbNotFound
			return err
		}
		v := b.Get([]byte(key))

		if v == nil {
			err = berror.DbNotFound
			return err
		}
		result = make([]byte, len(v))
		copy(result, v)

		return nil
	})
	return
}

func (DB) GetUnmarshalled(bucket, key string) (result M, err error) {
	BoltDB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			err = berror.DbNotFound
			return err
		}
		_v := b.Get([]byte(key))

		if _v == nil {
			err = berror.DbNotFound
			return err
		}
		err = json.Unmarshal(_v, &result)
		return nil
	})
	return
}

func (DB) GetUnmarshalledIntoInterface(bucket, key string, _interface interface{}) (err error) {
	BoltDB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			err = berror.DbNotFound
			return err
		}
		_v := b.Get([]byte(key))

		if _v == nil {
			err = berror.DbNotFound
			return err
		}
		err = json.Unmarshal(_v, _interface)
		return nil
	})
	return
}

func (DB) GetAll(bucket string) (result [][]byte, err error) {
	BoltDB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			err = berror.DbNotFound
			return err
		}
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			if len(v) == 0 {
				if nb := b.Bucket(k); nb != nil {
					continue
				}
			}
			_v := make([]byte, len(v))
			copy(_v, v)
			result = append(result, _v)
		}
		return nil
	})
	return
}

func (DB) GetAllUnmarshalled(bucket string) (result []M, err error) {
	result = []M{}
	BoltDB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			err = berror.DbNotFound
			return err
		}
		c := b.Cursor()
		for k, _v := c.First(); k != nil; k, _v = c.Next() {
			if len(_v) == 0 {
				if nb := b.Bucket(k); nb != nil {
					continue
				}
			}
			var m M
			err = json.Unmarshal(_v, &m)
			if err != nil {
				log.Error("Error when unmarshall", err.Error())
				continue
			}
			result = append(result, m)
		}
		return nil
	})
	return
}

func (DB) GetAllKeys(bucket string) (data []string, err error) {
	data = []string{}
	BoltDB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			err = berror.DbNotFound
			return err
		}
		c := b.Cursor()
		if c != nil {
			for k, v := c.First(); k != nil; k, v = c.Next() {
				if len(v) == 0 {
					if nb := b.Bucket(k); nb != nil {
						continue
					}
				}
				data = append(data, string(k))
			}
		}
		return nil
	})
	return
}

func (DB) GetAllKeysByPrefix(bucket, _prefix string) (data []string, err error) {
	data = []string{}
	BoltDB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			err = berror.DbNotFound
			return err
		}
		c := b.Cursor()
		prefix := []byte(_prefix)
		for k, _ := c.Seek(prefix); bytes.HasPrefix(k, prefix); k, _ = c.Next() {
			data = append(data, string(k))
		}
		return nil
	})
	return
}

func (DB) GetFromNested(bucket, nestedBucket, key string) (result M, err error) {
	BoltDB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			err = berror.DbNotFound
			return err
		}
		nB := b.Bucket([]byte(nestedBucket))
		if nB == nil {
			err = berror.DbNotFound
			return err
		}
		_v := nB.Get([]byte(key))

		if _v == nil {
			err = berror.DbNotFound
			return err
		}
		err = json.Unmarshal(_v, &result)
		return nil
	})
	return
}

func (DB) GetNextSequenceForBucket(bucket string, subBucket *string) (sequence int, err error) {
	var _sequence uint64
	BoltDB.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucket))
		if err != nil {
			log.Error("Can't create bucket:", bucket, err.Error())
			return err
		}
		if subBucket == nil {
			_sequence, err = b.NextSequence()
			return err
		}
		subB, err := b.CreateBucketIfNotExists([]byte(*subBucket))
		if err != nil {
			log.Error("Can't create subBucket bucket:", bucket+"."+*subBucket, err.Error())
			return err
		}
		_sequence, err = subB.NextSequence()
		return err
	})
	return int(_sequence), err
}

func (DB) Count(bucket string) (count int) {
	BoltDB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return nil
		}
		stat := b.Stats()
		count = stat.KeyN
		return nil
	})
	return count
}

func (DB) Save(bucket, key string, value interface{}) (err error) {
	encoded, ok := value.([]byte)
	if !ok {
		encoded, err = json.Marshal(value)
		if err != nil {
			return err
		}
	}
	BoltDB.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucket))
		if err != nil {
			log.Error("Can't create bucket:", bucket, err.Error())
			return err
		}
		err = b.Put([]byte(key), encoded)
		return err
	})
	return
}

func (DB) SaveInNested(bucket, nestedBucket, key string, value interface{}) (err error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	BoltDB.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucket))
		if err != nil {
			log.Error("Can't create bucket:", bucket, err.Error())
			return err
		}
		nb, err := b.CreateBucketIfNotExists([]byte(nestedBucket))
		err = nb.Put([]byte(key), encoded)
		return err
	})
	return
}

func (DB) DbWriteTo(w io.Writer) (err error) {
	BoltDB.View(func(tx *bolt.Tx) error {
		_, err = tx.WriteTo(w)
		return err
	})

	return err
}
