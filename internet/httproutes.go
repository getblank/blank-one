package internet

import (
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/go-chi/chi"
	"github.com/pkg/errors"

	"github.com/getblank/blank-router/taskq"
	"github.com/getblank/blank-sr/config"
	"github.com/getblank/uuid"

	"github.com/getblank/blank-one/sr"
)

const apiV1baseURI = "/api/v1/"

var (
	routesBuildingCompleted bool
	errUserIDNotFound       = errors.New("not found")
	paramConverterRGX       = regexp.MustCompile(":([a-zA-Z]+[a-zA-Z0-9]*)")
)

type result struct {
	Type     string            `json:"type"`
	Data     string            `json:"data"`
	RAWData  interface{}       `json:"rawData"`
	Code     int               `json:"code"`
	Header   map[string]string `json:"header"`
	FileName string            `json:"fileName"`
	FilePath string            `json:"filePath"`
	Store    string            `json:"store"`
	ID       string            `json:"_id"`
}

func onConfigUpdate(c map[string]config.Store) {
	wamp.Disconnect()
	log.Info("New config arrived")
	if routesBuildingCompleted {
		log.Warn("Routes already built. Need to restart if http hooks or actions modified.")
	}

	httpEnabledStores := []config.Store{}
	for s, store := range c {
		if !strings.HasPrefix(store.Store, "_") {
			httpEnabledStores = append(httpEnabledStores, store)
		}

		storeName := s
		groupURI := "/hooks/" + storeName
		var lowerGroupURI string
		var lowerGroup chi.Router
		if lowerStoreName := strings.ToLower(storeName); lowerStoreName != storeName {
			lowerGroupURI = "/hooks/" + lowerStoreName
			lowerGroup = r.Route(lowerGroupURI, nil)
		}

		group := r.Route(groupURI, nil)
		for i, hook := range store.HTTPHooks {
			hook.URI = convertHookURI(hook.URI)
			if len(hook.URI) == 0 {
				log.Error("Empty URI in hook", strconv.Itoa(i), " for "+groupURI+". Will ignored")
				continue
			}

			var handler func(pattern string, handlerFn http.HandlerFunc)
			var lowerHandler func(pattern string, handlerFn http.HandlerFunc)
			switch hook.Method {
			case "GET", "Get", "get":
				handler = group.Get
				if lowerGroupURI != "" {
					lowerHandler = lowerGroup.Get
				}
			case "POST", "Post", "post":
				handler = group.Post
				if lowerGroupURI != "" {
					lowerHandler = lowerGroup.Post
				}
			case "PUT", "Put", "put":
				handler = group.Put
				if lowerGroupURI != "" {
					lowerHandler = lowerGroup.Put
				}
			case "PATCH", "Patch", "patch":
				handler = group.Patch
				if lowerGroupURI != "" {
					lowerHandler = lowerGroup.Patch
				}
			case "DELETE", "Delete", "delete":
				handler = group.Delete
				if lowerGroupURI != "" {
					lowerHandler = lowerGroup.Delete
				}
			case "HEAD", "Head", "head":
				handler = group.Head
				if lowerGroupURI != "" {
					lowerHandler = lowerGroup.Head
				}
			case "OPTIONS", "Options", "options":
				handler = group.Options
				if lowerGroupURI != "" {
					lowerHandler = lowerGroup.Options
				}
			default:
				log.Warn("UNKNOWN HTTP METHOD. Will use GET method ", hook)
				handler = group.Get
				if lowerGroupURI != "" {
					lowerHandler = lowerGroup.Get
				}
			}

			hookIndex := i
			hookHandler := func(w http.ResponseWriter, r *http.Request) {
				t := taskq.Task{
					Store:  storeName,
					Type:   taskq.HTTPHook,
					UserID: "root",
					Arguments: map[string]interface{}{
						"request":   extractRequest(r),
						"hookIndex": hookIndex,
					},
				}
				_res, err := taskq.PushAndGetResult(&t, 0)
				if err != nil {
					errorResponse(w, http.StatusSeeOther, err)
					return
				}

				res, err := parseResult(_res)
				if err != nil {
					errorResponse(w, http.StatusInternalServerError, err)
					return
				}

				defaultResponse(w, res)
			}

			handler(hook.URI, hookHandler)
			log.Infof("Created '%s' httpHook for store '%s' with path %s", hook.Method, storeName, groupURI+hook.URI)
			if lowerHandler != nil {
				lowerHandler(hook.URI, hookHandler)
				log.Infof("Created '%s' httpHook on lower case for store '%s' with path %s", hook.Method, storeName, lowerGroupURI+hook.URI)
			}
		}

		if len(store.Actions) > 0 {
			createHTTPActions(storeName, store.Actions)
		}

		if len(store.StoreActions) > 0 {
			createHTTPActions(storeName, store.StoreActions)
		}

		if store.Type == "file" || store.Type == "files" {
			createFileHandlers(storeName)
		}
	}

	routesBuildingCompleted = true
	log.Info("Routes building complete")

	createRESTAPI(httpEnabledStores)
	log.Info("REST API building complete")
}

