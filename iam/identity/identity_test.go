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

func TestIdentity(t *testing.T) {
	user := User{
		Name: "foobar",
	}

	identity := user.marshalIdentity()

	require.Equal(t, "foobar", identity.Name())

	identity.user.Alias = "raboof"
	require.Equal(t, "raboof", identity.Name())

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

func TestDefaultIdentity(t *testing.T) {
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

	identity, err := im.GetDefaultVerifier()
	require.NoError(t, err)
	require.NotNil(t, identity)
	require.Equal(t, "foobar", identity.Name())
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

	password := identity.GetServiceBasicAuth()
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

func TestJWT(t *testing.T) {
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

	access, refresh, err := im.CreateJWT("foobaz")
	require.Error(t, err)
	require.Equal(t, "", access)
	require.Equal(t, "", refresh)

	access, refresh, err = im.CreateJWT("foobar")
	require.NoError(t, err)

	identity, err := im.GetVerifier("foobar")
	require.NoError(t, err)
	require.NotNil(t, identity)

	ok, err := identity.VerifyJWT("something")
	require.Error(t, err)
	require.False(t, ok)

	ok, err = identity.VerifyJWT(access)
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = identity.VerifyJWT(refresh)
	require.NoError(t, err)
	require.True(t, ok)

	err = im.Create(User{Name: "foobaz"})
	require.NoError(t, err)

	access, refresh, err = im.CreateJWT("foobaz")
	require.NoError(t, err)

	ok, err = identity.VerifyJWT(access)
	require.Error(t, err)
	require.False(t, ok)

	ok, err = identity.VerifyJWT(refresh)
	require.Error(t, err)
	require.False(t, ok)
}

func TestCreateUser(t *testing.T) {
	adapter, err := createAdapter()
	require.NoError(t, err)

	im, err := New(Config{
		Adapter:   adapter,
		Superuser: User{Name: "foobar", Alias: "foobalias"},
		JWTRealm:  "test-realm",
		JWTSecret: "abc123",
		Logger:    nil,
	})
	require.NoError(t, err)
	require.NotNil(t, im)

	err = im.Create(User{Name: "foobar"})
	require.Error(t, err)

	err = im.Create(User{Name: "foobaz", Alias: "foobalias"})
	require.Error(t, err)

	err = im.Create(User{Name: "foobaz", Alias: "alias"})
	require.NoError(t, err)

	err = im.Create(User{Name: "fooboz", Alias: "alias"})
	require.Error(t, err)

	err = im.Create(User{Name: "foobaz", Alias: "somealias"})
	require.Error(t, err)
}

func TestAlias(t *testing.T) {
	adapter, err := createAdapter()
	require.NoError(t, err)

	im, err := New(Config{
		Adapter:   adapter,
		Superuser: User{Name: "foobar", Alias: "foobalias"},
		JWTRealm:  "test-realm",
		JWTSecret: "abc123",
		Logger:    nil,
	})
	require.NoError(t, err)
	require.NotNil(t, im)

	err = im.Create(User{Name: "foobaz", Alias: "alias"})
	require.NoError(t, err)

	identity, err := im.Get("foobar")
	require.NoError(t, err)
	require.Equal(t, "foobar", identity.Name)
	require.Equal(t, "foobalias", identity.Alias)

	identity, err = im.Get("foobalias")
	require.NoError(t, err)
	require.Equal(t, "foobar", identity.Name)
	require.Equal(t, "foobalias", identity.Alias)

	identity, err = im.Get("foobaz")
	require.NoError(t, err)
	require.Equal(t, "foobaz", identity.Name)
	require.Equal(t, "alias", identity.Alias)

	identity, err = im.Get("alias")
	require.NoError(t, err)
	require.Equal(t, "foobaz", identity.Name)
	require.Equal(t, "alias", identity.Alias)
}

func TestCreateUserAuth0(t *testing.T) {
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

	require.ElementsMatch(t, []string{"localjwt"}, im.Validators())

	err = im.Create(User{
		Name:      "foobaz",
		Superuser: false,
		Auth: UserAuth{
			API: UserAuthAPI{
				Auth0: UserAuthAPIAuth0{
					User: "auth0|123456",
					Tenant: Auth0Tenant{
						Domain:   "example.com",
						Audience: "https://api.example.com/",
						ClientID: "123456",
					},
				},
			},
		},
	})
	require.Error(t, err)

	err = im.Create(User{
		Name:      "foobaz",
		Superuser: false,
		Auth: UserAuth{
			API: UserAuthAPI{
				Auth0: UserAuthAPIAuth0{
					User: "auth0|123456",
					Tenant: Auth0Tenant{
						Domain:   "datarhei-demo.eu.auth0.com",
						Audience: "https://datarhei-demo.eu.auth0.com/api/v2/",
						ClientID: "123456",
					},
				},
			},
		},
	})
	require.NoError(t, err)

	identity, err := im.GetVerifierFromAuth0("foobaz")
	require.Error(t, err)
	require.Nil(t, identity)

	identity, err = im.GetVerifierFromAuth0("auth0|123456")
	require.NoError(t, err)
	require.NotNil(t, identity)

	manager, ok := im.(*identityManager)
	require.True(t, ok)
	require.NotNil(t, manager)

	require.Equal(t, 1, len(manager.tenants))
	require.Equal(t, map[string]string{"auth0|123456": "foobaz"}, manager.auth0UserIdentityMap)

	require.ElementsMatch(t, []string{
		"localjwt",
		"auth0 domain=datarhei-demo.eu.auth0.com audience=https://datarhei-demo.eu.auth0.com/api/v2/ clientid=123456",
	}, im.Validators())

	err = im.Create(User{
		Name:      "fooboz",
		Superuser: false,
		Auth: UserAuth{
			API: UserAuthAPI{
				Auth0: UserAuthAPIAuth0{
					User: "auth0|123456",
					Tenant: Auth0Tenant{
						Domain:   "datarhei-demo.eu.auth0.com",
						Audience: "https://datarhei-demo.eu.auth0.com/api/v2/",
						ClientID: "123456",
					},
				},
			},
		},
	})
	require.Error(t, err)

	err = im.Create(User{
		Name:      "fooboz",
		Superuser: false,
		Auth: UserAuth{
			API: UserAuthAPI{
				Auth0: UserAuthAPIAuth0{
					User: "auth0|987654",
					Tenant: Auth0Tenant{
						Domain:   "datarhei-demo.eu.auth0.com",
						Audience: "https://datarhei-demo.eu.auth0.com/api/v2/",
						ClientID: "987654",
					},
				},
			},
		},
	})
	require.NoError(t, err)

	require.Equal(t, 1, len(manager.tenants))
	require.Equal(t, map[string]string{"auth0|123456": "foobaz", "auth0|987654": "fooboz"}, manager.auth0UserIdentityMap)

	require.ElementsMatch(t, []string{
		"localjwt",
		"auth0 domain=datarhei-demo.eu.auth0.com audience=https://datarhei-demo.eu.auth0.com/api/v2/ clientid=123456",
		"auth0 domain=datarhei-demo.eu.auth0.com audience=https://datarhei-demo.eu.auth0.com/api/v2/ clientid=987654",
	}, im.Validators())

	im.Close()
}

func TestLoadAndSave(t *testing.T) {
	adptr, err := createAdapter()
	require.NoError(t, err)

	dummyfs := adptr.(*fileAdapter).fs

	im, err := New(Config{
		Adapter:   adptr,
		Superuser: User{Name: "foobar"},
		JWTRealm:  "test-realm",
		JWTSecret: "abc123",
		Logger:    nil,
	})
	require.NoError(t, err)
	require.NotNil(t, im)

	err = im.Save()
	require.NoError(t, err)

	_, err = dummyfs.Stat("./users.json")
	require.NoError(t, err)

	data, err := dummyfs.ReadFile("./users.json")
	require.NoError(t, err)
	require.Equal(t, []byte("[]"), data)

	err = im.Create(User{Name: "foobaz", Alias: "alias"})
	require.NoError(t, err)

	identity, err := im.GetVerifier("foobaz")
	require.NoError(t, err)
	require.NotNil(t, identity)

	identity, err = im.GetVerifier("alias")
	require.NoError(t, err)
	require.NotNil(t, identity)

	err = im.Save()
	require.NoError(t, err)

	im, err = New(Config{
		Adapter:   adptr,
		Superuser: User{Name: "foobar"},
		JWTRealm:  "test-realm",
		JWTSecret: "abc123",
		Logger:    nil,
	})
	require.NoError(t, err)
	require.NotNil(t, im)

	identity, err = im.GetVerifier("foobaz")
	require.NoError(t, err)
	require.NotNil(t, identity)

	identity, err = im.GetVerifier("alias")
	require.NoError(t, err)
	require.NotNil(t, identity)
}

func TestUpdateUser(t *testing.T) {
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

	err = im.Create(User{Name: "fooboz"})
	require.NoError(t, err)

	err = im.Update("unknown", User{Name: "fooboz"})
	require.Error(t, err)

	err = im.Update("foobar", User{Name: "foobar"})
	require.Error(t, err)

	err = im.Update("foobar", User{Name: "fooboz"})
	require.Error(t, err)

	identity, err := im.GetVerifier("foobar")
	require.NoError(t, err)
	require.NotNil(t, identity)
	require.Equal(t, "foobar", identity.Name())

	err = im.Update("foobar", User{Name: "foobaz"})
	require.Error(t, err)
	require.Equal(t, "foobar", identity.Name())

	identity, err = im.GetVerifier("foobaz")
	require.Error(t, err)
	require.Nil(t, identity)

	identity, err = im.GetVerifier("fooboz")
	require.NoError(t, err)
	require.NotNil(t, identity)
	require.Equal(t, "fooboz", identity.Name())

	err = im.Update("fooboz", User{Name: "foobaz"})
	require.NoError(t, err)
}

func TestUpdateUserAlias(t *testing.T) {
	adapter, err := createAdapter()
	require.NoError(t, err)

	im, err := New(Config{
		Adapter:   adapter,
		Superuser: User{Name: "foobar", Alias: "superalias"},
		JWTRealm:  "test-realm",
		JWTSecret: "abc123",
		Logger:    nil,
	})
	require.NoError(t, err)
	require.NotNil(t, im)

	err = im.Create(User{Name: "fooboz"})
	require.NoError(t, err)

	identity, err := im.GetVerifier("fooboz")
	require.NoError(t, err)
	require.NotNil(t, identity)
	require.Equal(t, "fooboz", identity.Name())

	_, err = im.GetVerifier("alias")
	require.Error(t, err)

	err = im.Update("fooboz", User{Name: "fooboz", Alias: "alias"})
	require.NoError(t, err)

	identity, err = im.GetVerifier("fooboz")
	require.NoError(t, err)
	require.NotNil(t, identity)
	require.Equal(t, "alias", identity.Name())

	err = im.Create(User{Name: "barfoo", Alias: "alias2"})
	require.NoError(t, err)

	err = im.Update("fooboz", User{Name: "fooboz", Alias: "alias2"})
	require.Error(t, err)

	err = im.Update("fooboz", User{Name: "barfoo", Alias: "alias"})
	require.Error(t, err)

	err = im.Update("fooboz", User{Name: "fooboz", Alias: "superalias"})
	require.Error(t, err)

	err = im.Update("fooboz", User{Name: "foobar", Alias: ""})
	require.Error(t, err)
}

func TestUpdateUserAuth0(t *testing.T) {
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

	err = im.Create(User{
		Name:      "foobaz",
		Superuser: false,
		Auth: UserAuth{
			API: UserAuthAPI{
				Auth0: UserAuthAPIAuth0{
					User: "auth0|123456",
					Tenant: Auth0Tenant{
						Domain:   "datarhei-demo.eu.auth0.com",
						Audience: "https://datarhei-demo.eu.auth0.com/api/v2/",
						ClientID: "123456",
					},
				},
			},
		},
	})
	require.NoError(t, err)

	identity, err := im.GetVerifierFromAuth0("auth0|123456")
	require.NoError(t, err)
	require.NotNil(t, identity)

	identity, err = im.GetVerifier("foobaz")
	require.NoError(t, err)
	require.NotNil(t, identity)

	user, err := im.Get("foobaz")
	require.NoError(t, err)

	user.Name = "fooboz"

	err = im.Update("foobaz", user)
	require.NoError(t, err)

	identity, err = im.GetVerifierFromAuth0("auth0|123456")
	require.NoError(t, err)
	require.NotNil(t, identity)

	identity, err = im.GetVerifier("fooboz")
	require.NoError(t, err)
	require.NotNil(t, identity)
}

func TestRemoveUser(t *testing.T) {
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

	err = im.Delete("fooboz")
	require.Error(t, err)

	err = im.Delete("foobar")
	require.Error(t, err)

	err = im.Create(User{
		Name:      "foobaz",
		Superuser: false,
		Auth: UserAuth{
			API: UserAuthAPI{
				Password: "apisecret",
				Auth0:    UserAuthAPIAuth0{},
			},
			Services: UserAuthServices{
				Basic:   []string{"secret"},
				Token:   []string{"tokensecret"},
				Session: []string{"sessionsecret"},
			},
		},
	})
	require.NoError(t, err)

	identity, err := im.GetVerifier("foobaz")
	require.NoError(t, err)
	require.NotNil(t, identity)

	ok, err := identity.VerifyAPIPassword("apisecret")
	require.True(t, ok)
	require.NoError(t, err)

	ok, err = identity.VerifyServiceBasicAuth("secret")
	require.True(t, ok)
	require.NoError(t, err)

	ok, err = identity.VerifyServiceToken("tokensecret")
	require.True(t, ok)
	require.NoError(t, err)

	session := identity.GetServiceSession(nil, time.Hour)

	ok, data, err := identity.VerifyServiceSession(session)
	require.True(t, ok)
	require.Equal(t, nil, data)
	require.NoError(t, err)

	access, refresh, err := im.CreateJWT("foobaz")
	require.NoError(t, err)

	ok, err = identity.VerifyJWT(access)
	require.True(t, ok)
	require.NoError(t, err)

	ok, err = identity.VerifyJWT(refresh)
	require.True(t, ok)
	require.NoError(t, err)

	err = im.Delete("foobaz")
	require.NoError(t, err)

	ok, err = identity.VerifyAPIPassword("apisecret")
	require.False(t, ok)
	require.Error(t, err)

	ok, err = identity.VerifyServiceBasicAuth("secret")
	require.False(t, ok)
	require.Error(t, err)

	ok, err = identity.VerifyServiceToken("tokensecret")
	require.False(t, ok)
	require.Error(t, err)

	ok, data, err = identity.VerifyServiceSession(session)
	require.False(t, ok)
	require.Equal(t, nil, data)
	require.Error(t, err)

	ok, err = identity.VerifyJWT(access)
	require.False(t, ok)
	require.Error(t, err)

	ok, err = identity.VerifyJWT(refresh)
	require.False(t, ok)
	require.Error(t, err)

	identity, err = im.GetVerifier("foobaz")
	require.Error(t, err)
	require.Nil(t, identity)

	access, refresh, err = im.CreateJWT("foobaz")
	require.Error(t, err)
	require.Empty(t, access)
	require.Empty(t, refresh)
}

func TestRemoveUserAuth0(t *testing.T) {
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

	err = im.Create(User{
		Name:      "foobaz",
		Superuser: false,
		Auth: UserAuth{
			API: UserAuthAPI{
				Auth0: UserAuthAPIAuth0{
					User: "auth0|123456",
					Tenant: Auth0Tenant{
						Domain:   "datarhei-demo.eu.auth0.com",
						Audience: "https://datarhei-demo.eu.auth0.com/api/v2/",
						ClientID: "123456",
					},
				},
			},
		},
	})
	require.NoError(t, err)

	err = im.Create(User{
		Name:      "fooboz",
		Superuser: false,
		Auth: UserAuth{
			API: UserAuthAPI{
				Auth0: UserAuthAPIAuth0{
					User: "auth0|987654",
					Tenant: Auth0Tenant{
						Domain:   "datarhei-demo.eu.auth0.com",
						Audience: "https://datarhei-demo.eu.auth0.com/api/v2/",
						ClientID: "987654",
					},
				},
			},
		},
	})
	require.NoError(t, err)

	manager, ok := im.(*identityManager)
	require.True(t, ok)
	require.NotNil(t, manager)

	require.Equal(t, 1, len(manager.tenants))
	require.Equal(t, map[string]string{"auth0|123456": "foobaz", "auth0|987654": "fooboz"}, manager.auth0UserIdentityMap)

	require.ElementsMatch(t, []string{
		"localjwt",
		"auth0 domain=datarhei-demo.eu.auth0.com audience=https://datarhei-demo.eu.auth0.com/api/v2/ clientid=123456",
		"auth0 domain=datarhei-demo.eu.auth0.com audience=https://datarhei-demo.eu.auth0.com/api/v2/ clientid=987654",
	}, im.Validators())

	err = im.Delete("foobaz")
	require.NoError(t, err)

	require.Equal(t, 1, len(manager.tenants))
	require.Equal(t, map[string]string{"auth0|987654": "fooboz"}, manager.auth0UserIdentityMap)

	require.ElementsMatch(t, []string{
		"localjwt",
		"auth0 domain=datarhei-demo.eu.auth0.com audience=https://datarhei-demo.eu.auth0.com/api/v2/ clientid=987654",
	}, im.Validators())

	err = im.Delete("fooboz")
	require.NoError(t, err)

	require.Equal(t, 0, len(manager.tenants))
	require.ElementsMatch(t, []string{
		"localjwt",
	}, im.Validators())
}

func TestAutosave(t *testing.T) {
	adptr, err := createAdapter()
	require.NoError(t, err)

	dummyfs := adptr.(*fileAdapter).fs

	im, err := New(Config{
		Adapter:   adptr,
		Superuser: User{Name: "foobar"},
		JWTRealm:  "test-realm",
		JWTSecret: "abc123",
		Logger:    nil,
	})
	require.NoError(t, err)
	require.NotNil(t, im)

	err = im.Save()
	require.NoError(t, err)

	_, err = dummyfs.Stat("./users.json")
	require.NoError(t, err)

	data, err := dummyfs.ReadFile("./users.json")
	require.NoError(t, err)
	require.Equal(t, []byte("[]"), data)

	err = im.Create(User{Name: "foobaz"})
	require.NoError(t, err)

	data, err = dummyfs.ReadFile("./users.json")
	require.NoError(t, err)
	require.NotEqual(t, []byte("[]"), data)

	user, err := im.Get("foobaz")
	require.NoError(t, err)

	user.Name = "fooboz"

	err = im.Update("foobaz", user)
	require.NoError(t, err)

	data, err = dummyfs.ReadFile("./users.json")
	require.NoError(t, err)
	require.NotEqual(t, []byte("[]"), data)

	err = im.Delete("fooboz")
	require.NoError(t, err)

	data, err = dummyfs.ReadFile("./users.json")
	require.NoError(t, err)
	require.Equal(t, []byte("[]"), data)
}
