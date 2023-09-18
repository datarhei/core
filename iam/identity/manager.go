package identity

import (
	"fmt"
	"sync"
	"time"

	"github.com/datarhei/core/v16/log"
	jwtgo "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

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

	userlist UserList

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
		userlist:             NewUserList(),
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
	identity, err := im.createIdentity(config.Superuser)
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
	im.userlist = nil
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

	userlist := NewUserList()

	now := time.Now()

	for _, u := range users {
		if u.CreatedAt.IsZero() {
			u.CreatedAt = now
		}

		if u.UpdatedAt.IsZero() {
			u.UpdatedAt = now
		}

		err := userlist.Add(u)
		if err != nil {
			return fmt.Errorf("invalid user %s from adapter: %w", u.Name, err)
		}
	}

	userlist.Delete(im.root.user.Name)
	userlist.Delete(im.root.user.Alias)

	im.lock.Lock()
	defer im.lock.Unlock()

	im.autosave = false
	defer func() {
		im.autosave = true
	}()

	for name := range im.identities {
		im.delete(name)
	}

	im.userlist = userlist

	for _, u := range userlist.List() {
		identity, err := im.createIdentity(u)
		if err != nil {
			continue
		}

		im.identities[u.Name] = identity
	}

	return nil
}

func (im *identityManager) Create(u User) error {
	im.lock.Lock()
	defer im.lock.Unlock()

	if im.root.user.Name == u.Name || im.root.user.Alias == u.Name {
		return fmt.Errorf("the identity %s already exists", u.Name)
	}

	if len(u.Alias) != 0 {
		if im.root.user.Name == u.Alias || im.root.user.Alias == u.Alias {
			return fmt.Errorf("the identity %s already exists", u.Alias)
		}
	}

	now := time.Now()

	u.CreatedAt = now
	u.UpdatedAt = now

	err := im.userlist.Add(u)
	if err != nil {
		return err
	}

	identity, err := im.createIdentity(u)
	if err != nil {
		return err
	}

	im.identities[identity.user.Name] = identity

	if im.autosave {
		im.save()
	}

	return nil
}

func (im *identityManager) createIdentity(u User) (*identity, error) {
	u = u.clone()
	identity := u.marshalIdentity()

	if len(identity.user.Auth.API.Auth0.User) != 0 {
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

		im.auth0UserIdentityMap[identity.user.Auth.API.Auth0.User] = identity.user.Name
	}

	identity.valid = true

	im.logger.Debug().WithField("name", identity.Name()).Log("Identity created")

	return identity, nil
}

func (im *identityManager) Update(nameOrAlias string, u User) error {
	im.lock.Lock()
	defer im.lock.Unlock()

	if im.root.user.Name == nameOrAlias || im.root.user.Alias == nameOrAlias {
		return fmt.Errorf("the identity %s cannot be updated", nameOrAlias)
	}

	if im.root.user.Name == u.Name || im.root.user.Alias == u.Name {
		return fmt.Errorf("the identity %s already exists", u.Name)
	}

	if len(u.Alias) != 0 {
		if im.root.user.Name == u.Alias || im.root.user.Alias == u.Alias {
			return fmt.Errorf("the identity %s already exists", u.Alias)
		}
	}

	oldUser, err := im.userlist.Get(nameOrAlias)
	if err != nil {
		return err
	}

	u.CreatedAt = oldUser.CreatedAt
	u.UpdatedAt = time.Now()

	if err = im.userlist.Update(nameOrAlias, u); err != nil {
		return err
	}

	user, err := im.userlist.Get(u.Name)
	if err != nil {
		return err
	}

	_, ok := im.identities[oldUser.Name]
	if !ok {
		return fmt.Errorf("identity not found")
	}

	identity, err := im.createIdentity(u)
	if err != nil {
		return err
	}

	im.identities[user.Name] = identity

	im.logger.Debug().WithFields(log.Fields{
		"oldname": oldUser.Name,
		"newname": user.Name,
	}).Log("Identity updated")

	if im.autosave {
		im.save()
	}

	return nil
}

func (im *identityManager) Delete(nameOrAlias string) error {
	im.lock.Lock()
	defer im.lock.Unlock()

	err := im.delete(nameOrAlias)
	if err != nil {
		return err
	}

	return nil
}

func (im *identityManager) delete(nameOrAlias string) error {
	if im.root.user.Name == nameOrAlias || im.root.user.Alias == nameOrAlias {
		return fmt.Errorf("this identity can't be removed")
	}

	user, err := im.userlist.Get(nameOrAlias)
	if err != nil {
		return err
	}

	identity, ok := im.identities[user.Name]
	if !ok {
		return fmt.Errorf("identity not found")
	}

	im.userlist.Delete(user.Name)
	delete(im.identities, user.Name)

	identity.lock.Lock()
	identity.valid = false
	identity.lock.Unlock()

	if len(user.Auth.API.Auth0.User) == 0 {
		if im.autosave {
			im.save()
		}

		return nil
	}

	delete(im.auth0UserIdentityMap, user.Auth.API.Auth0.User)

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

func (im *identityManager) getIdentity(nameOrAlias string) (*identity, error) {
	var identity *identity = nil

	if im.root.user.Name == nameOrAlias || im.root.user.Alias == nameOrAlias {
		identity = im.root
	} else {
		user, err := im.userlist.Get(nameOrAlias)
		if err != nil {
			return nil, fmt.Errorf("identity not found")
		}
		identity = im.identities[user.Name]
	}

	if identity == nil {
		return nil, fmt.Errorf("identity not found")
	}

	identity.jwtRealm = im.jwtRealm
	identity.jwtKeyFunc = func(*jwtgo.Token) (interface{}, error) { return im.jwtSecret, nil }

	return identity, nil
}

func (im *identityManager) Get(nameOrAlias string) (User, error) {
	im.lock.RLock()
	defer im.lock.RUnlock()

	identity, err := im.getIdentity(nameOrAlias)
	if err != nil {
		return User{}, err
	}

	user := identity.user.clone()

	return user, nil
}

func (im *identityManager) GetVerifier(nameOrAlias string) (Verifier, error) {
	im.lock.RLock()
	defer im.lock.RUnlock()

	return im.getIdentity(nameOrAlias)
}

func (im *identityManager) GetVerifierFromAuth0(auth0Name string) (Verifier, error) {
	im.lock.RLock()
	defer im.lock.RUnlock()

	name, ok := im.auth0UserIdentityMap[auth0Name]
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

	return im.userlist.List()
}

func (im *identityManager) Save() error {
	im.lock.RLock()
	defer im.lock.RUnlock()

	return im.save()
}

func (im *identityManager) save() error {
	users := im.userlist.List()

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

func (im *identityManager) CreateJWT(nameOrAlias string) (string, string, error) {
	im.lock.RLock()
	defer im.lock.RUnlock()

	identity, err := im.getIdentity(nameOrAlias)
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
