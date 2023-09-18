package store

import (
	"testing"
	"time"

	"github.com/datarhei/core/v16/iam/access"
	"github.com/datarhei/core/v16/iam/identity"
	"github.com/stretchr/testify/require"
)

func TestAddIdentityCommand(t *testing.T) {
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
}

func TestAddIdentity(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	idty := identity.User{
		Name: "foobar",
	}

	err = s.addIdentity(CommandAddIdentity{
		Identity: idty,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.data.Users.Users))
	require.Equal(t, 0, len(s.data.Policies.Policies))

	err = s.addIdentity(CommandAddIdentity{
		Identity: idty,
	})
	require.Error(t, err)
	require.Equal(t, 1, len(s.data.Users.Users))
	require.Equal(t, 0, len(s.data.Policies.Policies))

	u := s.GetUser("foobar")
	require.Equal(t, 1, len(u.Users))

	user := u.Users[0]

	require.Equal(t, user.CreatedAt, user.UpdatedAt)
	require.NotEqual(t, time.Time{}, user.CreatedAt)
}

func TestAddIdentityWithAlias(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	idty := identity.User{
		Name: "foobar",
	}

	err = s.addIdentity(CommandAddIdentity{
		Identity: idty,
	})
	require.NoError(t, err)

	idty = identity.User{
		Name:  "foobaz",
		Alias: "foobar",
	}

	err = s.addIdentity(CommandAddIdentity{
		Identity: idty,
	})
	require.Error(t, err)

	idty = identity.User{
		Name:  "foobaz",
		Alias: "foobaz",
	}

	err = s.addIdentity(CommandAddIdentity{
		Identity: idty,
	})
	require.NoError(t, err)

	idty = identity.User{
		Name:  "barfoo",
		Alias: "foobaz",
	}

	err = s.addIdentity(CommandAddIdentity{
		Identity: idty,
	})
	require.Error(t, err)
}

func TestRemoveIdentityCommand(t *testing.T) {
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

	err = s.applyCommand(Command{
		Operation: OpRemoveIdentity,
		Data: CommandRemoveIdentity{
			Name: "foobar",
		},
	})
	require.NoError(t, err)
	require.Equal(t, 0, len(s.data.Users.Users))
}

func TestRemoveIdentity(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	identity := identity.User{
		Name: "foobar",
	}

	err = s.addIdentity(CommandAddIdentity{
		Identity: identity,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.data.Users.Users))
	require.Equal(t, 0, len(s.data.Policies.Policies))

	err = s.removeIdentity(CommandRemoveIdentity{
		Name: "foobar",
	})
	require.NoError(t, err)
	require.Equal(t, 0, len(s.data.Users.Users))
	require.Equal(t, 0, len(s.data.Policies.Policies))

	err = s.removeIdentity(CommandRemoveIdentity{
		Name: "foobar",
	})
	require.NoError(t, err)
	require.Equal(t, 0, len(s.data.Users.Users))
	require.Equal(t, 0, len(s.data.Policies.Policies))
}

func TestRemoveIdentityWithAlias(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	idty := identity.User{
		Name:  "foobar",
		Alias: "foobaz",
	}

	err = s.addIdentity(CommandAddIdentity{
		Identity: idty,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.data.Users.Users))

	err = s.removeIdentity(CommandRemoveIdentity{
		Name: "foobaz",
	})
	require.NoError(t, err)
	require.Equal(t, 0, len(s.data.Users.Users))

	err = s.removeIdentity(CommandRemoveIdentity{
		Name: "foobar",
	})
	require.NoError(t, err)
	require.Equal(t, 0, len(s.data.Users.Users))
}

