package internet

import (
	"net/http"

	"github.com/json-iterator/go"

	"github.com/getblank/blank-router/berrors"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

var (
	headerContentType        = "Content-Type"
	headerContentDisposition = "Content-Disposition"
	applicationJSON          = "application/json; charset=utf-8"
	textHTML                 = "text/html; charset=utf-8"
	applicationXML           = "application/xml; charset=utf-8"
)

func errorResponse(w http.ResponseWriter, status int, err error) {
	jsonResponseWithStatus(w, status, err.Error())
}

func invalidArguments(w http.ResponseWriter) {
	errorResponse(w, http.StatusBadRequest, berrors.ErrInvalidArguments)
}

func jsonResponse(w http.ResponseWriter, data interface{}) {
	jsonResponseWithStatus(w, http.StatusOK, data)
}

func jsonResponseWithStatus(w http.ResponseWriter, status int, data interface{}) {
	encoded, err := json.Marshal(data)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err)
		return
	}

	w.Header().Set(headerContentType, applicationJSON)
	w.WriteHeader(status)
	if _, err := w.Write(encoded); err != nil {
		log.Debugf("[jsonResponseWithStatus] write error: %v", err)
	}
}

func jsonBlobResponseWithStatus(w http.ResponseWriter, status int, data []byte) {
	w.Header().Set(headerContentType, applicationJSON)
	w.WriteHeader(status)
	if _, err := w.Write(data); err != nil {
		log.Debugf("[jsonBlobResponseWithStatus] write error: %v", err)
	}
}

func htmlResponse(w http.ResponseWriter, data string) {
	htmlResponseWithStatus(w, http.StatusOK, data)
}

func htmlResponseWithStatus(w http.ResponseWriter, status int, data string) {
	w.Header().Set(headerContentType, textHTML)
	w.WriteHeader(status)
	if _, err := w.Write([]byte(data)); err != nil {
		log.Debugf("[htmlResponseWithStatus] write error: %v", err)
	}
}

func redirectResponse(w http.ResponseWriter, location string) {
	redirectResponseWithStatus(w, http.StatusTemporaryRedirect, location)
}

func redirectResponseWithStatus(w http.ResponseWriter, status int, location string) {
	w.Header().Set("Location", location)
	w.WriteHeader(status)
}

func xmlResponseWithStatus(w http.ResponseWriter, status int, data []byte) {
	w.Header().Set(headerContentType, applicationXML)
	w.WriteHeader(status)
	if _, err := w.Write([]byte(data)); err != nil {
		log.Debugf("[xmlResponseWithStatus] write error: %v", err)
	}
}
