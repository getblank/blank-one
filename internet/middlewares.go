package internet

import (
	"fmt"
	"net/http"
)

func allowAnyOriginMiddleware(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}

func versionMiddleware(version string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			var commit, buildTime string // TODO: get info from application config
			versionString := fmt.Sprintf("Blank Router/%s (https://getblank.net). Config build time: %s, git hash: %s.", version, buildTime, commit)
			w.Header().Set("Server", versionString)
			next.ServeHTTP(w, r)
		}

		return http.HandlerFunc(fn)
	}
}
