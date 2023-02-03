package iam

import (
	"fmt"
	"regexp"
	"sync"

	"github.com/datarhei/core/v16/http/jwt/jwks"

	jwtgo "github.com/golang-jwt/jwt/v4"
)

// Auth0
// there needs to be a mapping from the Auth.User to Name
// the same Auth0.User can't have multiple identities
// the whole jwks will be part of this package

type User struct {
	Name      string `json:"name"`
	Superuser bool   `json:"superuser"`
	Auth      struct {
		API struct {
			Userpass struct {
				Enable   bool   `json:"enable"`
				Password string `json:"password"`
			} `json:"userpass"`
			Auth0 struct {
				Enable bool        `json:"enable"`
				User   string      `json:"user"`
				Tenant Auth0Tenant `json:"tenant"`
			} `json:"auth0"`
		} `json:"api"`
		Services struct {
			Basic struct {
				Enable   bool   `json:"enable"`
				Password string `json:"password"`
			} `json:"basic"`
			Token string `json:"token"`
		} `json:"services"`
	} `json:"auth"`
}

func (u *User) validate() error {
	if len(u.Name) == 0 {
		return fmt.Errorf("the name is required")
	}

	re := regexp.MustCompile(`[^A-Za-z0-9_-]`)
	if re.MatchString(u.Name) {
		return fmt.Errorf("the name can only the contain [A-Za-z0-9_-]")
	}

	if u.Auth.API.Userpass.Enable && len(u.Auth.API.Userpass.Password) == 0 {
		return fmt.Errorf("a password for API login is required")
	}

	if u.Auth.API.Auth0.Enable && len(u.Auth.API.Auth0.User) == 0 {
		return fmt.Errorf("a user for Auth0 login is required")
	}

	if u.Auth.Services.Basic.Enable && len(u.Auth.Services.Basic.Password) == 0 {
		return fmt.Errorf("a password for service basic auth is required")
	}

	return nil
}

func (u *User) marshalIdentity() *identity {
	i := &identity{
		user: *u,
	}

	return i
}

type identity struct {
	user User

	tenant *auth0Tenant

	valid bool

	lock sync.RWMutex
}

func (i *identity) VerifyAPIPassword(password string) bool {
	i.lock.RLock()
	defer i.lock.RUnlock()

	if !i.isValid() {
		return false
	}

	if !i.user.Auth.API.Userpass.Enable {
		return false
	}

	return i.user.Auth.API.Userpass.Password == password
}

func (i *identity) VerifyAPIAuth0(jwt string) bool {
	i.lock.RLock()
	defer i.lock.RUnlock()

	if !i.isValid() {
		return false
	}

	if !i.user.Auth.API.Auth0.Enable {
		return false
	}

	p := &jwtgo.Parser{}
	token, _, err := p.ParseUnverified(jwt, jwtgo.MapClaims{})
	if err != nil {
		return false
	}

	var subject string
	if claims, ok := token.Claims.(jwtgo.MapClaims); ok {
		if sub, ok := claims["sub"]; ok {
			subject = sub.(string)
		}
	}

	if subject != i.user.Auth.API.Auth0.User {
		return false
	}

	var issuer string
	if claims, ok := token.Claims.(jwtgo.MapClaims); ok {
		if iss, ok := claims["iss"]; ok {
			issuer = iss.(string)
		}
	}

	if issuer != i.tenant.issuer {
		return false
	}

	token, err = jwtgo.Parse(jwt, i.auth0KeyFunc)
	if err != nil {
		return false
	}

	if !token.Valid {
		return false
	}

	return true
}

