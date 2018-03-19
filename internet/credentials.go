package internet

import (
	"net/http"
)

type credentials struct {
	userID    interface{}
	sessionID string
	claims    *blankClaims
}

func clearBlankToken(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: "access_token", Value: "deleted", Path: "/", MaxAge: -1})
}
