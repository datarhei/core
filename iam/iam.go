package iam

import (
	"github.com/datarhei/core/v16/io/fs"
	"github.com/datarhei/core/v16/log"
)

type IAM interface {
	Enforce(user, domain, resource, action string) bool
	IsDomain(domain string) bool

	AddPolicy(username, domain, resource, actions string) bool
	RemovePolicy(username, domain, resource, actions string) bool

	Validators() []string

	GetIdentity(name string) (IdentityVerifier, error)
	GetIdentityByAuth0(name string) (IdentityVerifier, error)
	GetDefaultIdentity() (IdentityVerifier, error)

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

func (i *iam) Enforce(user, domain, resource, action string) bool {
	superuser := false

	if identity, err := i.im.GetVerifier(user); err == nil {
		if identity.IsSuperuser() {
			superuser = true
		}
	}

	l := i.logger.Debug().WithFields(log.Fields{
		"subject":   user,
		"domain":    domain,
		"resource":  resource,
		"action":    action,
		"superuser": superuser,
	})

	if superuser {
		user = "$superuser"
	}

	ok, rule := i.am.Enforce(user, domain, resource, action)

	if !ok {
		l.Log("no match")
	} else {
		l.WithField("rule", rule).Log("match")
	}

	return ok
}

func (i *iam) GetIdentity(name string) (IdentityVerifier, error) {
	return i.im.GetVerifier(name)
}

func (i *iam) GetIdentityByAuth0(name string) (IdentityVerifier, error) {
	return i.im.GetVerifierByAuth0(name)
}

func (i *iam) GetDefaultIdentity() (IdentityVerifier, error) {
	return i.im.GetDefaultVerifier()
}

func (i *iam) CreateJWT(name string) (string, string, error) {
	return i.im.CreateJWT(name)
}

func (i *iam) IsDomain(domain string) bool {
	return i.am.HasGroup(domain)
}

func (i *iam) Validators() []string {
	return i.im.Validators()
}

func (i *iam) AddPolicy(username, domain, resource, actions string) bool {
	return i.am.AddPolicy(username, domain, resource, actions)
}

func (i *iam) RemovePolicy(username, domain, resource, actions string) bool {
	return i.am.RemovePolicy(username, domain, resource, actions)
}
