package internet

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi"

	"github.com/getblank/blank-router/berrors"
	"github.com/getblank/blank-router/taskq"
	"github.com/getblank/blank-sr/config"
	"github.com/getblank/uuid"
)

func createRESTAPI(httpEnabledStores []config.Store) {
	if len(httpEnabledStores) == 0 {
		return
	}

	t := taskq.Task{
		Type:   taskq.DbAction,
		Store:  "_serverSettings",
		UserID: "root",
		Arguments: map[string]interface{}{
			"actionId": "restdoc",
			"data":     httpEnabledStores,
		},
	}

	res, err := taskq.PushAndGetResult(&t, 0)
	if err != nil {
		log.Errorf("Can't compile REST docs, error: %v", err)
		return
	}

	html, ok := res.(string)
	if !ok {
		log.Error("Invalid response type from doc compiler")
		return
	}

	r.Get(apiV1baseURI[:len(apiV1baseURI)-1], func(w http.ResponseWriter, r *http.Request) {
		htmlResponse(w, html)
	})

	log.Info("REST API Documentation generated")

	for _, store := range httpEnabledStores {
		createRESTAPIForStore(store)
	}
}

func createRESTAPIForStore(store config.Store) {
	baseURI := apiV1baseURI + store.Store
	lowerBaseURI := strings.ToLower(baseURI)

	gr := r.With(allowAnyOriginMiddleware, jwtAuthMiddleware(true))
	gr.Get(baseURI, restGetAllDocumentsHandler(store.Store))
	log.Debugf("Created GET all REST method %s", baseURI)
	if baseURI != lowerBaseURI {
		gr.Get(lowerBaseURI, restGetAllDocumentsHandler(store.Store))
		log.Debugf("Created GET all REST method %s", lowerBaseURI)
	}

	r := r.With(allowAnyOriginMiddleware, jwtAuthMiddleware(false))
	r.Post(baseURI, restPostDocumentHandler(store.Store))
	log.Debugf("Created POST REST method %s", baseURI)

	if baseURI != lowerBaseURI {
		r.Post(lowerBaseURI, restPostDocumentHandler(store.Store))
		log.Debugf("Created POST REST method %s", lowerBaseURI)
	}

	itemURI := baseURI + "/{id}"
	lowerItemURI := lowerBaseURI + "/{id}"
	r.Get(itemURI, restGetDocumentHandler(store.Store))
	log.Debugf("Created GET REST method %s", itemURI)
	if itemURI != lowerItemURI {
		r.Get(lowerItemURI, restGetDocumentHandler(store.Store))
		log.Debugf("Created GET REST method %s", lowerItemURI)
	}

	r.Put(itemURI, restPutDocumentHandler(store.Store))
	log.Debugf("Created PUT REST method %s", itemURI)
	if itemURI != lowerItemURI {
		r.Put(lowerItemURI, restPutDocumentHandler(store.Store))
		log.Debugf("Created PUT REST method %s", lowerItemURI)
	}

	r.Delete(itemURI, restDeleteDocumentHandler(store.Store))
	log.Debugf("Created DELETE REST method %s", itemURI)
	if itemURI != lowerItemURI {
		r.Delete(lowerItemURI, restDeleteDocumentHandler(store.Store))
		log.Debugf("Created DELETE REST method %s", lowerItemURI)
	}

	for _, a := range store.Actions {
		actionURI := itemURI + "/" + a.ID
		lowerActionURI := lowerItemURI + "/" + strings.ToLower(a.ID)
		r.Post(actionURI, restActionHandler(store.Store, a.ID))
		log.Debugf("Created POST action REST method %s", actionURI)
		if actionURI != lowerActionURI {
			r.Post(lowerActionURI, restActionHandler(store.Store, a.ID))
			log.Debugf("Created POST action REST method %s", lowerActionURI)
		}
	}

	for _, a := range store.StoreActions {
		actionURI := baseURI + "/" + a.ID
		lowerActionURI := lowerBaseURI + "/" + strings.ToLower(a.ID)
		r.Post(actionURI, restActionHandler(store.Store, a.ID))
		log.Debugf("Created POST storeAction REST method %s", actionURI)
		if actionURI != lowerActionURI {
			r.Post(lowerActionURI, restActionHandler(store.Store, a.ID))
			log.Debugf("Created POST storeAction REST method %s", lowerActionURI)
		}
	}
}

