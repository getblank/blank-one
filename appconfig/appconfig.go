package appconfig

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"

	log "github.com/Sirupsen/logrus"
	"golang.org/x/tools/godoc/vfs"
	"golang.org/x/tools/godoc/vfs/zipfs"

	"github.com/getblank/blank-sr/config"
)

const (
	libZipFileName    = "lib.zip"
	assetsZipFileName = "assets.zip"
)

var (
	errLibCreateError = errors.New("Error saving uploaded file")

	libFS    vfs.FileSystem
	assetsFS vfs.FileSystem
	libZip   []byte
	fsLocker sync.RWMutex
)

func GetAsset(w http.ResponseWriter, filePath string) {
	fsLocker.RLock()
	if assetsFS == nil {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("file not found"))
		fsLocker.RUnlock()
		return
	}

	b, err := vfs.ReadFile(assetsFS, strings.TrimPrefix(filePath, "/assets"))
	fsLocker.RUnlock()
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(err.Error()))
		return
	}

	contentType := mime.TypeByExtension(path.Ext(filePath))
	if len(contentType) == 0 {
		contentType = http.DetectContentType(b)
	}

	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	w.Write(b)
}

func PostAssetsHandler(rw http.ResponseWriter, request *http.Request) {
	postLibHandler(rw, request, assetsZipFileName)
}

func GetLibZip() []byte {
	fsLocker.RLock()
	defer fsLocker.RUnlock()

	return libZip[:]
}

func PostConfigHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Only POST request is allowed"))
		return
	}
	decoder := json.NewDecoder(r.Body)
	var data map[string]config.Store

	defer func() {
		if r := recover(); r != nil {
			w.WriteHeader(http.StatusBadRequest)
			switch r.(type) {
			case string:
				w.Write([]byte(r.(string)))
			case error:
				w.Write([]byte(r.(error).Error()))
			}
		}
	}()

	err := decoder.Decode(&data)
	if err != nil {
		panic(err)
	}

	w.Write([]byte("OK"))
	config.ReloadConfig(data)
}

func PostLibHandler(rw http.ResponseWriter, request *http.Request) {
	postLibHandler(rw, request, libZipFileName)
}

func makeLibFS() {
	lib, err := ioutil.ReadFile(libZipFileName)
	if err != nil {
		log.WithError(err).Warn("No lib.zip file found")
		return
	}

	zr, err := zip.NewReader(bytes.NewReader(lib), int64(len(lib)))
	if err != nil {
		log.WithError(err).Error("Can't make zip.Reader from lib.zip file ")
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
		log.WithError(err).Warn("No assets.zip file found")
		return
	}
	zr, err := zip.NewReader(bytes.NewReader(lib), int64(len(lib)))
	if err != nil {
		log.WithError(err).Error("Can't make zip.Reader from assets.zip file ")
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
		rw.Write([]byte("can't read file"))
		return
	}

	out, err := os.Create(fileName)
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		rw.Write([]byte("can't create file"))
		return
	}

	defer out.Close()
	written, err := io.Copy(out, buf)
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		rw.Write([]byte("can't write file"))
		return
	}

	log.Infof("new %s file created. Written %v bytes", fileName, written)
	// wamp.Publish("config", config.Get())
}

func init() {
	makeLibFS()
	makeAssetsFS()
}