func createFileHandlers(storeName string) {
	groupURI := fmt.Sprintf("/files/%s", storeName)
	group := r.Route(groupURI, nil)

	group.With(jwtAuthMiddleware(false)).Post("/", postFileHandler(storeName))
	group.With(jwtAuthMiddleware(true)).Get("/:id", getFileHandler(storeName))
	group.With(jwtAuthMiddleware(false)).Post("/:id", postFileHandler(storeName))
	group.With(jwtAuthMiddleware(false)).Delete("/:id", deleteFileHandler(storeName))
	log.Infof("Created handlers for fileStore '%s' with path %s:id", storeName, groupURI)
}

func writeFileFromFileStore(w http.ResponseWriter, storeName, fileID, fileName string) {
	res, err := http.Get(fmt.Sprintf("%s/%s/%s", sr.FSAddress(), storeName, fileID))
	if err != nil {
		jsonResponseWithStatus(w, res.StatusCode, res.Status)
		return
	}

	defer res.Body.Close()
	if len(fileName) == 0 {
		if _, params, err := mime.ParseMediaType(res.Header.Get(headerContentDisposition)); err == nil && len(params["filename"]) > 0 {
			fileName = params["filename"]
		} else {
			fileName = res.Header.Get("File-Name")
		}
	}

	for k, v := range res.Header {
		if k == "File-Name" {
			continue
		}

		for _, h := range v {
			w.Header().Add(k, h)
		}
	}

	w.Header().Set(headerContentDisposition, fmt.Sprintf("attachment; filename=%s", fileName))
	body, _ := ioutil.ReadAll(res.Body)
	if _, err := w.Write(body); err != nil {

	}
}

func createHTTPActions(storeName string, actions []config.Action) {
	groupURI := "/actions/" + storeName + "/"
	group := r.Route(groupURI, nil)
	for _, v := range actions {
		actionID := v.ID
		handler := func(w http.ResponseWriter, r *http.Request) {
			c := r.Context().Value(credKey)
			if c == nil {
				log.Warn("HTTP ACTION: no cred in echo context")
				jsonResponseWithStatus(w, http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized))
				return
			}

			cred, ok := c.(credentials)
			if !ok {
				log.Warn("HTTP ACTION: invalid cred in echo context")
				jsonResponseWithStatus(w, http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized))
				return
			}

			t := taskq.Task{
				Store:  storeName,
				Type:   taskq.DbAction,
				UserID: cred.userID,
				Arguments: map[string]interface{}{
					"request":  extractRequest(r),
					"actionId": actionID,
				},
			}

			if cred.claims != nil {
				t.Arguments["tokenInfo"] = cred.claims.toMap()
			}

			if itemID := r.URL.Query().Get("item-id"); len(itemID) > 0 {
				t.Arguments["itemId"] = itemID
			}

			_res, err := taskq.PushAndGetResult(&t, 0)
			if err != nil {
				errorResponse(w, http.StatusSeeOther, err)
				return
			}

			res, err := parseResult(_res)
			if err != nil {
				errorResponse(w, http.StatusInternalServerError, err)
				return
			}

			defaultResponse(w, res)
		}

		if v.Type == "http" {
			group.With(jwtAuthMiddleware(false)).Get(actionID, handler)
		} else {
			group.With(jwtAuthMiddleware(false)).Post(actionID, handler)
		}

		log.Infof("Registered httpAction for store '%s' with path %s", storeName, groupURI+v.ID)
	}
}

