package internet

import (
	"net/http"

	"github.com/json-iterator/go"

	"github.com/getblank/blank-router/berrors"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

var (
	headerContentType     = "Content-Type"
	applicationJavascript = "application/javascript; charset=utf-8"
	textHTML              = "text/html; charset=utf-8"
)

func errorResponse(w http.ResponseWriter, status int, err error) {
	w.Header().Set(headerContentType, applicationJavascript)
	w.WriteHeader(status)
	w.Write([]byte(`"` + err.Error() + `"`))
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

	w.Header().Set(headerContentType, applicationJavascript)
	w.WriteHeader(status)
	if _, err := w.Write(encoded); err != nil {

	}
}

func htmlResponse(w http.ResponseWriter, data string) {
	htmlResponseWithStatus(w, http.StatusOK, data)
}

func htmlResponseWithStatus(w http.ResponseWriter, status int, data string) {
	w.Header().Set(headerContentType, textHTML)
	w.WriteHeader(status)
	if _, err := w.Write([]byte(data)); err != nil {

	}
}

func redirectResponse(w http.ResponseWriter, location string) {
	w.Header().Set("Location", location)
	w.WriteHeader(http.StatusTemporaryRedirect)
}
