package identity

import (
	"testing"
	"time"

	"github.com/datarhei/core/v16/io/fs"
	"github.com/stretchr/testify/require"
)

func createAdapter() (Adapter, error) {
	dummyfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	if err != nil {
		return nil, err
	}

	return NewJSONAdapter(dummyfs, "./users.json", nil)
}

func TestIdentity(t *testing.T) {
	user := User{
		Name: "foobar",
	}

	identity := user.marshalIdentity()

	require.Equal(t, "foobar", identity.Name())

	identity.user.Alias = "raboof"
	require.Equal(t, "raboof", identity.Alias())

	require.False(t, identity.isValid())
	identity.valid = true
	require.True(t, identity.isValid())

	require.False(t, identity.IsSuperuser())
	identity.user.Superuser = true
	require.True(t, identity.IsSuperuser())

	adapter, err := createAdapter()
	require.NoError(t, err)

	im, err := New(Config{
		Adapter:   adapter,
		Superuser: User{Name: "foobar"},
		JWTRealm:  "test-realm",
		JWTSecret: "abc123",
		Logger:    nil,
	})
	require.NoError(t, err)
	require.NotNil(t, im)

	id, err := im.GetVerifier("unknown")
	require.Error(t, err)
	require.Nil(t, id)
}

func TestIdentityAPIAuth(t *testing.T) {
	user := User{
		Name: "foobar",
	}

	identity := user.marshalIdentity()

	ok, err := identity.VerifyAPIPassword("secret")
	require.False(t, ok)
	require.Error(t, err)

	identity.user.Auth.API.Password = "secret"

	ok, err = identity.VerifyAPIPassword("secret")
	require.False(t, ok)
	require.Error(t, err)

	identity.valid = true

	ok, err = identity.VerifyAPIPassword("secret")
	require.True(t, ok)
	require.NoError(t, err)

	identity.user.Auth.API.Password = ""

	ok, err = identity.VerifyAPIPassword("secret")
	require.False(t, ok)
	require.Error(t, err)

	identity.user.Auth.API.Password = "terces"

	ok, err = identity.VerifyAPIPassword("secret")
	require.False(t, ok)
	require.NoError(t, err)
}

func TestIdentityServiceBasicAuth(t *testing.T) {
	user := User{
		Name: "foobar",
	}

	identity := user.marshalIdentity()

	ok, err := identity.VerifyServiceBasicAuth("secret")
	require.False(t, ok)
	require.Error(t, err)

	identity.user.Auth.Services.Basic = append(identity.user.Auth.Services.Basic, "secret")

	ok, err = identity.VerifyServiceBasicAuth("secret")
	require.False(t, ok)
	require.Error(t, err)

	identity.valid = true

	ok, err = identity.VerifyServiceBasicAuth("secret")
	require.True(t, ok)
	require.NoError(t, err)

	identity.user.Auth.Services.Basic[0] = ""

	ok, err = identity.VerifyServiceBasicAuth("secret")
	require.False(t, ok)
	require.NoError(t, err)

	identity.user.Auth.Services.Basic[0] = "terces"

	ok, err = identity.VerifyServiceBasicAuth("secret")
	require.False(t, ok)
	require.NoError(t, err)

	userinfo := identity.GetServiceBasicAuth()
	password, _ := userinfo.Password()
	require.Equal(t, "terces", password)
}

func TestIdentityServiceTokenAuth(t *testing.T) {
	user := User{
		Name: "foobar",
	}

	identity := user.marshalIdentity()

	ok, err := identity.VerifyServiceToken("secret")
	require.False(t, ok)
	require.Error(t, err)

	identity.user.Auth.Services.Token = []string{"secret"}

	ok, err = identity.VerifyServiceToken("secret")
	require.False(t, ok)
	require.Error(t, err)

	identity.valid = true

	ok, err = identity.VerifyServiceToken("secret")
	require.True(t, ok)
	require.NoError(t, err)

	identity.user.Auth.Services.Token = []string{"terces"}

	ok, err = identity.VerifyServiceToken("secret")
	require.False(t, ok)
	require.NoError(t, err)

	token := identity.GetServiceToken()
	require.Equal(t, "foobar:terces", token)
}

func TestIdentityServiceSessionAuth(t *testing.T) {
	user := User{
		Name: "foobar",
	}

	identity := user.marshalIdentity()

	session := identity.GetServiceSession(nil, time.Hour)
	require.Empty(t, session)

	identity.user.Auth.Services.Session = []string{"bla"}

	session = identity.GetServiceSession(nil, time.Hour)
	require.Empty(t, session)

	identity.valid = true

	session = identity.GetServiceSession(nil, time.Hour)
	require.NotEmpty(t, session)

	ok, data, err := identity.VerifyServiceSession(session)
	require.True(t, ok)
	require.Equal(t, nil, data)
	require.NoError(t, err)
}
