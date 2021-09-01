package sessionstore

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io/ioutil"
	"os"
	"sync"
	"time"

	"github.com/getblank/blank-sr/bdb"
	"github.com/getblank/blank-sr/berror"
	"github.com/getblank/blank-sr/config"
	"github.com/getblank/uuid"
	"github.com/golang-jwt/jwt"
	log "github.com/sirupsen/logrus"
)

var (
	bucket                = "__sessions"
	sessions              = map[string]*Session{}
	locker                sync.RWMutex
	sessionUpdateHandlers = []func(*Session){}
	sessionDeleteHandlers = []func(*Session){}
	db                    = bdb.DB{}

	publicKey      *rsa.PublicKey
	privateKey     *rsa.PrivateKey
	publicKeyBytes []byte
	rsaLocker      sync.RWMutex
)

// PublicKeyBytes returns PEM RSA Key as []byte
func PublicKeyBytes() []byte {
	rsaLocker.RLock()
	defer rsaLocker.RUnlock()

	return publicKeyBytes
}

const keysDir = "keys"

// Session represents user session in Blank
type Session struct {
	APIKey      string      `json:"apiKey"`
	AccessToken string      `json:"access_token,omitempty"`
	UserID      interface{} `json:"userId"`
	Connections []*Conn     `json:"connections"`
	CreatedAt   time.Time   `json:"createdAt"`
	LastRequest time.Time   `json:"lastRequest"`
	TTL         time.Time   `json:"ttl"`
	V           int         `json:"__v"`
	sync.RWMutex
}

// Conn represents WAMP connection in session
type Conn struct {
	ConnID        string                 `json:"connId"`
	Subscriptions map[string]interface{} `json:"subscriptions"`
}

// Init is the entrypoint of sessionstore
func Init() {
	initRSAKeys()

	loadSessions()
	go ttlWatcher()
}

