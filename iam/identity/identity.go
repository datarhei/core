package identity

import (
	"fmt"
	"strings"
	"sync"
	"time"

	enctoken "github.com/datarhei/core/v16/encoding/token"
	"github.com/datarhei/core/v16/iam/jwks"
	"github.com/datarhei/core/v16/log"
	"github.com/google/uuid"

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
	Password string           `json:"password"`
	Auth0    UserAuthAPIAuth0 `json:"auth0"`
}

type UserAuthAPIAuth0 struct {
	User   string      `json:"user"`
	Tenant Auth0Tenant `json:"tenant"`
}

type UserAuthServices struct {
	Basic   []string `json:"basic"`   // Passwords for BasicAuth
	Token   []string `json:"token"`   // Tokens/Streamkey for RTMP and SRT
	Session []string `json:"session"` // Secrets for session JWT
}

func (u *User) Validate() error {
	if len(u.Name) == 0 {
		return fmt.Errorf("a name is required")
	}

	if strings.HasPrefix(u.Name, "$") {
		return fmt.Errorf("name is not allowed to start with $")
	}

	if len(u.Auth.API.Auth0.User) != 0 {
		t, err := newAuth0Tenant(u.Auth.API.Auth0.Tenant)
		if err != nil {
			return fmt.Errorf("auth0: %w", err)
		}

		t.Cancel()
	}

	return nil
}

func (u *User) marshalIdentity() *identity {
	i := &identity{
		user: *u,
	}

	return i
}

func (u *User) clone() User {
	user := *u

	user.Auth.Services.Basic = make([]string, len(u.Auth.Services.Basic))
	copy(user.Auth.Services.Basic, u.Auth.Services.Basic)

	user.Auth.Services.Token = make([]string, len(u.Auth.Services.Token))
	copy(user.Auth.Services.Token, u.Auth.Services.Token)

	user.Auth.Services.Session = make([]string, len(u.Auth.Services.Session))
	copy(user.Auth.Services.Session, u.Auth.Services.Session)

	return user
}

