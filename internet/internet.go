package internet

import (
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"

	"github.com/getblank/blank-router/berrors"
	"github.com/getblank/blank-router/taskq"
	"github.com/getblank/uuid"

	"github.com/getblank/blank-one/appconfig"
	"github.com/getblank/blank-one/sessions"
)

var (
	port = "8080"
	r    = chi.NewRouter()
)

func Init(version string) {
	if p := os.Getenv("BLANK_HTTP_PORT"); p != "" {
		port = p
	}

	initMiddlewares(version)
	initBaseRoutes()

	r.Get("/preved", func(w http.ResponseWriter, r *http.Request) {
		uri := r.URL.Path
		w.Write([]byte("welcome " + uri))
	})

	r.Get("/preved/{id}", func(w http.ResponseWriter, r *http.Request) {
		uri := r.URL.Path
		id := chi.URLParam(r, "id")
		w.Write([]byte("parameters " + uri + " " + id))
	})

	log.Info("Init internet server on port ", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}

func initMiddlewares(version string) {
	r.Use(middleware.StripSlashes)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))
	r.Use(versionMiddleware(version))
}

func initBaseRoutes() {
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		redirectResponse(w, "app/")
	})

	r.With(onlyGet).Get("/*", assetsHandler)

	r.Get("/public-key", func(w http.ResponseWriter, r *http.Request) {
		w.Write(sessions.PublicKeyBytes())
	})

	r.Get("/common-settings", commonSettingsHandler)

	r.With(allowAnyOriginMiddleware).Post("/login", loginHandler)
	r.With(allowAnyOriginMiddleware).Post("/logout", logoutHandler)
	r.With(allowAnyOriginMiddleware).Get("/logout", logoutHandler)
	r.With(allowAnyOriginMiddleware).Post("/register", registerHandler)
	r.With(allowAnyOriginMiddleware).Post("/check-user", checkUserHandler)
	r.With(allowAnyOriginMiddleware).Post("/send-reset-link", sendResetLinkHandler)
	r.With(allowAnyOriginMiddleware).Post("/reset-password", resetPasswordHandler)
	r.With(allowAnyOriginMiddleware).Post("/check-jwt", checkJWTHandler)
	r.With(allowAnyOriginMiddleware).Get("/check-jwt", checkJWTHandler)
	r.With(allowAnyOriginMiddleware).Options("/check-jwt", checkJWTOptionsHandler)

	r.With(allowAnyOriginMiddleware).Get("/sso-frame", ssoFrameHandler)
}

func onlyGet(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			w.WriteHeader(http.StatusMethodNotAllowed)
			if _, err := w.Write([]byte("Allow: GET")); err != nil {

			}

			return
		}

		next.ServeHTTP(w, r)
	})
}

func assetsHandler(w http.ResponseWriter, r *http.Request) {
	uriPath := r.URL.Path
	var uri string
	assetsRequest := strings.HasPrefix(uriPath, "/app/assets")
	if uriPath == "/" || (strings.HasPrefix(uriPath, "/app") && !assetsRequest) {
		uri = "/assets/blank/index.html"
	} else {
		uri = strings.TrimPrefix(uriPath, "/app")
	}

	if len(path.Ext(uri)) == 0 {
		uri += "/index.html"
	}

	appconfig.GetAsset(w, uri)
}

func checkJWTHandler(w http.ResponseWriter, r *http.Request) {
	res := map[string]interface{}{"valid": false}
	var valid bool
	publicRSAKey := sessions.PublicKey()
	if publicRSAKey == nil {
		log.Warn("JWT is not ready yet")
		jsonResponseWithStatus(w, http.StatusOK, res)
		return
	}

	if token := extractToken(r); token != "" {
		if claims, err := extractClaimsFromJWT(token); err == nil {
			if _, err = sessions.CheckSession(claims.SessionID); err == nil {
				res["valid"] = true
				user := map[string]interface{}{}
				user["_id"] = claims.UserID
				for k, v := range claims.Extra {
					user[k] = v
				}

				res["user"] = user
				valid = true
			}
		}
	}

	if !valid {
		if cookie, err := r.Cookie("access_token"); err == nil {
			cookie.Value = ""
			cookie.MaxAge = -1
			http.SetCookie(w, cookie)
		}

		jsonResponseWithStatus(w, http.StatusForbidden, res)
		return
	}

	jsonResponse(w, res)
}

func checkJWTOptionsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Method", "GET, POST")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
}

func checkUserHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(1024); err != nil {
		if err := r.ParseForm(); err != nil {
			invalidArguments(w)
			return
		}
	}

	form := r.PostForm
	email := form.Get("email")
	if email == "" {
		invalidArguments(w)
		return
	}

	t := taskq.Task{
		Type:   taskq.DbFind,
		UserID: "root",
		Store:  "users",
		Arguments: map[string]interface{}{
			"query": map[string]interface{}{
				"query": map[string]interface{}{
					"email": email,
				},
				"props": []string{"_id"},
			},
		},
	}

	_res, err := taskq.PushAndGetResult(&t, 0)
	if err != nil {
		jsonResponse(w, "USER_NOT_FOUND")
		return
	}

	res, ok := _res.(map[string]interface{})
	if !ok {
		errorResponse(w, http.StatusInternalServerError, berrors.ErrError)
		return
	}

	_items, ok := res["items"]
	if !ok {
		errorResponse(w, http.StatusInternalServerError, berrors.ErrError)
		return
	}

	items, ok := _items.([]interface{})
	if !ok {
		errorResponse(w, http.StatusInternalServerError, berrors.ErrError)
		return
	}

	if len(items) > 0 {
		jsonResponse(w, "USER_EXISTS")
		return
	}

	jsonResponse(w, "USER_NOT_FOUND")
}

func commonSettingsHandler(w http.ResponseWriter, r *http.Request) {
	t := taskq.Task{
		Type:      taskq.UserConfig,
		Arguments: map[string]interface{}{},
	}

	lang := r.URL.Query().Get("lang")
	if lang != "" {
		t.Arguments = map[string]interface{}{
			"lang": lang,
		}
	}

	resChan := taskq.Push(&t)

	res := <-resChan
	if res.Err != "" {
		jsonResponseWithStatus(w, http.StatusNotFound, res.Err)
		return
	}

	jsonResponse(w, res.Result)
}

func facebookLoginHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := uuid.NewV4()
	t := taskq.Task{
		Type: taskq.Auth,
		Arguments: map[string]interface{}{
			"social":      "facebook",
			"code":        r.URL.Query().Get("code"),
			"redirectUrl": r.URL.Query().Get("redirectUrl"),
			"sessionID":   sessionID,
		},
	}
	res, err := taskq.PushAndGetResult(&t, 0)
	if err != nil {
		htmlResponseWithStatus(w, http.StatusSeeOther, err.Error())
		return
	}

	user, ok := res.(map[string]interface{})
	if !ok {
		log.WithField("result", res).Warn("Invalid type of result on http login")
		htmlResponseWithStatus(w, http.StatusInternalServerError, berrors.ErrError.Error())
		return
	}

	apiKey, err := sessions.NewSession(user, sessionID)
	if err != nil {
		htmlResponseWithStatus(w, http.StatusInternalServerError, err.Error())
		return
	}

	claims, err := extractClaimsFromJWT(apiKey)
	if err != nil {
		log.Warnf("Can't parse JWT error: '%s', token: '%s'", err.Error(), apiKey)
	}

	expiresAt := strconv.Itoa(int(claims.ExpiresAt))

	result := `<script>
		(function(){
			localStorage.setItem("blank-access-token", "` + apiKey + `");
			createCookie("blank-token", "` + apiKey + `", ` + expiresAt + `);
			var redirectUrl = location.search.match(/redirectUrl=([^&]*)&?/);
			if (redirectUrl) {
				window.location = decodeURIComponent(redirectUrl[1]);
				return;
			}
			window.location = location.protocol + "//" + location.host;

			function createCookie(name, value, expiresAt) {
				var cookie = "" + name + "=" + value,
					deleting = expiresAt === -1,
					expires = "";
				if (expiresAt) {
					expires = "; expires=" + new Date(deleting ? 0 : expiresAt * 1000).toGMTString();
				}

				const hostname = document.location.hostname.split(".");
				for (var i = hostname.length - 1; i >= 0; i--) {
					const h = hostname.slice(i).join(".");
					document.cookie = cookie + expires + "; path=\/; domain=." + h + ";";
					if (!deleting && document.cookie.indexOf(cookie) > -1) {
						return;
					}
				}
			}
		}());
	</script>`

	jsonResponse(w, result)
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(1024); err != nil {
		if err := r.ParseForm(); err != nil {
			invalidArguments(w)
			return
		}
	}

	form := r.PostForm
	login := form.Get("login")
	password := form.Get("password")
	hashedPassword := form.Get("hashedPassword")
	if len(login) == 0 || (len(password) == 0 && len(hashedPassword) == 0) {
		invalidArguments(w)
		return
	}

	fp := map[string]interface{}{}
	for k := range form {
		fp[k] = form.Get(k)
	}

	sessionID := uuid.NewV4()
	fp["sessionID"] = sessionID
	t := taskq.Task{
		Type:      taskq.Auth,
		Arguments: fp,
	}

	res, err := taskq.PushAndGetResult(&t, 0)
	if err != nil {
		errorResponse(w, http.StatusForbidden, err)
		return
	}

	user, ok := res.(map[string]interface{})
	if !ok {
		log.WithField("result", res).Warn("Invalid type of result on http login")
		errorResponse(w, http.StatusInternalServerError, berrors.ErrError)
		return
	}

	accessToken, err := sessions.NewSession(user, sessionID)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err)
		return
	}

	claims, err := extractClaimsFromJWT(accessToken)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err)
		return
	}

	accessTokenCookie := &http.Cookie{
		Name:     "access_token",
		Value:    accessToken,
		Expires:  time.Unix(claims.ExpiresAt, 0),
		Path:     "/",
		HttpOnly: true,
	}
	http.SetCookie(w, accessTokenCookie)

	result := map[string]interface{}{
		"access_token": accessToken,
		"user":         user,
	}

	jsonResponse(w, result)
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	accessToken := extractToken(r)
	if len(accessToken) == 0 {
		jsonResponse(w, http.StatusText(http.StatusOK))
		return
	}

	claims, err := extractClaimsFromJWT(accessToken)
	if err != nil {
		clearBlankToken(w)
		jsonResponse(w, http.StatusText(http.StatusOK))
		return
	}

	apiKey, userID := claims.SessionID, claims.UserID
	arguments := map[string]interface{}{"userId": userID, "sessionId": apiKey}
	for k, v := range claims.Extra {
		arguments[k] = v
	}

	t := taskq.Task{
		Type:      taskq.SignOut,
		UserID:    userID,
		Arguments: arguments,
	}
	_, err = taskq.PushAndGetResult(&t, 30*time.Second)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err)
		return
	}

	_, err = sessions.CheckSession(apiKey)
	if err != nil {
		log.Warnf("srClient.CheckSession for apiKey %s error: %v", apiKey, err)
		jsonResponse(w, http.StatusText(http.StatusOK))
		return
	}

	err = sessions.DeleteSession(apiKey)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, err)
		return
	}

	go func() {
		t := taskq.Task{
			Type:      taskq.DidSignOut,
			UserID:    userID,
			Arguments: arguments,
		}
		_, err = taskq.PushAndGetResult(&t, 30*time.Second)
		if err != nil {
			log.Errorf("User %s didSignOut error: %v", userID, err)
		}
	}()

	if redirectURL := r.URL.Query().Get("redirectUrl"); redirectURL != "" {
		clearBlankToken(w)
		redirectResponse(w, redirectURL)
		return
	}

	clearBlankToken(w)
	jsonResponse(w, http.StatusText(http.StatusOK))
}

func registerHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(1024); err != nil {
		if err := r.ParseForm(); err != nil {
			invalidArguments(w)
			return
		}
	}

	formParams := r.PostForm
	args := map[string]interface{}{"redirectUrl": r.URL.Query().Get("redirectUrl")}
	for k := range formParams {
		args[k] = formParams.Get(k)
	}

	t := taskq.Task{
		Type:      taskq.SignUp,
		Arguments: args,
	}

	res, err := taskq.PushAndGetResult(&t, 0)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, err)
		return
	}

	jsonResponse(w, res)
}

func resetPasswordHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(1024); err != nil {
		if err := r.ParseForm(); err != nil {
			invalidArguments(w)
			return
		}
	}

	formParams := r.PostForm
	args := map[string]interface{}{}
	for k := range formParams {
		args[k] = formParams.Get(k)
	}

	t := taskq.Task{
		Type:      taskq.PasswordReset,
		Arguments: args,
	}

	res, err := taskq.PushAndGetResult(&t, 0)
	if err != nil {
		errorResponse(w, http.StatusSeeOther, err)
		return
	}

	jsonResponse(w, res)
}

func sendResetLinkHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(1024); err != nil {
		if err := r.ParseForm(); err != nil {
			invalidArguments(w)
			return
		}
	}

	formParams := r.PostForm
	email := formParams.Get("email")
	if email == "" {
		invalidArguments(w)
		return
	}

	t := taskq.Task{
		Type: taskq.PasswordResetRequest,
		Arguments: map[string]interface{}{
			"email": email,
		},
	}
	res, err := taskq.PushAndGetResult(&t, 0)
	if err != nil {
		errorResponse(w, http.StatusSeeOther, err)
		return
	}

	jsonResponse(w, res)
}
