package internet

import (
	"errors"
	"time"

	"golang.org/x/net/websocket"

	"github.com/getblank/blank-router/berrors"
	"github.com/getblank/blank-router/taskq"
	"github.com/getblank/rgx"
	"github.com/getblank/wango"

	"github.com/getblank/blank-one/sessions"
)

const (
	uriState  = "com.state"
	uriAction = "com.action"
	uriTime   = "com.time"

	uriSubConfig = "com.config"
	uriSubStores = "com.stores"
	uriSubUser   = "com.user"
)

var (
	wamp                  = wango.New()
	rgxRPC                = rgx.New(`^com\.stores\.(?P<store>[a-zA-Z0-9_]*).(?P<command>[a-z\-]*)$`)
	errUnknownMethod      = errors.New("method unknown")
	forbiddenMessageBytes = []byte(`[403,"Forbidden"]`)
)

func wampHandler(ws *websocket.Conn) {
	r := ws.Request()
	var canUpgrade bool
	var cred credentials
	token := extractToken(r)
	if token != "" {
		claims, err := extractClaimsFromJWT(token)
		if err == nil {
			_, err = sessions.CheckSession(claims.SessionID)
			if err == nil {
				canUpgrade = true
				cred = credentials{userID: claims.UserID, sessionID: claims.SessionID, claims: claims}
			}
		}
	}

	if !canUpgrade {
		if _, err := ws.Write(forbiddenMessageBytes); err != nil {
			log.Debugf("[wampHandler] write forbidden error: %v", err)
			return
		}

		if err := ws.WriteClose(403); err != nil {
			log.Debugf("[wampHandler] WriteClose error: %v", err)
		}

		return
	}

	wamp.WampHandler(ws, cred)
}

func wampInit() {
	wamp.StringMode()
	wamp.SetSessionOpenCallback(sessionOpenCallback)
	wamp.SetSessionCloseCallback(sessionCloseCallback)

	err := wamp.RegisterRPCHandler(uriState, stateHandler)
	if err != nil {
		panic(err)
	}
	err = wamp.RegisterRPCHandler(uriAction, actionHandler)
	if err != nil {
		panic(err)
	}
	err = wamp.RegisterRPCHandler(rgxRPC.Regexp, rgxRPCHandler)
	if err != nil {
		panic(err)
	}

	err = wamp.RegisterRPCHandler("com.check-user", checkUserWAMPHandler)
	if err != nil {
		panic(err)
	}

	err = wamp.RegisterSubHandler(uriSubUser, subUserHandler, nil, nil)
	if err != nil {
		panic(err)
	}
	err = wamp.RegisterSubHandler(uriSubConfig, subConfigHandler, nil, nil)
	if err != nil {
		panic(err)
	}
	err = wamp.RegisterRPCHandler(uriTime, timeHandler)
	if err != nil {
		panic(err)
	}
	err = wamp.RegisterSubHandler(uriSubStores, subStoresHandler, unsubStoresHandler, nil)
	if err != nil {
		panic(err)
	}
}

func sessionOpenCallback(c *wango.Conn) {

}

func sessionCloseCallback(c *wango.Conn) {
	extra := c.GetExtra()
	if extra == nil {
		return
	}
	cred, ok := extra.(credentials)
	if !ok {
		log.Warn("Invalid type of extra on session close")
		return
	}
	log.Infof("User id: %s disconnected", cred.userID)
	err := sessions.DeleteConnection(cred.sessionID, c.ID())
	if err != nil {
		log.Errorf("Can't delete connection when session closed, error: %v", err)
	}
}

func timeHandler(c *wango.Conn, uri string, args ...interface{}) (interface{}, error) {
	return time.Now().Format(time.RFC3339Nano), nil
}

func actionHandler(c *wango.Conn, uri string, args ...interface{}) (interface{}, error) {
	if len(args) < 3 {
		return nil, berrors.ErrInvalidArguments
	}
	var userID interface{}
	extra := c.GetExtra()
	if extra != nil {
		cred, ok := extra.(credentials)
		if !ok {
			log.Warn("Invalid type of extra on connection when rpx handler")
			return nil, berrors.ErrError
		}
		_, err := sessions.CheckSession(cred.sessionID)
		if err != nil {
			return nil, berrors.ErrForbidden
		}
		userID = cred.userID
	}
	store, ok := args[0].(string)
	if !ok {
		return nil, berrors.ErrInvalidArguments
	}
	actionID, ok := args[1].(string)
	if !ok {
		return nil, berrors.ErrInvalidArguments
	}
	t := taskq.Task{
		Type:   taskq.DbAction,
		Store:  store,
		UserID: userID,
		Arguments: map[string]interface{}{
			"itemId":   args[2],
			"actionId": actionID,
		},
	}
	if len(args) > 3 {
		t.Arguments["data"] = args[3]
	}
	resChan := taskq.Push(&t)

	res := <-resChan
	if res.Err != "" {
		return nil, errors.New(res.Err)
	}

	return res.Result, nil
}