type Verifier interface {
	Name() string

	VerifyJWT(jwt string) (bool, error)

	VerifyAPIPassword(password string) (bool, error)
	VerifyAPIAuth0(jwt string) (bool, error)

	VerifyServiceBasicAuth(password string) (bool, error)
	VerifyServiceToken(token string) (bool, error)
	VerifyServiceSession(jwt string) (bool, interface{}, error)

	GetServiceBasicAuth() string
	GetServiceToken() string
	GetServiceSession(interface{}) string

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

	var subject string
	if claims, ok := token.Claims.(jwtgo.MapClaims); ok {
		if sub, ok := claims["sub"]; ok {
			subject = sub.(string)
		}
	}

	if subject != i.user.Auth.API.Auth0.User {
		return false, fmt.Errorf("wrong subject")
	}

	var issuer string
	if claims, ok := token.Claims.(jwtgo.MapClaims); ok {
		if iss, ok := claims["iss"]; ok {
			issuer = iss.(string)
		}
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

	var subject string
	if claims, ok := token.Claims.(jwtgo.MapClaims); ok {
		if sub, ok := claims["sub"]; ok {
			subject = sub.(string)
		}
	}

	if subject != i.user.Name {
		return false, fmt.Errorf("wrong subject")
	}

	var issuer string
	if claims, ok := token.Claims.(jwtgo.MapClaims); ok {
		if sub, ok := claims["iss"]; ok {
			issuer = sub.(string)
		}
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

func (i *identity) GetServiceBasicAuth() string {
	i.lock.RLock()
	defer i.lock.RUnlock()

	if !i.isValid() {
		return ""
	}

	for _, password := range i.user.Auth.Services.Basic {
		if len(password) == 0 {
			continue
		}

		return password
	}

	return ""
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

		return enctoken.Marshal(i.Name(), token)
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

	var subject string
	if sub, ok := claims["sub"]; ok {
		subject = sub.(string)
	}

	if subject != i.user.Name {
		return false, nil, fmt.Errorf("wrong subject")
	}

	var issuer string
	if sub, ok := claims["iss"]; ok {
		issuer = sub.(string)
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

func (i *identity) GetServiceSession(data interface{}) string {
	i.lock.RLock()
	defer i.lock.RUnlock()

	if !i.isValid() {
		return ""
	}

	if len(i.user.Auth.Services.Session) == 0 {
		return ""
	}

	now := time.Now()
	accessExpires := now.Add(time.Minute * 10)

	// Create access token
	accessToken := jwtgo.NewWithClaims(jwtgo.SigningMethodHS256, jwtgo.MapClaims{
		"iss":  i.jwtRealm,
		"sub":  i.user.Name,
		"iat":  now.Unix(),
		"exp":  accessExpires.Unix(),
		"exi":  uint64(accessExpires.Sub(now).Seconds()),
		"jti":  uuid.New().String(),
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

type Manager interface {
	Create(identity User) error
	Update(name string, identity User) error
	Delete(name string) error

	Get(name string) (User, error)
	GetVerifier(name string) (Verifier, error)
	GetVerifierFromAuth0(name string) (Verifier, error)
	GetDefaultVerifier() (Verifier, error)

	Reload() error // Reload users from adapter
	Save() error   // Save users to adapter
	List() []User  // List all users

	Validators() []string
	CreateJWT(name string) (string, string, error)

	Close()
}

type identityManager struct {
	root *identity

	identities map[string]*identity
	tenants    map[string]*auth0Tenant

	auth0UserIdentityMap map[string]string

	adapter  Adapter
	autosave bool
	logger   log.Logger

	jwtRealm  string
	jwtSecret []byte

	lock sync.RWMutex
}

type Config struct {
	Adapter   Adapter
	Superuser User
	JWTRealm  string
	JWTSecret string
	Logger    log.Logger
}

func New(config Config) (Manager, error) {
	im := &identityManager{
		identities:           map[string]*identity{},
		tenants:              map[string]*auth0Tenant{},
		auth0UserIdentityMap: map[string]string{},
		adapter:              config.Adapter,
		jwtRealm:             config.JWTRealm,
		jwtSecret:            []byte(config.JWTSecret),
		logger:               config.Logger,
	}

	if im.logger == nil {
		im.logger = log.New("")
	}

	if im.adapter == nil {
		return nil, fmt.Errorf("no adapter provided")
	}

	config.Superuser.Superuser = true
	identity, err := im.create(config.Superuser)
	if err != nil {
		return nil, err
	}

	im.root = identity

	err = im.Reload()
	if err != nil {
		return nil, err
	}

	im.Save()

	return im, nil
}

func (im *identityManager) Close() {
	im.lock.Lock()
	defer im.lock.Unlock()

	im.adapter = nil
	im.auth0UserIdentityMap = map[string]string{}
	im.identities = map[string]*identity{}
	im.root = nil

	for _, t := range im.tenants {
		t.Cancel()
	}

	im.tenants = map[string]*auth0Tenant{}
}

func (im *identityManager) Reload() error {
	users, err := im.adapter.LoadIdentities()
	if err != nil {
		return fmt.Errorf("load users from adapter: %w", err)
	}

	im.lock.Lock()
	defer im.lock.Unlock()

	im.autosave = false
	defer func() {
		im.autosave = true
	}()

	names := []string{}

	for name := range im.identities {
		names = append(names, name)
	}

	for _, name := range names {
		im.delete(name)
	}

	for _, u := range users {
		if im.root != nil && u.Name == im.root.user.Name {
			continue
		}

		_, ok := im.identities[u.Name]
		if ok {
			continue
		}

		if err := u.Validate(); err != nil {
			return fmt.Errorf("invalid user from adapter: %s, %w", u.Name, err)
		}

		identity, err := im.create(u)
		if err != nil {
			continue
		}

		im.identities[identity.user.Name] = identity
	}

	return nil
}

func (im *identityManager) Create(u User) error {
	if err := u.Validate(); err != nil {
		return err
	}

	im.lock.Lock()
	defer im.lock.Unlock()

	if im.root != nil && im.root.user.Name == u.Name {
		return fmt.Errorf("identity already exists")
	}

	_, ok := im.identities[u.Name]
	if ok {
		return fmt.Errorf("identity already exists")
	}

	identity, err := im.create(u)
	if err != nil {
		return err
	}

	im.identities[identity.user.Name] = identity

	if im.autosave {
		im.save()
	}

	return nil
}

func (im *identityManager) create(u User) (*identity, error) {
	u = u.clone()
	identity := u.marshalIdentity()

	if len(identity.user.Auth.API.Auth0.User) != 0 {
		if _, ok := im.auth0UserIdentityMap[identity.user.Auth.API.Auth0.User]; ok {
			return nil, fmt.Errorf("the Auth0 user has already an identity")
		}

		auth0Key := identity.user.Auth.API.Auth0.Tenant.key()

		if tenant, ok := im.tenants[auth0Key]; !ok {
			tenant, err := newAuth0Tenant(identity.user.Auth.API.Auth0.Tenant)
			if err != nil {
				return nil, err
			}

			im.tenants[auth0Key] = tenant
			identity.tenant = tenant
		} else {
			tenant.AddClientID(identity.user.Auth.API.Auth0.Tenant.ClientID)
			identity.tenant = tenant
		}

		im.auth0UserIdentityMap[identity.user.Auth.API.Auth0.User] = u.Name
	}

	identity.valid = true

	im.logger.Debug().WithField("name", identity.Name()).Log("Identity created")

	return identity, nil
}

func (im *identityManager) Update(name string, u User) error {
	if err := u.Validate(); err != nil {
		return err
	}

	im.lock.Lock()
	defer im.lock.Unlock()

	if im.root.user.Name == name {
		return fmt.Errorf("this identity can't be updated")
	}

	oldidentity, ok := im.identities[name]
	if !ok {
		return fmt.Errorf("not found")
	}

	if name != u.Name {
		_, err := im.getIdentity(u.Name)
		if err == nil {
			return fmt.Errorf("identity already exist")
		}
	}

	err := im.delete(name)
	if err != nil {
		return err
	}

	identity, err := im.create(u)
	if err != nil {
		if identity, err := im.create(oldidentity.user); err != nil {
			return err
		} else {
			im.identities[identity.user.Name] = identity
		}

		return err
	}

	im.identities[identity.user.Name] = identity

	im.logger.Debug().WithFields(log.Fields{
		"oldname": name,
		"newname": identity.Name(),
	}).Log("Identity updated")

	if im.autosave {
		im.save()
	}

	return nil
}

func (im *identityManager) Delete(name string) error {
	im.lock.Lock()
	defer im.lock.Unlock()

	err := im.delete(name)
	if err != nil {
		return err
	}

	return nil
}

func (im *identityManager) delete(name string) error {
	if im.root.user.Name == name {
		return fmt.Errorf("this identity can't be removed")
	}

	identity, ok := im.identities[name]
	if !ok {
		return fmt.Errorf("not found")
	}

	delete(im.identities, name)

	identity.lock.Lock()
	identity.valid = false
	identity.lock.Unlock()

	if len(identity.user.Auth.API.Auth0.User) == 0 {
		if im.autosave {
			im.save()
		}

		return nil
	}

	delete(im.auth0UserIdentityMap, identity.user.Auth.API.Auth0.User)

	// find out if the tenant is still used somewhere else
	found := false
	for _, i := range im.identities {
		if i.tenant == identity.tenant {
			found = true
			break
		}
	}

	if !found {
		identity.tenant.Cancel()
		delete(im.tenants, identity.user.Auth.API.Auth0.Tenant.key())

		if im.autosave {
			im.save()
		}

		return nil
	}

	// find out if the tenant's clientid is still used somewhere else
	found = false
	for _, i := range im.identities {
		if len(i.user.Auth.API.Auth0.User) == 0 {
			continue
		}

		if i.user.Auth.API.Auth0.Tenant.ClientID == identity.user.Auth.API.Auth0.Tenant.ClientID {
			found = true
			break
		}
	}

	if !found {
		identity.tenant.RemoveClientID(identity.user.Auth.API.Auth0.Tenant.ClientID)
	}

	if im.autosave {
		if err := im.save(); err != nil {
			return err
		}
	}

	return nil
}

func (im *identityManager) getIdentity(name string) (*identity, error) {
	var identity *identity = nil

	if im.root.user.Name == name {
		identity = im.root
	} else {
		identity = im.identities[name]
	}

	if identity == nil {
		return nil, fmt.Errorf("not found")
	}

	identity.jwtRealm = im.jwtRealm
	identity.jwtKeyFunc = func(*jwtgo.Token) (interface{}, error) { return im.jwtSecret, nil }

	return identity, nil
}

func (im *identityManager) Get(name string) (User, error) {
	im.lock.RLock()
	defer im.lock.RUnlock()

	identity, err := im.getIdentity(name)
	if err != nil {
		return User{}, err
	}

	user := identity.user.clone()

	return user, nil
}

func (im *identityManager) GetVerifier(name string) (Verifier, error) {
	im.lock.RLock()
	defer im.lock.RUnlock()

	return im.getIdentity(name)
}

func (im *identityManager) GetVerifierFromAuth0(name string) (Verifier, error) {
	im.lock.RLock()
	defer im.lock.RUnlock()

	name, ok := im.auth0UserIdentityMap[name]
	if !ok {
		return nil, fmt.Errorf("not found")
	}

	return im.getIdentity(name)
}

func (im *identityManager) GetDefaultVerifier() (Verifier, error) {
	return im.root, nil
}

func (im *identityManager) List() []User {
	im.lock.RLock()
	defer im.lock.RUnlock()

	users := []User{}

	for _, identity := range im.identities {
		users = append(users, identity.user.clone())
	}

	return users
}

func (im *identityManager) Save() error {
	im.lock.RLock()
	defer im.lock.RUnlock()

	return im.save()
}

func (im *identityManager) save() error {
	users := []User{}

	for _, u := range im.identities {
		users = append(users, u.user)
	}

	return im.adapter.SaveIdentities(users)
}

func (im *identityManager) Autosave(auto bool) {
	im.lock.Lock()
	defer im.lock.Unlock()

	im.autosave = auto
}

func (im *identityManager) Validators() []string {
	validators := []string{"localjwt"}

	im.lock.RLock()
	defer im.lock.RUnlock()

	for _, t := range im.tenants {
		for _, clientid := range t.clientIDs {
			validators = append(validators, fmt.Sprintf("auth0 domain=%s audience=%s clientid=%s", t.domain, t.audience, clientid))
		}
	}

	return validators
}

func (im *identityManager) CreateJWT(name string) (string, string, error) {
	im.lock.RLock()
	defer im.lock.RUnlock()

	identity, err := im.getIdentity(name)
	if err != nil {
		return "", "", err
	}

	now := time.Now()
	accessExpires := now.Add(time.Minute * 10)
	refreshExpires := now.Add(time.Hour * 24)

	// Create access token
	accessToken := jwtgo.NewWithClaims(jwtgo.SigningMethodHS256, jwtgo.MapClaims{
		"iss":    im.jwtRealm,
		"sub":    identity.Name(),
		"usefor": "access",
		"iat":    now.Unix(),
		"exp":    accessExpires.Unix(),
		"exi":    uint64(accessExpires.Sub(now).Seconds()),
		"jti":    uuid.New().String(),
	})

	// Generate encoded access token
	at, err := accessToken.SignedString(im.jwtSecret)
	if err != nil {
		return "", "", err
	}

	// Create refresh token
	refreshToken := jwtgo.NewWithClaims(jwtgo.SigningMethodHS256, jwtgo.MapClaims{
		"iss":    im.jwtRealm,
		"sub":    identity.Name(),
		"usefor": "refresh",
		"iat":    now.Unix(),
		"exp":    refreshExpires.Unix(),
		"exi":    uint64(refreshExpires.Sub(now).Seconds()),
		"jti":    uuid.New().String(),
	})

	// Generate encoded refresh token
	rt, err := refreshToken.SignedString(im.jwtSecret)
	if err != nil {
		return "", "", err
	}

	return at, rt, nil
}

type Auth0Tenant struct {
	Domain   string `json:"domain"`
	Audience string `json:"audience"`
	ClientID string `json:"client_id"`
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

	lock sync.Mutex
}

func newAuth0Tenant(tenant Auth0Tenant) (*auth0Tenant, error) {
	t := &auth0Tenant{
		domain:    tenant.Domain,
		issuer:    "https://" + tenant.Domain + "/",
		audience:  tenant.Audience,
		clientIDs: []string{tenant.ClientID},
		certs:     nil,
	}

	url := t.issuer + ".well-known/jwks.json"
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

func (a *auth0Tenant) AddClientID(clientid string) {
	a.lock.Lock()
	defer a.lock.Unlock()

	found := false
	for _, id := range a.clientIDs {
		if id == clientid {
			found = true
			break
		}
	}

	if found {
		return
	}

	a.clientIDs = append(a.clientIDs, clientid)
}

func (a *auth0Tenant) RemoveClientID(clientid string) {
	a.lock.Lock()
	defer a.lock.Unlock()

	clientids := []string{}

	for _, id := range a.clientIDs {
		if id == clientid {
			continue
		}

		clientids = append(clientids, id)
	}

	a.clientIDs = clientids
}
