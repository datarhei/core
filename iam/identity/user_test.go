package identity

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUserName(t *testing.T) {
	user := User{}

	err := user.Validate()
	require.Error(t, err)

	user.Name = "foobar_5"
	err = user.Validate()
	require.NoError(t, err)

	user.Name = "foobar:5"
	err = user.Validate()
	require.NoError(t, err)

	user.Name = "$foob:ar"
	err = user.Validate()
	require.Error(t, err)
}

func TestUserAlias(t *testing.T) {
	user := User{
		Name: "foober",
	}

	err := user.Validate()
	require.NoError(t, err)

	user.Alias = "foobar"
	err = user.Validate()
	require.NoError(t, err)

	user.Alias = "$foob:ar"
	err = user.Validate()
	require.Error(t, err)
}

func TestUserListAdd(t *testing.T) {
	l := NewUserList()

	_, err := l.Get("foobar")
	require.Error(t, err)

	err = l.Add(User{Name: "foobar"})
	require.NoError(t, err)

	_, err = l.Get("foobar")
	require.NoError(t, err)

	err = l.Add(User{Name: "foobaz", Alias: "foobar"})
	require.Error(t, err)

	err = l.Add(User{Name: "foobaz", Alias: "foobaz"})
	require.NoError(t, err)

	err = l.Add(User{Name: "barfoo", Alias: "foobaz"})
	require.Error(t, err)
}

func TestUserListDelete(t *testing.T) {
	l := NewUserList()

	_, err := l.Get("foobar")
	require.Error(t, err)

	err = l.Add(User{Name: "foobar"})
	require.NoError(t, err)

	_, err = l.Get("foobar")
	require.NoError(t, err)

	l.Delete("foobar")
	require.NoError(t, err)

	_, err = l.Get("foobar")
	require.Error(t, err)

	err = l.Add(User{Name: "foobar", Alias: "foobaz"})
	require.NoError(t, err)

	_, err = l.Get("foobaz")
	require.NoError(t, err)

	l.Delete("foobaz")
	require.NoError(t, err)

	_, err = l.Get("foobaz")
	require.Error(t, err)
}

func TestUserListUpdate(t *testing.T) {
	l := NewUserList()

	err := l.Add(User{Name: "foobar"})
	require.NoError(t, err)

	err = l.Update("foobaz", User{Name: "foobar"})
	require.Error(t, err)

	err = l.Update("foobar", User{Name: "foobaz", Alias: "fooboz"})
	require.NoError(t, err)

	_, err = l.Get("foobar")
	require.Error(t, err)

	_, err = l.Get("foobaz")
	require.NoError(t, err)

	_, err = l.Get("fooboz")
	require.NoError(t, err)

	err = l.Add(User{Name: "foobar"})
	require.NoError(t, err)

	err = l.Update("foobaz", User{Name: "foobar"})
	require.Error(t, err)

	err = l.Update("fooboz", User{Name: "fooboz"})
	require.NoError(t, err)

	_, err = l.Get("foobaz")
	require.Error(t, err)
}

func TestUserListList(t *testing.T) {
	l := NewUserList()

	err := l.Add(User{Name: "foobar", Alias: "foobaz"})
	require.NoError(t, err)

	users := l.List()
	require.Equal(t, 1, len(users))

	err = l.Add(User{Name: "barfoo", Alias: "bazfoo"})
	require.NoError(t, err)

	users = l.List()
	require.Equal(t, 2, len(users))
}
