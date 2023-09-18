package identity

import (
	"fmt"
	"net/url"
	"sync"
	"time"

	enctoken "github.com/datarhei/core/v16/encoding/token"

	jwtgo "github.com/golang-jwt/jwt/v5"
)

// Auth0
// there needs to be a mapping from the Auth.User to Name
// the same Auth0.User can't have multiple identities
// the whole jwks will be part of this package

type Verifier interface {
	Name() string  // Name returns the name of the identity.
	Alias() string // Alias returns the alias of the identity, or an empty string if no alias has been set.

	VerifyJWT(jwt string) (bool, error)

	VerifyAPIPassword(password string) (bool, error)
	VerifyAPIAuth0(jwt string) (bool, error)

	VerifyServiceBasicAuth(password string) (bool, error)
	VerifyServiceToken(token string) (bool, error)
	VerifyServiceSession(jwt string) (bool, interface{}, error)

	GetServiceBasicAuth() *url.Userinfo
	GetServiceToken() string
	GetServiceSession(interface{}, time.Duration) string

	IsSuperuser() bool
}

type identity struct {
	user User

	tenant *auth0Tenant

	jwtRealm   string
	jwtKeyFunc func(*jwtgo.Token) (interface{}, error)

	valid bool

	lock sync.RWMutex
}

func (i *identity) Name() string {
	return i.user.Name
}

func (i *identity) Alias() string {
	return i.user.Alias
}

func (i *identity) VerifyAPIPassword(password string) (bool, error) {
	i.lock.RLock()
	defer i.lock.RUnlock()

	if !i.isValid() {
		return false, fmt.Errorf("invalid identity")
	}

	if len(i.user.Auth.API.Password) == 0 {
		return false, fmt.Errorf("authentication method disabled")
	}

	return i.user.Auth.API.Password == password, nil
}

func (i *identity) VerifyAPIAuth0(jwt string) (bool, error) {
	i.lock.RLock()
	defer i.lock.RUnlock()

	if !i.isValid() {
		return false, fmt.Errorf("invalid identity")
	}

	if len(i.user.Auth.API.Auth0.User) == 0 {
		return false, fmt.Errorf("authentication method disabled")
	}

	p := &jwtgo.Parser{}
	token, _, err := p.ParseUnverified(jwt, jwtgo.MapClaims{})
	if err != nil {
		return false, err
	}

	subject, err := token.Claims.GetSubject()
	if err != nil {
		return false, fmt.Errorf("invalid subject: %w", err)
	}

	if subject != i.user.Auth.API.Auth0.User {
		return false, fmt.Errorf("wrong subject")
	}

	issuer, err := token.Claims.GetIssuer()
	if err != nil {
		return false, fmt.Errorf("invalid issuer: %w", err)
	}

	if issuer != i.tenant.issuer {
		return false, fmt.Errorf("wrong issuer")
	}

	token, err = jwtgo.Parse(jwt, i.auth0KeyFunc)
	if err != nil {
		return false, err
	}

	if !token.Valid {
		return false, fmt.Errorf("invalid token")
	}

	return true, nil
}

func (i *identity) auth0KeyFunc(token *jwtgo.Token) (interface{}, error) {
	// Verify 'aud' claim
	if aud, err := token.Claims.GetAudience(); err != nil {
		return nil, fmt.Errorf("invalid audience: %w", err)
	} else if len(aud) == 0 {
		return nil, fmt.Errorf("audience is not present")
	}

	// Verify 'iss' claim
	if iss, err := token.Claims.GetIssuer(); err != nil {
		return nil, fmt.Errorf("invalid issuer: %w", err)
	} else if len(iss) == 0 {
		return nil, fmt.Errorf("issuer is not present")
	}

	// Verify 'sub' claim
	if sub, err := token.Claims.GetSubject(); err != nil {
		return nil, fmt.Errorf("invalid subject: %w", err)
	} else if len(sub) == 0 {
		return nil, fmt.Errorf("subject is not present")
	}

	// find the key
	if _, ok := token.Header["kid"]; !ok {
		return nil, fmt.Errorf("kid not found")
	}

	kid := token.Header["kid"].(string)

	key, err := i.tenant.certs.Key(kid)
	if err != nil {
		return nil, fmt.Errorf("no cert for kid found: %w", err)
	}

	// find algorithm
	if _, ok := token.Header["alg"]; !ok {
		return nil, fmt.Errorf("kid not found")
	}

	alg := token.Header["alg"].(string)

	if key.Alg() != alg {
		return nil, fmt.Errorf("signing method doesn't match")
	}

	// get the public key
	publicKey, err := key.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("invalid public key: %w", err)
	}

	return publicKey, nil
}