func (i *identity) auth0KeyFunc(token *jwtgo.Token) (interface{}, error) {
	// Verify 'aud' claim
	checkAud := token.Claims.(jwtgo.MapClaims).VerifyAudience(i.tenant.audience, false)
	if !checkAud {
		return nil, fmt.Errorf("invalid audience")
	}

	// Verify 'iss' claim
	checkIss := token.Claims.(jwtgo.MapClaims).VerifyIssuer(i.tenant.issuer, false)
	if !checkIss {
		return nil, fmt.Errorf("invalid issuer")
	}

	// Verify 'sub' claim
	if _, ok := token.Claims.(jwtgo.MapClaims)["sub"]; !ok {
		return nil, fmt.Errorf("sub claim is required")
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

func (i *identity) VerifyServiceBasicAuth(password string) bool {
	i.lock.RLock()
	defer i.lock.RUnlock()

	if !i.isValid() {
		return false
	}

	if !i.user.Auth.Services.Basic.Enable {
		return false
	}

	return i.user.Auth.Services.Basic.Password == password
}

func (i *identity) VerifyServiceToken(token string) bool {
	i.lock.RLock()
	defer i.lock.RUnlock()

	if !i.isValid() {
		return false
	}

	return i.user.Auth.Services.Token == token
}

func (i *identity) isValid() bool {
	return i.valid
}

func (i *identity) IsSuperuser() bool {
	i.lock.RLock()
	defer i.lock.RUnlock()

	return i.user.Superuser
}

type IdentityVerifier interface {
	VerifyAPIPassword(password string) bool
	VerifyAPIAuth0(jwt string) bool

	VerifyServiceBasicAuth(password string) bool
	VerifyServiceToken(token string) bool

	IsSuperuser() bool
}

type IdentityManager interface {
	Create(identity User) error
	Remove(name string) error
	Get(name string) (User, error)
	GetVerifier(name string) (IdentityVerifier, error)
	Rename(oldname, newname string) error
	Update(name string, identity User) error

	Load(path string) error
	Save(path string) error
}

type Auth0Tenant struct {
	Domain   string
	Audience string
	ClientID string
}

func (t *Auth0Tenant) key() string {
	return t.Domain + t.Audience
}

type auth0Tenant struct {
	domain    string
	issuer    string
	audience  string
	clientIDs []string
	certs     jwks.JWKS
}

func newAuth0Tenant(tenant Auth0Tenant) (*auth0Tenant, error) {
	t := &auth0Tenant{
		domain:    tenant.Domain,
		issuer:    "https://" + tenant.Domain + "/",
		audience:  tenant.Audience,
		clientIDs: []string{tenant.ClientID},
		certs:     nil,
	}

	url := t.issuer + "/.well-known/jwks.json"
	certs, err := jwks.NewFromURL(url, jwks.Config{})
	if err != nil {
		return nil, err
	}

	t.certs = certs

	return t, nil
}

type identityManager struct {
	identities map[string]*identity
	tenants    map[string]*auth0Tenant

	auth0UserIdentityMap map[string]string

	lock sync.RWMutex
}

func NewIdentityManager() (IdentityManager, error) {
	return &identityManager{
		identities:           map[string]*identity{},
		tenants:              map[string]*auth0Tenant{},
		auth0UserIdentityMap: map[string]string{},
	}, nil
}

func (i *identityManager) Create(u User) error {
	if err := u.validate(); err != nil {
		return err
	}

	i.lock.Lock()
	defer i.lock.Unlock()

	_, ok := i.identities[u.Name]
	if ok {
		return fmt.Errorf("identity already exists")
	}

	identity := u.marshalIdentity()

	if identity.user.Auth.API.Auth0.Enable {
		if _, ok := i.auth0UserIdentityMap[identity.user.Auth.API.Auth0.User]; ok {
			return fmt.Errorf("the Auth0 user has already an identity")
		}

		auth0Key := identity.user.Auth.API.Auth0.Tenant.key()

		if _, ok := i.tenants[auth0Key]; !ok {
			tenant, err := newAuth0Tenant(identity.user.Auth.API.Auth0.Tenant)
			if err != nil {
				return err
			}

			i.tenants[auth0Key] = tenant
		}

	}

	i.identities[identity.user.Name] = identity

	return nil
}

func (i *identityManager) Update(name string, identity User) error {
	return nil
}

func (i *identityManager) Remove(name string) error {
	i.lock.Lock()
	defer i.lock.Unlock()

	user, ok := i.identities[name]
	if !ok {
		return nil
	}

	delete(i.identities, name)

	user.lock.Lock()
	user.valid = false
	user.lock.Unlock()

	return nil
}

func (i *identityManager) getIdentity(name string) (*identity, error) {
	i.lock.RLock()
	defer i.lock.RUnlock()

	identity, ok := i.identities[name]
	if !ok {
		return nil, fmt.Errorf("not found")
	}

	return identity, nil
}

func (i *identityManager) Get(name string) (User, error) {
	i.lock.RLock()
	defer i.lock.RUnlock()

	identity, ok := i.identities[name]
	if !ok {
		return User{}, fmt.Errorf("not found")
	}

	return identity.user, nil
}

func (i *identityManager) GetVerifier(name string) (IdentityVerifier, error) {
	i.lock.RLock()
	defer i.lock.RUnlock()

	identity, ok := i.identities[name]
	if !ok {
		return nil, fmt.Errorf("not found")
	}

	return identity, nil
}

func (i *identityManager) Rename(oldname, newname string) error {
	i.lock.Lock()
	defer i.lock.Unlock()

	identity, ok := i.identities[oldname]
	if !ok {
		return nil
	}

	if _, ok := i.identities[newname]; ok {
		return fmt.Errorf("the new name already exists")
	}

	delete(i.identities, oldname)

	identity.user.Name = newname
	i.identities[newname] = identity

	return nil
}

func (i *identityManager) Load(path string) error {
	return fmt.Errorf("not implemented")
}

func (i *identityManager) Save(path string) error {
	return fmt.Errorf("not implemented")
}
