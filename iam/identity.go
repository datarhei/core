package iam

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sync"

	"github.com/datarhei/core/v16/iam/jwks"
	"github.com/datarhei/core/v16/io/fs"

	jwtgo "github.com/golang-jwt/jwt/v4"
)

// Auth0
// there needs to be a mapping from the Auth.User to Name
// the same Auth0.User can't have multiple identities
// the whole jwks will be part of this package

type User struct {
	Name      string   `json:"name"`
	Superuser bool     `json:"superuser"`
	Auth      UserAuth `json:"auth"`
}

type UserAuth struct {
	API      UserAuthAPI      `json:"api"`
	Services UserAuthServices `json:"services"`
}

type UserAuthAPI struct {
	Userpass UserAuthPassword `json:"userpass"`
	Auth0    UserAuthAPIAuth0 `json:"auth0"`
}

type UserAuthAPIAuth0 struct {
	Enable bool        `json:"enable"`
	User   string      `json:"user"`
	Tenant Auth0Tenant `json:"tenant"`
}

type UserAuthServices struct {
	Basic UserAuthPassword `json:"basic"`
	Token string           `json:"token"`
}

type UserAuthPassword struct {
	Enable   bool   `json:"enable"`
	Password string `json:"password"`
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

func (i *identity) Name() string {
	return i.user.Name
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
	Name() string

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
	GetVerifierByAuth0(name string) (IdentityVerifier, error)
	Rename(oldname, newname string) error
	Update(name string, identity User) error

	Save() error
	Close()
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

func (a *auth0Tenant) Cancel() {
	a.certs.Cancel()
}

type identityManager struct {
	identities map[string]*identity
	tenants    map[string]*auth0Tenant

	auth0UserIdentityMap map[string]string

	fs       fs.Filesystem
	filePath string

	lock sync.RWMutex
}

func NewIdentityManager(fs fs.Filesystem, superuser User) (IdentityManager, error) {
	im := &identityManager{
		identities:           map[string]*identity{},
		tenants:              map[string]*auth0Tenant{},
		auth0UserIdentityMap: map[string]string{},
		fs:                   fs,
		filePath:             "./users.json",
	}

	err := im.load(im.filePath)
	if err != nil {
		return nil, err
	}

	if len(im.identities) == 0 {
		superuser.Superuser = true
		im.Create(superuser)

		im.save(im.filePath)
	}

	return im, nil
}

func (im *identityManager) Close() {
	im.lock.Lock()
	defer im.lock.Unlock()

	im.fs = nil
	im.auth0UserIdentityMap = map[string]string{}
	im.identities = map[string]*identity{}

	for _, t := range im.tenants {
		t.Cancel()
	}

	im.tenants = map[string]*auth0Tenant{}

	return
}

func (im *identityManager) Create(u User) error {
	if err := u.validate(); err != nil {
		return err
	}

	im.lock.Lock()
	defer im.lock.Unlock()

	_, ok := im.identities[u.Name]
	if ok {
		return fmt.Errorf("identity already exists")
	}

	identity := u.marshalIdentity()

	if identity.user.Auth.API.Auth0.Enable {
		if _, ok := im.auth0UserIdentityMap[identity.user.Auth.API.Auth0.User]; ok {
			return fmt.Errorf("the Auth0 user has already an identity")
		}

		auth0Key := identity.user.Auth.API.Auth0.Tenant.key()

		if _, ok := im.tenants[auth0Key]; !ok {
			tenant, err := newAuth0Tenant(identity.user.Auth.API.Auth0.Tenant)
			if err != nil {
				return err
			}

			im.tenants[auth0Key] = tenant
		}

	}

	im.identities[identity.user.Name] = identity

	return nil
}

func (im *identityManager) Update(name string, identity User) error {
	return nil
}

func (im *identityManager) Remove(name string) error {
	im.lock.Lock()
	defer im.lock.Unlock()

	user, ok := im.identities[name]
	if !ok {
		return nil
	}

	delete(im.identities, name)

	user.lock.Lock()
	user.valid = false
	user.lock.Unlock()

	return nil
}

func (im *identityManager) getIdentity(name string) (*identity, error) {
	identity, ok := im.identities[name]
	if !ok {
		return nil, fmt.Errorf("not found")
	}

	return identity, nil
}

func (im *identityManager) Get(name string) (User, error) {
	im.lock.RLock()
	defer im.lock.RUnlock()

	identity, err := im.getIdentity(name)
	if err != nil {
		return User{}, fmt.Errorf("not found")
	}

	return identity.user, nil
}

func (im *identityManager) GetVerifier(name string) (IdentityVerifier, error) {
	im.lock.RLock()
	defer im.lock.RUnlock()

	return im.getIdentity(name)
}

func (im *identityManager) GetVerifierByAuth0(name string) (IdentityVerifier, error) {
	im.lock.RLock()
	defer im.lock.RUnlock()

	name, ok := im.auth0UserIdentityMap[name]
	if !ok {
		return nil, fmt.Errorf("not found")
	}

	return im.getIdentity(name)
}

func (im *identityManager) Rename(oldname, newname string) error {
	im.lock.Lock()
	defer im.lock.Unlock()

	identity, ok := im.identities[oldname]
	if !ok {
		return nil
	}

	if _, ok := im.identities[newname]; ok {
		return fmt.Errorf("the new name already exists")
	}

	delete(im.identities, oldname)

	identity.user.Name = newname
	im.identities[newname] = identity

	return nil
}

func (im *identityManager) load(filePath string) error {
	if im.fs == nil {
		return fmt.Errorf("no filesystem provided")
	}

	if _, err := im.fs.Stat(filePath); os.IsNotExist(err) {
		return nil
	}

	data, err := im.fs.ReadFile(filePath)
	if err != nil {
		return err
	}

	users := []User{}

	err = json.Unmarshal(data, &users)
	if err != nil {
		return err
	}

	for _, u := range users {
		err = im.Create(u)
		if err != nil {
			return err
		}
	}

	return nil
}

func (im *identityManager) Save() error {
	return im.save(im.filePath)
}

func (im *identityManager) save(filePath string) error {
	if im.fs == nil {
		return fmt.Errorf("no filesystem provided")
	}

	if filePath == "" {
		return fmt.Errorf("invalid file path, file path cannot be empty")
	}

	im.lock.RLock()
	defer im.lock.RUnlock()

	users := []User{}

	for _, u := range im.identities {
		users = append(users, u.user)
	}

	jsondata, err := json.MarshalIndent(users, "", "    ")
	if err != nil {
		return err
	}

	_, _, err = im.fs.WriteFileSafe(filePath, jsondata)

	return err
}
