package iam

import "github.com/datarhei/core/v16/io/fs"

type IAM interface {
	Enforce(user, domain, resource, action string) bool

	GetIdentity(name string) (IdentityVerifier, error)
	GetIdentityByAuth0(name string) (IdentityVerifier, error)

	Close()
}

type iam struct {
	im IdentityManager
	am AccessManager
}

func NewIAM(fs fs.Filesystem, superuser User) (IAM, error) {
	im, err := NewIdentityManager(fs, superuser)
	if err != nil {
		return nil, err
	}

	am, err := NewAccessManager(fs)
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

func (i *iam) Enforce(user, domain, resource, action string) bool {
	return i.am.Enforce(user, domain, resource, action)
}

func (i *iam) GetIdentity(name string) (IdentityVerifier, error) {
	return i.im.GetVerifier(name)
}

func (i *iam) GetIdentityByAuth0(name string) (IdentityVerifier, error) {
	return i.im.GetVerifierByAuth0(name)
}
