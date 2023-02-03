package iam

type AccessEnforcer interface {
	Enforce(name, domain, resource, action string) bool
}

type AccessManager interface {
	AccessEnforcer

	AddPolicy()
}

type access struct {
}

func NewAccessManager() (AccessManager, error) {
	return &access{}, nil
}

func (a *access) AddPolicy()                                         {}
func (a *access) Enforce(name, domain, resource, action string) bool { return false }
