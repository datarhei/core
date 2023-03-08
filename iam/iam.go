package iam

import (
	"github.com/datarhei/core/v16/io/fs"
	"github.com/datarhei/core/v16/log"
)

type IAM interface {
	Enforce(name, domain, resource, action string) bool
	HasDomain(domain string) bool

	AddPolicy(name, domain, resource string, actions []string) bool
	RemovePolicy(name, domain, resource string, actions []string) bool

	ListPolicies(name, domain, resource string, actions []string) []Policy

	Validators() []string

	CreateIdentity(u User) error
	GetIdentity(name string) (User, error)
	UpdateIdentity(name string, u User) error
	DeleteIdentity(name string) error
	ListIdentities() []User
	SaveIdentities() error

	GetVerifier(name string) (IdentityVerifier, error)
	GetVerfierFromAuth0(name string) (IdentityVerifier, error)
	GetDefaultVerifier() (IdentityVerifier, error)

	CreateJWT(name string) (string, string, error)

	Close()
}

type iam struct {
	im IdentityManager
	am AccessManager

	logger log.Logger
}

type Config struct {
	FS        fs.Filesystem
	Superuser User
	JWTRealm  string
	JWTSecret string
	Logger    log.Logger
}

func NewIAM(config Config) (IAM, error) {
	im, err := NewIdentityManager(IdentityConfig{
		FS:        config.FS,
		Superuser: config.Superuser,
		JWTRealm:  config.JWTRealm,
		JWTSecret: config.JWTSecret,
		Logger:    config.Logger,
	})
	if err != nil {
		return nil, err
	}

	am, err := NewAccessManager(AccessConfig{
		FS:     config.FS,
		Logger: config.Logger,
	})
	if err != nil {
		return nil, err
	}

	iam := &iam{
		im:     im,
		am:     am,
		logger: config.Logger,
	}

	if iam.logger == nil {
		iam.logger = log.New("")
	}

	return iam, nil
}

func (i *iam) Close() {
	i.im.Close()
	i.im = nil

	i.am = nil
}

func (i *iam) Enforce(name, domain, resource, action string) bool {
	if len(name) == 0 {
		name = "$anon"
	}

	if len(domain) == 0 {
		domain = "$none"
	}

	superuser := false

	if identity, err := i.im.GetVerifier(name); err == nil {
		if identity.IsSuperuser() {
			superuser = true
		}
	}

	l := i.logger.Debug().WithFields(log.Fields{
		"subject":   name,
		"domain":    domain,
		"resource":  resource,
		"action":    action,
		"superuser": superuser,
	})

	if superuser {
		name = "$superuser"
	}

	ok, rule := i.am.Enforce(name, domain, resource, action)

	if !ok {
		l.Log("no match")
	} else {
		l.WithField("rule", rule).Log("match")
	}

	return ok
}

func (i *iam) CreateIdentity(u User) error {
	return i.im.Create(u)
}

func (i *iam) GetIdentity(name string) (User, error) {
	return i.im.Get(name)
}

func (i *iam) UpdateIdentity(name string, u User) error {
	return i.im.Update(name, u)
}

func (i *iam) DeleteIdentity(name string) error {
	return i.im.Delete(name)
}

func (i *iam) ListIdentities() []User {
	return nil
}

func (i *iam) SaveIdentities() error {
	return i.im.Save()
}

func (i *iam) GetVerifier(name string) (IdentityVerifier, error) {
	return i.im.GetVerifier(name)
}

func (i *iam) GetVerfierFromAuth0(name string) (IdentityVerifier, error) {
	return i.im.GetVerifierFromAuth0(name)
}

func (i *iam) GetDefaultVerifier() (IdentityVerifier, error) {
	return i.im.GetDefaultVerifier()
}

func (i *iam) CreateJWT(name string) (string, string, error) {
	return i.im.CreateJWT(name)
}

func (i *iam) HasDomain(domain string) bool {
	return i.am.HasGroup(domain)
}

func (i *iam) Validators() []string {
	return i.im.Validators()
}

func (i *iam) AddPolicy(name, domain, resource string, actions []string) bool {
	if len(name) == 0 {
		name = "$anon"
	}

	if len(domain) == 0 {
		domain = "$none"
	}

	return i.am.AddPolicy(name, domain, resource, actions)
}

func (i *iam) RemovePolicy(name, domain, resource string, actions []string) bool {
	return i.am.RemovePolicy(name, domain, resource, actions)
}

func (i *iam) ListPolicies(name, domain, resource string, actions []string) []Policy {
	return i.am.ListPolicies(name, domain, resource, actions)
}