func checkUserWAMPHandler(c *wango.Conn, uri string, args ...interface{}) (interface{}, error) {
	if len(args) == 0 {
		return nil, berrors.ErrInvalidArguments
	}
	t := taskq.Task{
		Type:   taskq.DbFind,
		UserID: "root",
		Store:  "users",
		Arguments: map[string]interface{}{
			"query": map[string]interface{}{
				"query": map[string]interface{}{
					"email": args[0],
				},
				"props": []string{"_id"},
			},
		},
	}
	_res, err := taskq.PushAndGetResult(&t, 0)
	if err != nil {
		return "USER_NOT_FOUND", nil
	}
	res, ok := _res.(map[string]interface{})
	if !ok {
		return nil, berrors.ErrError
	}
	_items, ok := res["items"]
	if !ok {
		return nil, berrors.ErrError
	}
	items, ok := _items.([]interface{})
	if !ok {
		return nil, berrors.ErrError
	}
	if len(items) > 0 {
		return "USER_EXISTS", nil
	}
	return "USER_NOT_FOUND", nil
}

func stateHandler(c *wango.Conn, uri string, args ...interface{}) (interface{}, error) {
	return "ready", nil
}

func rgxRPCHandler(c *wango.Conn, uri string, args ...interface{}) (interface{}, error) {
	if len(args) == 0 {
		return nil, berrors.ErrInvalidArguments
	}

	var userID interface{} = "guest"

	extra := c.GetExtra()
	if extra != nil {
		cred, ok := extra.(credentials)
		if !ok {
			log.Warn("Invalid type of extra on connection when rpx handler")
			return nil, berrors.ErrError
		}
		_, err := sessions.CheckSession(cred.sessionID)
		if err != nil {
			return nil, berrors.ErrForbidden
		}
		userID = cred.userID
	}

	match, ok := rgxRPC.FindStringSubmatchMap(uri)
	if ok {
		store := match["store"]
		t := taskq.Task{
			UserID: userID,
			Store:  store,
		}
		switch match["command"] {
		case "get":
			t.Type = taskq.DbGet
			t.Arguments = map[string]interface{}{"_id": args[0]}
			return taskq.PushAndGetResult(&t, 0)
		case "save":
			t.Type = taskq.DbSet
			t.Arguments = map[string]interface{}{"item": args[0]}
			return taskq.PushAndGetResult(&t, 0)
		case "insert":
			t.Type = taskq.DbInsert
			t.Arguments = map[string]interface{}{"item": args[0]}
			return taskq.PushAndGetResult(&t, 0)
		case "delete":
			t.Type = taskq.DbDelete
			t.Arguments = map[string]interface{}{"_id": args[0]}
			return taskq.PushAndGetResult(&t, 0)
		case "push":
			if len(args) < 3 {
				return nil, berrors.ErrInvalidArguments
			}
			t.Type = taskq.DbPush
			t.Arguments = map[string]interface{}{
				"_id":  args[0],
				"prop": args[1],
				"data": args[2],
			}
			return taskq.PushAndGetResult(&t, 0)
		case "load-refs":
			if len(args) < 4 {
				return nil, berrors.ErrInvalidArguments
			}
			t.Type = taskq.DbLoadRefs
			t.Arguments = map[string]interface{}{
				"_id":      args[0],
				"prop":     args[1],
				"selected": args[2],
				"query":    args[3],
			}
			return taskq.PushAndGetResult(&t, 0)
		case "find":
			t.Type = taskq.DbFind
			t.Arguments = map[string]interface{}{
				"query": args[0],
			}
			return taskq.PushAndGetResult(&t, 0)
		case "widget-data":
			if len(args) < 3 {
				return nil, berrors.ErrInvalidArguments
			}
			t.Type = taskq.WidgetData
			t.Arguments = map[string]interface{}{
				"widgetId": args[0],
				"data":     args[1],
				"itemId":   args[2],
			}
			return taskq.PushAndGetResult(&t, 0)
		}
	}
	return nil, errUnknownMethod
}