func TestUpdateUserCommand(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	idty1 := identity.User{
		Name: "foobar1",
	}

	idty2 := identity.User{
		Name: "foobar2",
	}

	err = s.applyCommand(Command{
		Operation: OpAddIdentity,
		Data: CommandAddIdentity{
			Identity: idty1,
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.data.Users.Users))

	err = s.applyCommand(Command{
		Operation: OpAddIdentity,
		Data: CommandAddIdentity{
			Identity: idty2,
		},
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(s.data.Users.Users))

	err = s.applyCommand(Command{
		Operation: OpUpdateIdentity,
		Data: CommandUpdateIdentity{
			Name: "foobar1",
			Identity: identity.User{
				Name: "foobar3",
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(s.data.Users.Users))
}

func TestUpdateIdentity(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	idty1 := identity.User{
		Name: "foobar1",
	}

	idty2 := identity.User{
		Name: "foobar2",
	}

	err = s.addIdentity(CommandAddIdentity{
		Identity: idty1,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.data.Users.Users))

	err = s.addIdentity(CommandAddIdentity{
		Identity: idty2,
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(s.data.Users.Users))

	foobar := s.GetUser("foobar1").Users[0]
	require.True(t, foobar.CreatedAt.Equal(foobar.UpdatedAt))
	require.False(t, time.Time{}.Equal(foobar.CreatedAt))

	idty := identity.User{
		Name: "foobaz",
	}

	err = s.updateIdentity(CommandUpdateIdentity{
		Name:     "foobar",
		Identity: idty,
	})
	require.Error(t, err)
	require.Equal(t, 2, len(s.data.Users.Users))

	idty.Name = "foobar2"

	err = s.updateIdentity(CommandUpdateIdentity{
		Name:     "foobar1",
		Identity: idty,
	})
	require.Error(t, err)
	require.Equal(t, 2, len(s.data.Users.Users))

	idty.Name = "foobaz"

	err = s.updateIdentity(CommandUpdateIdentity{
		Name:     "foobar1",
		Identity: idty,
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(s.data.Users.Users))

	u := s.GetUser("foobar1")
	require.Empty(t, u.Users)

	u = s.GetUser("foobar2")
	require.NotEmpty(t, u.Users)

	u = s.GetUser("foobaz")
	require.NotEmpty(t, u.Users)

	require.True(t, u.Users[0].CreatedAt.Equal(foobar.CreatedAt))
	require.False(t, u.Users[0].UpdatedAt.Equal(foobar.UpdatedAt))
	require.False(t, time.Time{}.Equal(foobar.CreatedAt))
}

func TestUpdateIdentityWithAlias(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	idty1 := identity.User{
		Name:  "foobar1",
		Alias: "fooalias1",
	}

	idty2 := identity.User{
		Name:  "foobar2",
		Alias: "fooalias2",
	}

	err = s.addIdentity(CommandAddIdentity{
		Identity: idty1,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.data.Users.Users))

	err = s.addIdentity(CommandAddIdentity{
		Identity: idty2,
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(s.data.Users.Users))

	idty := identity.User{
		Name: "foobaz",
	}

	err = s.updateIdentity(CommandUpdateIdentity{
		Name:     "foobar",
		Identity: idty,
	})
	require.Error(t, err)
	require.Equal(t, 2, len(s.data.Users.Users))

	idty.Name = "foobar2"

	err = s.updateIdentity(CommandUpdateIdentity{
		Name:     "foobar1",
		Identity: idty,
	})
	require.Error(t, err)
	require.Equal(t, 2, len(s.data.Users.Users))

	idty.Name = "foobaz"
	idty.Alias = "fooalias2"

	err = s.updateIdentity(CommandUpdateIdentity{
		Name:     "foobar1",
		Identity: idty,
	})
	require.Error(t, err)
	require.Equal(t, 2, len(s.data.Users.Users))

	idty.Name = "foobaz"
	idty.Alias = "fooalias"

	err = s.updateIdentity(CommandUpdateIdentity{
		Name:     "fooalias1",
		Identity: idty,
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(s.data.Users.Users))

	u := s.GetUser("foobar1")
	require.Empty(t, u.Users)

	u = s.GetUser("fooalias1")
	require.Empty(t, u.Users)

	u = s.GetUser("foobar2")
	require.NotEmpty(t, u.Users)

	u = s.GetUser("fooalias2")
	require.NotEmpty(t, u.Users)

	u = s.GetUser("foobaz")
	require.NotEmpty(t, u.Users)

	u = s.GetUser("fooalias")
	require.NotEmpty(t, u.Users)
}

func TestUpdateIdentityWithPolicies(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	idty1 := identity.User{
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

	err = s.addIdentity(CommandAddIdentity{
		Identity: idty1,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.data.Users.Users))

	err = s.setPolicies(CommandSetPolicies{
		Name:     "foobar",
		Policies: policies,
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(s.data.Policies.Policies["foobar"]))

	err = s.updateIdentity(CommandUpdateIdentity{
		Name:     "foobar",
		Identity: idty1,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.data.Users.Users))
	require.Equal(t, 2, len(s.data.Policies.Policies["foobar"]))

	idty2 := identity.User{
		Name: "foobaz",
	}

	err = s.updateIdentity(CommandUpdateIdentity{
		Name:     "foobar",
		Identity: idty2,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(s.data.Users.Users))
	require.Equal(t, 0, len(s.data.Policies.Policies["foobar"]))
	require.Equal(t, 2, len(s.data.Policies.Policies["foobaz"]))
}