func restActionHandler(storeName, actionID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := r.Context().Value(credKey)
		if c == nil {
			log.Warn("[rest action]: no cred in echo context")
			jsonResponseWithStatus(w, http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized))
			return
		}

		cred, ok := c.(credentials)
		if !ok {
			log.Warn("[rest action]: invalid cred in echo context")
			jsonResponseWithStatus(w, http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized))
			return
		}

		log.Debugf("REST ACTION: store: %s, actionID: %s. credentials extracted", storeName, actionID)
		t := taskq.Task{
			Type:   taskq.DbAction,
			Store:  storeName,
			UserID: cred.userID,
			Arguments: map[string]interface{}{
				"itemId":   chi.URLParam(r, "id"),
				"actionId": actionID,
				"request":  extractRequest(r),
			},
		}
		if cred.claims != nil {
			t.Arguments["tokenInfo"] = cred.claims.toMap()
		}

		res, err := taskq.PushAndGetResult(&t, 0)
		if err != nil {
			errText := err.Error()
			if strings.EqualFold(errText, "not found") {
				jsonResponseWithStatus(w, http.StatusNotFound, errText)
				return
			}

			fields := strings.SplitN(errText, " ", 2)
			statusCode := http.StatusInternalServerError
			if len(fields) > 1 {
				if i, err := strconv.Atoi(fields[0]); err == nil {
					statusCode = i
					errText = fields[1]
				}
			}

			jsonResponseWithStatus(w, statusCode, errText)
			return
		}

		jsonResponse(w, res)
	}
}

func restGetAllDocumentsHandler(storeName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := r.Context().Value(credKey)
		if c == nil {
			log.Warn("[rest get all]: no cred in echo context")
			jsonResponseWithStatus(w, http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized))
			return
		}

		cred, ok := c.(credentials)
		if !ok {
			log.Warn("[rest get all]: invalid cred in echo context")
			jsonResponseWithStatus(w, http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized))
			return
		}

		var query map[string]interface{}
		if q := r.URL.Query().Get("query"); len(q) > 0 {
			err := json.Unmarshal([]byte(q), &query)
			if err != nil {
				errorResponse(w, http.StatusBadRequest, err)
				return
			}
		}

		findQuery := map[string]interface{}{"query": query, "skip": 0, "take": 10}
		if s := r.URL.Query().Get("skip"); len(s) > 0 {
			var skip int
			skip, err := strconv.Atoi(s)
			if err != nil {
				errorResponse(w, http.StatusBadRequest, err)
				return
			}

			findQuery["skip"] = skip
		}

		if t := r.URL.Query().Get("take"); len(t) > 0 {
			var take int
			take, err := strconv.Atoi(t)
			if err != nil {
				errorResponse(w, http.StatusBadRequest, err)
				return
			}

			findQuery["take"] = take
		}

		if orderBy := r.URL.Query().Get("orderBy"); len(orderBy) > 0 {
			findQuery["orderBy"] = orderBy
		}

		t := taskq.Task{
			Type:   taskq.DbFind,
			UserID: cred.userID,
			Store:  storeName,
			Arguments: map[string]interface{}{
				"query": findQuery,
			},
		}
		if cred.claims != nil {
			t.Arguments["tokenInfo"] = cred.claims.toMap()
		}

		res, err := taskq.PushAndGetResult(&t, 0)
		if err != nil {
			if strings.EqualFold(err.Error(), "not found") {
				errorResponse(w, http.StatusNotFound, err)
				return
			}

			errorResponse(w, http.StatusSeeOther, err)
			return
		}

		jsonResponse(w, res)
	}
}

func restGetDocumentHandler(storeName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := r.Context().Value(credKey)
		if c == nil {
			log.Warn("[rest get]: no cred in echo context")
			jsonResponseWithStatus(w, http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized))
			return
		}

		cred, ok := c.(credentials)
		if !ok {
			log.Warn("[rest get]: invalid cred in echo context")
			jsonResponseWithStatus(w, http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized))
			return
		}

		id := chi.URLParam(r, "id")
		if len(id) == 0 {
			jsonResponseWithStatus(w, http.StatusBadRequest, http.StatusText(http.StatusBadRequest))
			return
		}

		t := taskq.Task{
			Type:   taskq.DbGet,
			UserID: cred.userID,
			Store:  storeName,
			Arguments: map[string]interface{}{
				"_id": id,
			},
		}
		if ver := r.URL.Query().Get("__v"); len(ver) > 0 {
			v, err := strconv.Atoi(ver)
			if err != nil {
				errorResponse(w, http.StatusBadRequest, errors.New("invalid __v param"))
				return
			}
			t.Arguments["__v"] = v
		}
		if cred.claims != nil {
			t.Arguments["tokenInfo"] = cred.claims.toMap()
		}

		res, err := taskq.PushAndGetResult(&t, 0)
		if err != nil {
			if strings.EqualFold(err.Error(), "not found") {
				errorResponse(w, http.StatusNotFound, err)
				return
			}

			errorResponse(w, http.StatusSeeOther, err)
			return
		}

		jsonResponse(w, res)
	}
}

