package iam

import "github.com/datarhei/core/v16/io/fs"

type IAM interface {
	Enforce(user, domain, resource, action string) (bool, string)
	IsDomain(domain string) bool

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
}

type Config struct {
	FS        fs.Filesystem
	Superuser User
	JWTSecret string
}

func NewIAM(config Config) (IAM, error) {
	im, err := NewIdentityManager(IdentityConfig{
		FS:        config.FS,
		Superuser: config.Superuser,
		JWTSecret: config.JWTSecret,
	})
	if err != nil {
		return nil, err
	}

	am, err := NewAccessManager(config.FS)
	if err != nil {
		return nil, err
	}

	return &iam{
		im: im,
		am: am,
	}, nil
}

func (i *iam) Close() {
	i.im.Close()
	i.im = nil

	i.am = nil

	return
}

func (i *iam) Enforce(user, domain, resource, action string) (bool, string) {
	return i.am.Enforce(user, domain, resource, action)
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
	return false
}

func (i *iam) Validators() []string {
	return i.im.Validators()
}
