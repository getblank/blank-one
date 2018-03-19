package internet

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/dgrijalva/jwt-go"

	"github.com/getblank/blank-one/sessions"
)

type blankClaims struct {
	UserID    interface{} `json:"userId"`
	SessionID string      `json:"sessionId"`
	Extra     map[string]interface{}
	jwt.StandardClaims
}

func (b *blankClaims) toMap() map[string]interface{} {
	res := map[string]interface{}{
		"_id": b.UserID,
		"jwtInfo": map[string]interface{}{
			"userId":    b.UserID,
			"sessionId": b.SessionID,
			"issuedAt":  b.IssuedAt,
			"ussiedBy":  b.Issuer,
			"expiredAt": b.ExpiresAt,
		},
	}

	for k, v := range b.Extra {
		res[k] = v
	}

	return res
}

func (b *blankClaims) UnmarshalJSON(p []byte) error {
	if b.Extra == nil {
		b.Extra = map[string]interface{}{}
	}

	jsonparser.ObjectEach(p, func(key []byte, value []byte, dataType jsonparser.ValueType, offset int) error {
		k := string(key)
		switch k {
		// standard claims
		case "aud":
			if dataType != jsonparser.String {
				return fmt.Errorf("invalid value type for key %s", k)
			}
			b.Audience, _ = jsonparser.ParseString(value)
		case "exp":
			if dataType != jsonparser.Number {
				return fmt.Errorf("invalid value type for key %s", k)
			}
			b.ExpiresAt, _ = jsonparser.ParseInt(value)
		case "jti":
			if dataType != jsonparser.String {
				return fmt.Errorf("invalid value type for key %s", k)
			}
			b.Id, _ = jsonparser.ParseString(value)
		case "iat":
			if dataType != jsonparser.Number {
				return fmt.Errorf("invalid value type for key %s", k)
			}
			b.IssuedAt, _ = jsonparser.ParseInt(value)
		case "iss":
			if dataType != jsonparser.String {
				return fmt.Errorf("invalid value type for key %s", k)
			}
			b.Issuer, _ = jsonparser.ParseString(value)
		case "nbf":
			if dataType != jsonparser.Number {
				return fmt.Errorf("invalid value type for key %s", k)
			}
			b.NotBefore, _ = jsonparser.ParseInt(value)
		case "sub":
			if dataType != jsonparser.String {
				return fmt.Errorf("invalid value type for key %s", k)
			}
			b.Subject, _ = jsonparser.ParseString(value)

			// custom claims
		case "sessionId":
			if dataType != jsonparser.String {
				return fmt.Errorf("invalid value type for key %s", k)
			}
			b.SessionID, _ = jsonparser.ParseString(value)
		case "userId":
			val, err := parseInterface(value, dataType)
			if err != nil {
				return err
			}
			b.UserID = val
		default:
			val, err := parseInterface(value, dataType)
			if err != nil {
				return err
			}
			b.Extra[k] = val
		}
		return nil
	})

	return nil
}

func parseInterface(value []byte, dataType jsonparser.ValueType) (val interface{}, err error) {
	switch dataType {
	case jsonparser.String:
		val, _ = jsonparser.ParseString(value)
	case jsonparser.Number:
		val, _ = jsonparser.ParseFloat(value)
	case jsonparser.Object, jsonparser.Array:
		err = json.Unmarshal(value, &val)
	case jsonparser.Boolean:
		val, _ = jsonparser.ParseBoolean(value)
	}

	return val, err
}

func jwtChecker(t *jwt.Token) (interface{}, error) {
	claims, ok := t.Claims.(*blankClaims)
	if !ok {
		return nil, errors.New("invalid claims")
	}

	if !claims.VerifyIssuer("Blank ltd", true) {
		return nil, errors.New("unknown issuer")
	}

	if !claims.VerifyExpiresAt(time.Now().Unix(), true) {
		return nil, errors.New("token expired")
	}

	return sessions.PublicKey(), nil
}

func extractAPIKeyAndUserIDromJWT(token string) (apiKey string, userID interface{}, err error) {
	claims, err := extractClaimsFromJWT(token)

	return claims.SessionID, claims.UserID, err
}

func extractClaimsFromJWT(token string) (claims *blankClaims, err error) {
	claims = new(blankClaims)
	_, err = jwt.ParseWithClaims(token, claims, jwtChecker)

	return claims, err
}

func extractToken(r *http.Request) string {
	var accessToken string
	if authHeader := r.Header.Get("Authorization"); len(authHeader) != 0 {
		if strings.HasPrefix(authHeader, "Bearer ") {
			return strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
		}
	}

	accessToken = r.URL.Query().Get("access_token")
	if len(accessToken) == 0 {
		if cookie, err := r.Cookie("access_token"); err == nil && cookie.Expires.Before(time.Now()) {
			accessToken = cookie.Value
		}
	}

	return accessToken
}
