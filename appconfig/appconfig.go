package appconfig

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"

	"golang.org/x/tools/godoc/vfs"
	"golang.org/x/tools/godoc/vfs/zipfs"

	"github.com/getblank/blank-sr/config"

	"github.com/getblank/blank-one/logging"
)

const (
	libZipFileName    = "lib.zip"
	assetsZipFileName = "assets.zip"
)

var (
	libFS    vfs.FileSystem
	assetsFS vfs.FileSystem
	libZip   []byte
	fsLocker sync.RWMutex

	log = logging.Logger()
)

func GetAsset(w http.ResponseWriter, filePath string) {
	fsLocker.RLock()
	if assetsFS == nil {
		w.WriteHeader(http.StatusNotFound)
		if _, err := w.Write([]byte("file not found")); err != nil {
			log.Errorf("write response error: %v", err)
		}

		fsLocker.RUnlock()
		return
	}

	b, err := vfs.ReadFile(assetsFS, strings.TrimPrefix(filePath, "/assets"))
	fsLocker.RUnlock()
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		if _, err := w.Write([]byte(err.Error())); err != nil {
			log.Errorf("write response error: %v", err)
		}
		return
	}

	contentType := mime.TypeByExtension(path.Ext(filePath))
	if len(contentType) == 0 {
		contentType = http.DetectContentType(b)
	}

	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(b); err != nil {
		log.Errorf("write response error: %v", err)
	}
}

func PostAssetsHandler(rw http.ResponseWriter, request *http.Request) {
	postLibHandler(rw, request, assetsZipFileName)
	makeAssetsFS()
}

func GetLibZip() []byte {
	fsLocker.RLock()
	defer fsLocker.RUnlock()

	return libZip[:]
}

func PostConfigHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusBadRequest)
		if _, err := w.Write([]byte("Only POST request is allowed")); err != nil {
			log.Errorf("write response error: %v", err)
		}
		return
	}

	decoder := json.NewDecoder(r.Body)
	var data map[string]config.Store

	defer func() {
		if r := recover(); r != nil {
			w.WriteHeader(http.StatusBadRequest)
			var res []byte
			switch r.(type) {
			case string:
				res = []byte(r.(string))
			case error:
				res = []byte(r.(error).Error())
			default:
				return
			}

			if _, err := w.Write(res); err != nil {
				log.Errorf("write response error: %v", err)
			}
		}
	}()

	err := decoder.Decode(&data)
	if err != nil {
		panic(err)
	}

	if _, err := w.Write([]byte("OK")); err != nil {
		log.Errorf("write response error: %v", err)
	}
	config.ReloadConfig(data)
}

func PostLibHandler(rw http.ResponseWriter, request *http.Request) {
	postLibHandler(rw, request, libZipFileName)
	makeLibFS()
}

func makeLibFS() {
	lib, err := ioutil.ReadFile(libZipFileName)
	if err != nil {
		log.Warnf("No lib.zip file found, error: %v", err)
		return
	}

	zr, err := zip.NewReader(bytes.NewReader(lib), int64(len(lib)))
	if err != nil {
		log.Errorf("Can't make zip.Reader from lib.zip file, error: %v", err)
		return
	}
	rc := &zip.ReadCloser{
		Reader: *zr,
	}
	fsLocker.Lock()
	libFS = zipfs.New(rc, "lib")
	libZip = lib
	fsLocker.Unlock()
}

func makeAssetsFS() {
	lib, err := ioutil.ReadFile(assetsZipFileName)
	if err != nil {
		log.Warnf("No assets.zip file found, error: %v", err)
		return
	}
	zr, err := zip.NewReader(bytes.NewReader(lib), int64(len(lib)))
	if err != nil {
		log.Errorf("Can't make zip.Reader from assets.zip file, error: %v", err)
		return
	}

	rc := &zip.ReadCloser{
		Reader: *zr,
	}
	fsLocker.Lock()
	assetsFS = zipfs.New(rc, "lib")
	fsLocker.Unlock()
}

func postLibHandler(rw http.ResponseWriter, request *http.Request, fileName string) {
	buf := bytes.NewBuffer(nil)
	_, err := buf.ReadFrom(request.Body)
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		if _, err := rw.Write([]byte("can't read file")); err != nil {
			log.Debugf("[postLibHandler] write error: %v", err)
		}
		return
	}

	out, err := os.Create(fileName)
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		if _, err := rw.Write([]byte("can't create file")); err != nil {
			log.Debugf("[postLibHandler] write error: %v", err)
		}
		return
	}

	defer out.Close()
	written, err := io.Copy(out, buf)
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		if _, err := rw.Write([]byte("can't write file")); err != nil {
			log.Debugf("[postLibHandler] write error: %v", err)
		}
		return
	}

	log.Infof("new %s file created. Written %v bytes", fileName, written)
}

func init() {
	makeLibFS()
	makeAssetsFS()
}
