package store

import (
	"testing"

	"github.com/datarhei/core/v16/iam/access"
	"github.com/datarhei/core/v16/iam/identity"
	"github.com/stretchr/testify/require"
)

func TestSetPoliciesCommand(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	identity := identity.User{
		Name: "foobar",
	}

	err = s.applyCommand(Command{
		Operation: OpAddIdentity,
		Data: CommandAddIdentity{
			Identity: identity,
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.data.Users.Users))
	require.Equal(t, 0, len(s.data.Policies.Policies))

	err = s.applyCommand(Command{
		Operation: OpSetPolicies,
		Data: CommandSetPolicies{
			Name: "foobar",
			Policies: []access.Policy{
				{
					Name:     "bla",
					Domain:   "bla",
					Resource: "bla",
					Actions:  []string{},
				},
				{
					Name:     "foo",
					Domain:   "foo",
					Resource: "foo",
					Actions:  []string{},
				},
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.data.Users.Users))
	require.Equal(t, 2, len(s.data.Policies.Policies["foobar"]))
}

func TestSetPolicies(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	identity := identity.User{
		Name: "foobar",
	}

	policies := []access.Policy{
		{
			Name:     "bla",
			Domain:   "bla",
			Resource: "bla",
			Actions:  []string{},
		},
		{
			Name:     "foo",
			Domain:   "foo",
			Resource: "foo",
			Actions:  []string{},
		},
	}

	err = s.setPolicies(CommandSetPolicies{
		Name:     "foobar",
		Policies: policies,
	})
	require.Error(t, err)
	require.Equal(t, 0, len(s.data.Policies.Policies["foobar"]))

	err = s.addIdentity(CommandAddIdentity{
		Identity: identity,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.data.Users.Users))
	require.Equal(t, 0, len(s.data.Policies.Policies))

	users := s.IAMIdentityGet("foobar")
	require.NotEmpty(t, users.Users)

	updatedAt := users.Users[0].UpdatedAt

	err = s.setPolicies(CommandSetPolicies{
		Name:     "foobar",
		Policies: policies,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.data.Users.Users))
	require.Equal(t, 2, len(s.data.Policies.Policies["foobar"]))

	users = s.IAMIdentityGet("foobar")
	require.NotEmpty(t, users.Users)

	require.False(t, updatedAt.Equal(users.Users[0].UpdatedAt))
}