func extractRequest(r *http.Request) map[string]interface{} {
	routeCtx := chi.RouteContext(r.Context())
	urlParams := routeCtx.URLParams
	params := map[string]string{}
	for _, p := range urlParams.Keys {
		params[p] = routeCtx.URLParam(p)
	}

	header := map[string]string{}
	for k, v := range r.Header {
		if len(v) > 0 {
			header[k] = v[0]
		}
	}

	if err := r.ParseMultipartForm(1024); err != nil {
		if err := r.ParseForm(); err != nil {
		}
	}

	formParams := r.PostForm
	var data interface{}
	if d := formParams.Get("data"); len(d) > 0 {
		data = d
	}

	var body string
	b, err := ioutil.ReadAll(r.Body)
	if err != nil && err != io.EOF {
		log.Errorf("Can't read request http body. Error: %v", err)
	} else {
		if rtype := header["Content-Type"]; strings.HasPrefix(rtype, "application/json") || strings.HasPrefix(rtype, "text/plain") {
			body = string(b)
		} else {
			body = base64.StdEncoding.EncodeToString(b)
		}
	}

	return map[string]interface{}{
		"params":  params,
		"query":   r.URL.Query(),
		"form":    formParams,
		"ip":      r.RemoteAddr,
		"referer": r.Referer(),
		"header":  header,
		"body":    body,
		"data":    data,
	}
}

func defaultResponse(w http.ResponseWriter, res *result) {
	if res == nil {
		jsonResponse(w, http.StatusText(http.StatusOK))
	}

	code := res.Code
	if code == 0 {
		code = http.StatusOK
	}

	for k, v := range res.Header {
		w.Header().Set(k, v)
	}

	switch res.Type {
	case "REDIRECT", "redirect":
		if code == 200 {
			code = http.StatusFound
		}

		redirectResponseWithStatus(w, code, res.Data)
		return
	case "JSON", "json":
		if res.RAWData == nil {
			jsonBlobResponseWithStatus(w, code, []byte(res.Data))
			return
		}

		jsonResponseWithStatus(w, code, res.RAWData)
		return
	case "HTML", "html":
		htmlResponseWithStatus(w, code, res.Data)
		return
	case "XML", "xml":
		xmlResponseWithStatus(w, code, []byte(res.Data))
		return
	case "file":
		responseFile(w, res)
		return
	default:
		jsonResponseWithStatus(w, http.StatusSeeOther, "unknown encoding type")
		return
	}
}

func responseFile(w http.ResponseWriter, res *result) {
	if len(res.Store) > 0 && len(res.ID) > 0 {
		writeFileFromFileStore(w, res.Store, res.ID, res.FileName)
		return
	}

	var buffer []byte
	var err error
	if len(res.FilePath) > 0 {
		buffer, err = ioutil.ReadFile(res.FilePath)
	} else {
		buffer, err = base64.StdEncoding.DecodeString(res.Data)
	}

	if err != nil {
		jsonResponseWithStatus(w, http.StatusInternalServerError, fmt.Sprintf("can't read file, error: %v", err))
		return
	}

	w.Header().Set(headerContentDisposition, fmt.Sprintf("attachment; filename=%s", res.FileName))
	w.Header().Set(headerContentType, detectContentType(res.FileName, buffer))
	if _, err := w.Write(buffer); err != nil {
	}
}

func parseResult(in interface{}) (*result, error) {
	if in == nil {
		return nil, nil
	}

	encoded, err := json.Marshal(in)
	if err != nil {
		return nil, errors.Wrap(err, "when parse result")
	}

	res := new(result)
	err = json.Unmarshal(encoded, res)
	if err != nil {
		err = errors.Wrap(err, "when unmarshal result")
	}

	return res, err
}