func (i *identity) VerifyJWT(jwt string) (bool, error) {
	i.lock.RLock()
	defer i.lock.RUnlock()

	if !i.isValid() {
		return false, fmt.Errorf("invalid identity")
	}

	p := &jwtgo.Parser{}
	token, _, err := p.ParseUnverified(jwt, jwtgo.MapClaims{})
	if err != nil {
		return false, err
	}

	subject, err := token.Claims.GetSubject()
	if err != nil {
		return false, fmt.Errorf("invalid subject: %w", err)
	}

	if subject != i.user.Name {
		return false, fmt.Errorf("wrong subject")
	}

	issuer, err := token.Claims.GetIssuer()
	if err != nil {
		return false, fmt.Errorf("invalid issuer: %w", err)
	}

	if issuer != i.jwtRealm {
		return false, fmt.Errorf("wrong issuer")
	}

	token, err = jwtgo.Parse(jwt, i.jwtKeyFunc, jwtgo.WithValidMethods([]string{"HS256"}))
	if err != nil {
		return false, err
	}

	if !token.Valid {
		return false, fmt.Errorf("invalid token")
	}

	return true, nil
}

func (i *identity) VerifyServiceBasicAuth(password string) (bool, error) {
	i.lock.RLock()
	defer i.lock.RUnlock()

	if !i.isValid() {
		return false, fmt.Errorf("invalid identity")
	}

	for _, pw := range i.user.Auth.Services.Basic {
		if len(pw) == 0 {
			continue
		}

		if pw == password {
			return true, nil
		}
	}

	return false, nil
}

func (i *identity) GetServiceBasicAuth() *url.Userinfo {
	i.lock.RLock()
	defer i.lock.RUnlock()

	if !i.isValid() {
		return nil
	}

	name := i.Alias()
	if len(name) == 0 {
		name = i.Name()
	}

	for _, password := range i.user.Auth.Services.Basic {
		if len(password) == 0 {
			continue
		}

		return url.UserPassword(name, password)
	}

	return url.User(name)
}

func (i *identity) VerifyServiceToken(token string) (bool, error) {
	i.lock.RLock()
	defer i.lock.RUnlock()

	if !i.isValid() {
		return false, fmt.Errorf("invalid identity")
	}

	for _, t := range i.user.Auth.Services.Token {
		if t == token {
			return true, nil
		}
	}

	return false, nil
}

func (i *identity) GetServiceToken() string {
	i.lock.RLock()
	defer i.lock.RUnlock()

	if !i.isValid() {
		return ""
	}

	for _, token := range i.user.Auth.Services.Token {
		if len(token) == 0 {
			continue
		}

		name := i.Alias()
		if len(name) == 0 {
			name = i.Name()
		}

		return enctoken.Marshal(name, token)
	}

	return ""
}

func (i *identity) VerifyServiceSession(jwt string) (bool, interface{}, error) {
	i.lock.RLock()
	defer i.lock.RUnlock()

	if !i.isValid() {
		return false, nil, fmt.Errorf("invalid identity")
	}

	if len(i.user.Auth.Services.Session) == 0 {
		return false, nil, nil
	}

	p := &jwtgo.Parser{}
	token, _, err := p.ParseUnverified(jwt, jwtgo.MapClaims{})
	if err != nil {
		return false, nil, err
	}

	claims, ok := token.Claims.(jwtgo.MapClaims)
	if !ok {
		return false, nil, fmt.Errorf("invalid claims")
	}

	subject, err := claims.GetSubject()
	if err != nil {
		return false, nil, fmt.Errorf("invalid subject: %w", err)
	}

	if subject != i.user.Name && subject != i.user.Alias {
		return false, nil, fmt.Errorf("wrong subject")
	}

	issuer, err := claims.GetIssuer()
	if err != nil {
		return false, nil, fmt.Errorf("invalid issuer: %w", err)
	}

	if issuer != i.jwtRealm {
		return false, nil, fmt.Errorf("wrong issuer")
	}

	for _, secret := range i.user.Auth.Services.Session {
		fn := func(*jwtgo.Token) (interface{}, error) { return []byte(secret), nil }
		token, err = jwtgo.Parse(jwt, fn, jwtgo.WithValidMethods([]string{"HS256"}))
		if err == nil {
			break
		}
	}

	if err != nil {
		return false, nil, fmt.Errorf("parse: %w", err)
	}

	if !token.Valid {
		return false, nil, fmt.Errorf("invalid token")
	}

	return true, claims["data"], nil
}

func (i *identity) GetServiceSession(data interface{}, ttl time.Duration) string {
	i.lock.RLock()
	defer i.lock.RUnlock()

	if !i.isValid() {
		return ""
	}

	if len(i.user.Auth.Services.Session) == 0 {
		return ""
	}

	now := time.Now()
	accessExpires := now.Add(ttl)

	// Create access token
	accessToken := jwtgo.NewWithClaims(jwtgo.SigningMethodHS256, jwtgo.MapClaims{
		"iss":  i.jwtRealm,
		"sub":  i.user.Name,
		"iat":  now.Unix(),
		"exp":  accessExpires.Unix(),
		"data": data,
	})

	// Generate encoded access token
	at, err := accessToken.SignedString([]byte(i.user.Auth.Services.Session[0]))
	if err != nil {
		return ""
	}

	return at
}

func (i *identity) isValid() bool {
	return i.valid
}

func (i *identity) IsSuperuser() bool {
	i.lock.RLock()
	defer i.lock.RUnlock()

	return i.user.Superuser
}
