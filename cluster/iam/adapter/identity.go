package adapter

import (
	"github.com/datarhei/core/v16/cluster/store"
	iamidentity "github.com/datarhei/core/v16/iam/identity"
)

type identityAdapter struct {
	store store.Store
}

func NewIdentityAdapter(store store.Store) (iamidentity.Adapter, error) {
	a := &identityAdapter{
		store: store,
	}

	return a, nil
}

func (a *identityAdapter) LoadIdentities() ([]iamidentity.User, error) {
	users := a.store.UserList()

	return users.Users, nil
}

func (a *identityAdapter) SaveIdentities([]iamidentity.User) error {
	return nil
}
