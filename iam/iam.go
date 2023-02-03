package iam

type IAM interface {
	Enforce(user, domain, resource, action string) bool

	GetIdentity(name string) IdentityVerifier
	GetIdentityByAuth0(name string) IdentityVerifier
}

type iam struct{}

func NewIAM() (IAM, error) {
	return &iam{}, nil
}

func (i *iam) Enforce(user, domain, resource, action string) bool {
	return false
}

func (i *iam) GetIdentity(name string) IdentityVerifier {
	return nil
}

func (i *iam) GetIdentityByAuth0(name string) IdentityVerifier {
	return nil
}