func getFileHandler(storeName string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		c := r.Context().Value(credKey)
		if c == nil {
			log.Warn("[file get]: no cred in echo context")
			jsonResponseWithStatus(w, http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized))
			return
		}

		cred, ok := c.(credentials)
		if !ok {
			log.Warn("[file get]: invalid cred in echo context")
			jsonResponseWithStatus(w, http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized))
			return
		}

		fileID := chi.URLParam(r, "id")
		t := taskq.Task{
			Type:      taskq.DbGet,
			UserID:    cred.userID,
			Store:     storeName,
			Arguments: map[string]interface{}{"_id": fileID},
		}
		if cred.claims != nil {
			t.Arguments["tokenInfo"] = cred.claims.toMap()
		}

		_, err := taskq.PushAndGetResult(&t, 0)
		if err != nil {
			errorResponse(w, http.StatusBadRequest, err)
			return
		}

		writeFileFromFileStore(w, storeName, fileID, "")
	}
}

func postFileHandler(storeName string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		c := r.Context().Value(credKey)
		if c == nil {
			log.Warn("[file post]: no cred in echo context")
			jsonResponseWithStatus(w, http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized))
			return
		}

		cred, ok := c.(credentials)
		if !ok {
			log.Warn("[file post]: invalid cred in echo context")
			jsonResponseWithStatus(w, http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized))
			return
		}

		fileID := chi.URLParam(r, "id")
		if fileID == "" {
			fileID = uuid.NewV4()
		}

		_, fileHeader, err := r.FormFile("file")
		if err != nil {
			jsonResponseWithStatus(w, http.StatusBadRequest, http.StatusText(http.StatusBadRequest))
			return
		}

		fileName := fileHeader.Filename
		t := taskq.Task{
			Type:      taskq.DbSet,
			UserID:    cred.userID,
			Store:     storeName,
			Arguments: map[string]interface{}{"item": map[string]string{"_id": fileID, "name": fileName}},
		}
		if cred.claims != nil {
			t.Arguments["tokenInfo"] = cred.claims.toMap()
		}

		_, err = taskq.PushAndGetResult(&t, 0)
		if err != nil {
			errorResponse(w, http.StatusBadRequest, err)
			return
		}

		file, err := fileHeader.Open()
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err)
			return
		}
		defer file.Close()

		req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/%s/%s", sr.FSAddress(), storeName, fileID), file)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err)
			return
		}

		req.Header.Set("File-Name", fileName)
		req.Header.Set(headerContentDisposition, fmt.Sprintf(`attachment; filename="%s"`, fileName))
		client := &http.Client{}
		_, err = client.Do(req)
		if err != nil {
			errorResponse(w, http.StatusInternalServerError, err)
			return
		}

		jsonResponse(w, fileID)
	}
}

func deleteFileHandler(storeName string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		c := r.Context().Value(credKey)
		if c == nil {
			log.Warn("[file delete]: no cred in echo context")
			jsonResponseWithStatus(w, http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized))
			return
		}

		cred, ok := c.(credentials)
		if !ok {
			log.Warn("[file delete]: invalid cred in echo context")
			jsonResponseWithStatus(w, http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized))
			return
		}

		fileID := chi.URLParam(r, "id")
		t := taskq.Task{
			Type:      taskq.DbDelete,
			UserID:    cred.userID,
			Store:     storeName,
			Arguments: map[string]interface{}{"item": map[string]string{"_id": fileID}},
		}
		if cred.claims != nil {
			t.Arguments["tokenInfo"] = cred.claims.toMap()
		}

		_, err := taskq.PushAndGetResult(&t, 0)
		if err != nil {
			errorResponse(w, http.StatusBadRequest, err)
			return
		}

		jsonResponse(w, http.StatusText(http.StatusOK))
	}
}

func convertHookURI(uri string) string {
	matched := paramConverterRGX.FindAllStringSubmatch(uri, -1)
	for _, v := range matched {
		uri = strings.Replace(uri, v[0], fmt.Sprintf("{%s}", v[1]), 1)
	}

	if !strings.HasPrefix(uri, "/") {
		uri = "/" + uri
	}

	return uri
}