func restPostDocumentHandler(storeName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := r.Context().Value(credKey)
		if c == nil {
			log.Warn("[rest post]: no cred in echo context")
			jsonResponseWithStatus(w, http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized))
			return
		}

		cred, ok := c.(credentials)
		if !ok {
			log.Warn("[rest post]: invalid cred in echo context")
			jsonResponseWithStatus(w, http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized))
			return
		}

		var item map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
			errorResponse(w, http.StatusBadRequest, err)
			return
		}

		if item["_id"] == nil {
			item["_id"] = uuid.NewV4()
		}

		t := taskq.Task{
			Type:   taskq.DbSet,
			UserID: cred.userID,
			Store:  storeName,
			Arguments: map[string]interface{}{
				"item": item,
			},
		}
		if cred.claims != nil {
			t.Arguments["tokenInfo"] = cred.claims.toMap()
		}

		res, err := taskq.PushAndGetResult(&t, 0)
		if err != nil {
			errorResponse(w, http.StatusSeeOther, err)
			return
		}

		item, ok = res.(map[string]interface{})
		if !ok {
			errorResponse(w, http.StatusInternalServerError, berrors.ErrError)
			return
		}

		jsonResponseWithStatus(w, http.StatusCreated, item["_id"])
	}
}

func restPutDocumentHandler(storeName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := r.Context().Value(credKey)
		if c == nil {
			log.Warn("[rest put]: no cred in echo context")
			jsonResponseWithStatus(w, http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized))
			return
		}

		cred, ok := c.(credentials)
		if !ok {
			log.Warn("[rest put]: invalid cred in echo context")
			jsonResponseWithStatus(w, http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized))
			return
		}

		id := chi.URLParam(r, "id")
		if len(id) == 0 {
			jsonResponseWithStatus(w, http.StatusBadRequest, http.StatusText(http.StatusBadRequest))
			return
		}

		var item map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
			errorResponse(w, http.StatusBadRequest, err)
			return
		}

		item["_id"] = id
		t := taskq.Task{
			Type:   taskq.DbSet,
			UserID: cred.userID,
			Store:  storeName,
			Arguments: map[string]interface{}{
				"item": item,
			},
		}
		if cred.claims != nil {
			t.Arguments["tokenInfo"] = cred.claims.toMap()
		}

		if _, err := taskq.PushAndGetResult(&t, 0); err != nil {
			errorResponse(w, http.StatusSeeOther, err)
			return
		}

		jsonResponse(w, http.StatusText(http.StatusOK))
	}
}

func restDeleteDocumentHandler(storeName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := r.Context().Value(credKey)
		if c == nil {
			log.Warn("[rest delete]: no cred in echo context")
			jsonResponseWithStatus(w, http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized))
			return
		}

		cred, ok := c.(credentials)
		if !ok {
			log.Warn("[rest delete]: invalid cred in echo context")
			jsonResponseWithStatus(w, http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized))
			return
		}

		id := chi.URLParam(r, "id")
		if len(id) == 0 {
			jsonResponseWithStatus(w, http.StatusBadRequest, http.StatusText(http.StatusBadRequest))
			return
		}

		t := taskq.Task{
			Type:   taskq.DbDelete,
			UserID: cred.userID,
			Store:  storeName,
			Arguments: map[string]interface{}{
				"_id": id,
			},
		}
		if cred.claims != nil {
			t.Arguments["tokenInfo"] = cred.claims.toMap()
		}

		if _, err := taskq.PushAndGetResult(&t, 0); err != nil {
			if strings.EqualFold(err.Error(), "not found") {
				jsonResponse(w, http.StatusText(http.StatusOK))
				return
			}

			errorResponse(w, http.StatusSeeOther, err)
			return
		}

		jsonResponse(w, http.StatusText(http.StatusOK))
	}
}
