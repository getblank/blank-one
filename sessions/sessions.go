package sessions

import (
	"crypto/rsa"

	log "github.com/Sirupsen/logrus"

	"github.com/getblank/blank-sr/sessionstore"
)

// All returns all registered sessions from sessionstore
func All() []*sessionstore.Session {
	return sessionstore.GetAll()
}

// CheckSession creates a new session in serviceRegistry
func CheckSession(apiKey string) (string, error) {
	s, err := sessionstore.GetByAPIKey(apiKey)
	if err != nil {
		return "", err
	}

	userID, ok := s.GetUserID().(string)
	if !ok {
		log.Warnf("[SessionRegistry.CheckSession] userID %v is not a string", s.GetUserID())
	}

	return userID, nil
}

// DeleteSession delete session with provided apiKey from serviceRegistry
func DeleteSession(apiKey string) error {
	s, err := sessionstore.GetByAPIKey(apiKey)
	if err != nil {
		return err
	}

	s.Delete()

	return nil
}

// NewSession creates a new session in serviceRegistry
func NewSession(user map[string]interface{}, sessionID string) (string, error) {
	return sessionstore.New(user, sessionID).AccessToken, nil
}

// AddSubscription sends subscription info to session store.
func AddSubscription(apiKey, connID, uri string, extra interface{}) error {
	s, err := sessionstore.GetByAPIKey(apiKey)
	if err != nil {
		return err
	}

	s.AddSubscription(connID, uri, extra)

	return nil
}

// DeleteConnection sends delete connection event to sessions store
func DeleteConnection(apiKey, connID string) error {
	s, err := sessionstore.GetByAPIKey(apiKey)
	if err != nil {
		return err
	}

	s.DeleteConnection(connID)

	return nil
}

// DeleteSubscription sends delete subscription event to sessions store
func DeleteSubscription(apiKey, connID, uri string) error {
	s, err := sessionstore.GetByAPIKey(apiKey)
	if err != nil {
		return err
	}

	s.DeleteSubscription(connID, uri)

	return nil
}

// PublicKeyBytes returns RSA public key as []byte
func PublicKeyBytes() []byte {
	return sessionstore.PublicKeyBytes()
}

// PublicKey returns RSA public key
func PublicKey() *rsa.PublicKey {
	return sessionstore.PublicKey()
}

func init() {
	sessionstore.Init()
}
