package internet

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/getblank/blank-one/sessions"
)

type ctxKey string

const credKey ctxKey = "cred"

// ErrSessionNotFound error
var ErrSessionNotFound = errors.New("session not found")

func allowAnyOriginMiddleware(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}

func jwtAuthMiddleware(allowGuests bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			accessToken := extractToken(r)
			if len(accessToken) == 0 {
				if allowGuests {
					ctx = context.WithValue(ctx, credKey, credentials{userID: "guest"})
					next.ServeHTTP(w, r.WithContext(ctx))

					return
				}

				jsonResponseWithStatus(w, http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized))

				return
			}

			claims, err := extractClaimsFromJWT(accessToken)
			if err != nil {
				errorResponse(w, http.StatusForbidden, err)
				return
			}

			_, err = sessions.CheckSession(claims.SessionID)
			if err != nil {
				errorResponse(w, http.StatusForbidden, ErrSessionNotFound)
				return
			}

			ctx = context.WithValue(ctx, credKey, credentials{userID: claims.UserID, sessionID: claims.SessionID, claims: claims})
			next.ServeHTTP(w, r.WithContext(ctx))
		}

		return http.HandlerFunc(fn)
	}
}

func versionMiddleware(version string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			var commit, buildTime string // TODO: get info from application config
			versionString := fmt.Sprintf("Blank One/%s (https://getblank.net). Config build time: %s, git hash: %s.", version, buildTime, commit)
			w.Header().Set("Server", versionString)
			next.ServeHTTP(w, r)
		}

		return http.HandlerFunc(fn)
	}
}