// New created new user session.
func New(user map[string]interface{}, sessionID string) *Session {
	userID := user["_id"]
	if len(sessionID) == 0 {
		sessionID = uuid.NewV4()
	}

	now := time.Now()
	jwtTTL, err := config.JWTTTL()
	if err != nil {
		log.WithError(err).Error("Can't get JWT TTL. Will setup 24 hours")
		jwtTTL = time.Hour * 24
	}

	ttl := now.Add(jwtTTL)
	claims := jwt.MapClaims{
		"iss":       "Blank ltd",
		"iat":       now.Unix(),
		"exp":       ttl.Unix(),
		"userId":    userID,
		"sessionId": sessionID,
	}

	for _, k := range config.JWTExtraProps() {
		if user[k] != nil {
			claims[k] = user[k]
		}
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tokenString, err := token.SignedString(privateKey)
	if err != nil {
		log.Fatal("Can't sign JWT")
	}

	s := &Session{
		APIKey:      sessionID,
		AccessToken: tokenString,
		UserID:      userID,
		Connections: []*Conn{},
		TTL:         ttl,
		CreatedAt:   time.Now(),
	}

	locker.Lock()
	defer locker.Unlock()

	sessions[s.APIKey] = s
	sessionUpdated(s)

	return s
}

// DeleteAllConnections deletes all connections from all sessions
func DeleteAllConnections() {
	locker.Lock()
	defer locker.Unlock()

	for _, s := range sessions {
		s.Connections = []*Conn{}
		s.Save()
	}
}

// GetAll returns all stored sessions
func GetAll() []*Session {
	result := make([]*Session, len(sessions))
	var i int
	locker.RLock()
	defer locker.RUnlock()

	for _, s := range sessions {
		result[i] = s
		i++
	}

	return result
}

// GetByAPIKey returns point to Session or error if it is not exists.
func GetByAPIKey(APIKey string) (s *Session, err error) {
	return getByAPIKey(APIKey)
}

// GetByUserID returns point to Session or error if it is not exists.
func GetByUserID(id interface{}) (s *Session, err error) {
	return getByUserID(id)
}

// Delete removes session by the APIKey provided from store
func Delete(APIKey string) {
	err := db.Delete(bucket, APIKey)
	if err != nil {
		log.Error("Can't delete session", APIKey, err.Error())
	}

	locker.Lock()
	defer locker.Unlock()

	s := sessions[APIKey]
	delete(sessions, APIKey)
	if s != nil {
		sessionDeleted(s)
	}
}

// DeleteAllForUser removes all sessions for user from store
func DeleteAllForUser(userID string) {
	locker.RLock()
	defer locker.RUnlock()

	for _, s := range sessions {
		if s.UserID == userID {
			go s.Delete()
		}
	}
}

// AddSubscription adds subscription URI with provided params to user session
func (s *Session) AddSubscription(connID, uri string, extra interface{}) {
	s.Lock()
	defer s.Unlock()

	var c *Conn
	for _, _c := range s.Connections {
		if _c.ConnID == connID {
			c = _c
			break
		}
	}

	if c == nil {
		c = new(Conn)
		c.ConnID = connID
		c.Subscriptions = map[string]interface{}{}
		s.Connections = append(s.Connections, c)
	}

	c.Subscriptions[uri] = extra
	sessionUpdated(s)
}

// DeleteConnection deletes WAMP connection from user session
func (s *Session) DeleteConnection(connID string) {
	s.Lock()
	defer s.Unlock()

	for i, _c := range s.Connections {
		if _c.ConnID == connID {
			s.Connections = append(s.Connections[:i], s.Connections[i+1:]...)
			break
		}
	}

	sessionUpdated(s)
}

// DeleteSubscription deletes subscription from connection of user session
func (s *Session) DeleteSubscription(connID, uri string) {
	s.Lock()
	defer s.Unlock()

	var c *Conn
	for _, _c := range s.Connections {
		if _c.ConnID == connID {
			c = _c
			break
		}
	}

	if c == nil {
		return
	}

	delete(c.Subscriptions, uri)
	sessionUpdated(s)
}

// Delete removes Session from store
func (s *Session) Delete() {
	err := db.Delete(bucket, s.APIKey)
	if err != nil {
		log.Error("Can't delete session", s, err.Error())
	}

	locker.Lock()
	defer locker.Unlock()

	delete(sessions, s.APIKey)
	sessionDeleted(s)
}

// Save saves session in store
func (s *Session) Save() {
	s = copySession(s)
	err := db.Save(bucket, s.APIKey, s)
	if err != nil {
		log.Error("Can't save session", s, err.Error())
	}
}

// GetUserID returns userID stored in session
func (s *Session) GetUserID() interface{} {
	return s.UserID
}

// GetAPIKey returns apiKey of session
func (s *Session) GetAPIKey() string {
	return s.APIKey
}

// OnSessionUpdate registers callback that will called when session updated
func OnSessionUpdate(handler func(*Session)) {
	sessionUpdateHandlers = append(sessionUpdateHandlers, handler)
	return
}

// OnSessionDelete registers callback that will called when session deleted
func OnSessionDelete(handler func(*Session)) {
	sessionDeleteHandlers = append(sessionDeleteHandlers, handler)
	return
}

// PublicKey returns *rsa.PublicKey
func PublicKey() *rsa.PublicKey {
	rsaLocker.RLock()
	defer rsaLocker.RUnlock()

	return publicKey
}

func getByAPIKey(APIKey string) (s *Session, err error) {
	locker.Lock()
	defer locker.Unlock()
	s, ok := sessions[APIKey]
	if !ok {
		return s, berror.DbNotFound
	}

	return s, err
}

func getByUserID(id interface{}) (s *Session, err error) {
	locker.RLock()
	defer locker.RUnlock()
	for _, v := range sessions {
		if v.UserID == id {
			v.Lock()
			defer v.Unlock()
			s := copySession(v)
			s.AccessToken = ""
			return copySession(v), nil
		}
	}

	return nil, berror.DbNotFound
}

func clearRottenSessions() {
	locker.Lock()
	defer locker.Unlock()
	now := time.Now()
	for _, s := range sessions {
		if s.TTL.Before(now) {
			err := db.Delete(bucket, s.APIKey)
			if err != nil {
				log.Error("Can't delete session", s, err.Error())
			}
			delete(sessions, s.APIKey)
		}
	}
}

func loadSessions() {
	_sessions, err := db.GetAll(bucket)
	if err != nil && err != berror.DbNotFound {
		log.Error("Can't read all sessions", err.Error())

		return
	}

	now := time.Now()
	locker.Lock()
	defer locker.Unlock()
	for _, _s := range _sessions {
		var s Session
		err := json.Unmarshal(_s, &s)
		if err != nil {
			log.Error("Can't unmarshal session", _s, err.Error())
			continue
		}

		if s.TTL.Before(now) {
			err := db.Delete(bucket, s.APIKey)
			if err != nil {
				log.Errorf("Can't delete session %s when Init(), error: %v", s.APIKey, err.Error())
			}
			continue
		}

		s.Connections = []*Conn{}
		s.Save()
		sessions[s.APIKey] = &s
	}
}

func ttlWatcher() {
	c := time.Tick(time.Minute)
	for {
		<-c
		clearRottenSessions()
	}
}

func sessionUpdated(s *Session, userUpdated ...bool) {
	var b bool
	if userUpdated != nil {
		b = userUpdated[0]
	}

	s.Save()
	_s := copySession(s)
	if !b {
		// what is this????
	}

	s.V++

	for _, handler := range sessionUpdateHandlers {
		go handler(_s)
	}
}

func sessionDeleted(s *Session) {
	s.V++
	for _, handler := range sessionDeleteHandlers {
		go handler(s)
	}
}

func copySession(s *Session) *Session {
	_s := &Session{
		APIKey:      s.APIKey,
		AccessToken: s.AccessToken,
		UserID:      s.UserID,
		Connections: make([]*Conn, len(s.Connections)),
		LastRequest: s.LastRequest,
		TTL:         s.TTL,
		V:           s.V,
	}

	for i := range _s.Connections {
		c := &Conn{ConnID: s.Connections[i].ConnID}
		c.Subscriptions = map[string]interface{}{}
		for k, v := range s.Connections[i].Subscriptions {
			c.Subscriptions[k] = v
		}

		_s.Connections[i] = c
	}

	return _s
}

func initRSAKeys() {
	rsaLocker.Lock()
	defer rsaLocker.Unlock()

	if public, private, err := loadRSAKeys(); err == nil {
		publicKeyBytes = public
		var err error
		publicKey, err = jwt.ParseRSAPublicKeyFromPEM(public)
		if err != nil {
			log.Fatal("Invalid public RSA key", err)
			panic(err)
		}

		privateKey, err = jwt.ParseRSAPrivateKeyFromPEM(private)
		if err != nil {
			log.Fatal("Invalid private RSA key", err)
			panic(err)
		}

		return
	}

	stat, err := os.Stat(keysDir)
	if err != nil {
		if os.IsNotExist(err) {
			os.Mkdir(keysDir, 0744)
		} else {
			log.Fatal("Can't access keys dir", err)
			panic(err)
		}
	} else {
		if !stat.IsDir() {
			log.Fatal("keys dir is not a dir")
			panic("keys dir is not a dir")
		}
	}

	generateRSAKeys()
}

func loadRSAKeys() (public, private []byte, err error) {
	public, err = ioutil.ReadFile(keysDir + "/jwt.pub")
	if err != nil {
		return nil, nil, err
	}

	private, err = ioutil.ReadFile(keysDir + "/jwt.key")
	if err != nil {
		return nil, nil, err
	}

	return
}

func generateRSAKeys() {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	pub, err := x509.MarshalPKIXPublicKey(k.Public())
	pemPrivate := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(k),
		},
	)

	pemPublic := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: pub,
	})

	err = ioutil.WriteFile(keysDir+"/jwt.pub", pemPublic, 0644)
	if err != nil {
		log.Fatal("Can't save public RSA key", err)

	}

	err = ioutil.WriteFile(keysDir+"/jwt.key", pemPrivate, 0644)
	if err != nil {
		log.Fatal("Can't save private RSA key", err)
	}

	publicKeyBytes = pemPublic

	publicKey, err = jwt.ParseRSAPublicKeyFromPEM(publicKeyBytes)
	if err != nil {
		log.Fatal("Invalid public RSA key", err)
		panic(err)
	}
}
