package iam

import (
	"github.com/datarhei/core/v16/iam/access"
	"github.com/datarhei/core/v16/iam/identity"
	"github.com/datarhei/core/v16/log"
)

type Enforcer interface {
	Enforce(name, domain, rtype, resource, action string) bool
}

type IAM interface {
	Enforcer

	HasDomain(domain string) bool
	ListDomains() []string

	HasPolicy(name, domain string, types []string, resource string, actions []string) bool
	AddPolicy(name, domain string, types []string, resource string, actions []string) error
	RemovePolicy(name, domain string, types []string, resource string, actions []string) error
	ListPolicies(name, domain string, types []string, resource string, actions []string) []access.Policy
	ReloadPolicies() error

	Validators() []string

	CreateIdentity(u identity.User) error
	GetIdentity(name string) (identity.User, error)
	UpdateIdentity(name string, u identity.User) error
	DeleteIdentity(name string) error
	ListIdentities() []identity.User
	ReloadIndentities() error

	GetVerifier(name string) (identity.Verifier, error)
	GetVerifierFromAuth0(name string) (identity.Verifier, error)
	GetDefaultVerifier() identity.Verifier

	CreateJWT(name string) (string, string, error)

	Close()
}

type iam struct {
	im identity.Manager
	am access.Manager

	logger log.Logger
}

type Config struct {
	PolicyAdapter   access.Adapter
	IdentityAdapter identity.Adapter
	Superuser       identity.User
	JWTRealm        string
	JWTSecret       string
	Logger          log.Logger
}

func New(config Config) (IAM, error) {
	im, err := identity.New(identity.Config{
		Adapter:   config.IdentityAdapter,
		Superuser: config.Superuser,
		JWTRealm:  config.JWTRealm,
		JWTSecret: config.JWTSecret,
		Logger:    config.Logger,
	})
	if err != nil {
		return nil, err
	}

	am, err := access.New(access.Config{
		Adapter: config.PolicyAdapter,
		Logger:  config.Logger,
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

func (i *iam) Enforce(name, domain, rtype, resource, action string) bool {
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
		"type":      rtype,
		"resource":  resource,
		"action":    action,
		"superuser": superuser,
	})

	if superuser {
		name = "$superuser"
	}

	ok, rule := i.am.Enforce(name, domain, rtype, resource, action)

	if !ok {
		l.Log("no match")
	} else {
		if name == "$superuser" {
			rule = ""
		}

		l.WithField("rule", rule).Log("match")
	}

	return ok
}

func (i *iam) CreateIdentity(u identity.User) error {
	return i.im.Create(u)
}

func (i *iam) GetIdentity(name string) (identity.User, error) {
	return i.im.Get(name)
}

func (i *iam) UpdateIdentity(name string, u identity.User) error {
	return i.im.Update(name, u)
}

func (i *iam) DeleteIdentity(name string) error {
	return i.im.Delete(name)
}

func (i *iam) ListIdentities() []identity.User {
	return i.im.List()
}

func (i *iam) ReloadIndentities() error {
	return i.im.Reload()
}

func (i *iam) GetVerifier(name string) (identity.Verifier, error) {
	return i.im.GetVerifier(name)
}

func (i *iam) GetVerifierFromAuth0(name string) (identity.Verifier, error) {
	return i.im.GetVerifierFromAuth0(name)
}

func (i *iam) GetDefaultVerifier() identity.Verifier {
	v, _ := i.im.GetDefaultVerifier()

	return v
}

func (i *iam) CreateJWT(name string) (string, string, error) {
	return i.im.CreateJWT(name)
}

func (i *iam) HasDomain(domain string) bool {
	return i.am.HasDomain(domain)
}

func (i *iam) ListDomains() []string {
	return i.am.ListDomains()
}

func (i *iam) Validators() []string {
	return i.im.Validators()
}

func (i *iam) HasPolicy(name, domain string, types []string, resource string, actions []string) bool {
	if len(name) == 0 {
		name = "$anon"
	}

	if len(domain) == 0 {
		domain = "$none"
	}

	return i.am.HasPolicy(name, domain, types, resource, actions)
}

func (i *iam) AddPolicy(name, domain string, types []string, resource string, actions []string) error {
	if len(name) == 0 {
		name = "$anon"
	}

	if len(domain) == 0 {
		domain = "$none"
	}

	if name != "$anon" {
		if user, err := i.im.Get(name); err == nil {
			// Update the "updatedAt" field
			i.im.Update(name, user)
		}
	}

	return i.am.AddPolicy(name, domain, types, resource, actions)
}

func (i *iam) RemovePolicy(name, domain string, types []string, resource string, actions []string) error {
	if len(name) != 0 && name != "$anon" {
		if user, err := i.im.Get(name); err == nil {
			// Update the "updatedAt" field
			i.im.Update(name, user)
		}
	}

	return i.am.RemovePolicy(name, domain, types, resource, actions)
}

func (i *iam) ListPolicies(name, domain string, types []string, resource string, actions []string) []access.Policy {
	return i.am.ListPolicies(name, domain, types, resource, actions)
}

func (i *iam) ReloadPolicies() error {
	return i.am.ReloadPolicies()
}
