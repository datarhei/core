package iam

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/http/api"
	apihandler "github.com/datarhei/core/v16/http/handler/api"
	"github.com/datarhei/core/v16/http/validator"
	"github.com/datarhei/core/v16/iam"
	iamidentity "github.com/datarhei/core/v16/iam/identity"
	"github.com/datarhei/core/v16/iam/policy"
	"github.com/datarhei/core/v16/io/fs"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

var basic string = "Basic"

func getIAM() (iam.IAM, error) {
	dummyfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	if err != nil {
		return nil, err
	}

	policyAdapter, err := policy.NewJSONAdapter(dummyfs, "./policy.json", nil)
	if err != nil {
		return nil, err
	}

	identityAdapter, err := iamidentity.NewJSONAdapter(dummyfs, "./users.json", nil)
	if err != nil {
		return nil, err
	}

	i, err := iam.New(iam.Config{
		PolicyAdapter:   policyAdapter,
		IdentityAdapter: identityAdapter,
		Superuser: iamidentity.User{
			Name: "admin",
		},
		JWTRealm:  "datarhei-core",
		JWTSecret: "1234567890",
		Logger:    nil,
	})
	if err != nil {
		return nil, err
	}

	i.CreateIdentity(iamidentity.User{
		Name: "foobar",
		Auth: iamidentity.UserAuth{
			API: iamidentity.UserAuthAPI{
				Password: "secret",
			},
			Services: iamidentity.UserAuthServices{
				Basic: []string{"secret"},
			},
		},
	})

	i.CreateIdentity(iamidentity.User{
		Name: "foobar:2",
		Auth: iamidentity.UserAuth{
			API: iamidentity.UserAuthAPI{
				Password: "secret",
			},
			Services: iamidentity.UserAuthServices{
				Basic: []string{"secret"},
			},
		},
	})

	return i, nil
}

func TestNoIAM(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()
	c := e.NewContext(req, res)
	h := New()(func(c echo.Context) error {
		return c.String(http.StatusOK, "test")
	})

	err := h(c)
	require.Error(t, err)

	he := err.(api.Error)
	require.Equal(t, http.StatusForbidden, he.Code)
}

func TestBasicAuth(t *testing.T) {
	iam, err := getIAM()
	require.NoError(t, err)

	iam.AddPolicy("foobar", "$none", []string{"fs"}, "/**", []string{"ANY"})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()
	c := e.NewContext(req, res)
	h := NewWithConfig(Config{
		IAM: iam,
	})(func(c echo.Context) error {
		return c.String(http.StatusOK, "test")
	})

	// No credentials
	err = h(c)
	require.Error(t, err)

	he := err.(api.Error)
	require.Equal(t, http.StatusUnauthorized, he.Code)
	require.Equal(t, basic+` realm=datarhei-core`, res.Header().Get(echo.HeaderWWWAuthenticate))

	// Valid credentials
	auth := basic + " " + base64.StdEncoding.EncodeToString([]byte("foobar:secret"))
	req.Header.Set(echo.HeaderAuthorization, auth)
	require.NoError(t, h(c))

	// Case-insensitive header scheme
	auth = strings.ToUpper(basic) + " " + base64.StdEncoding.EncodeToString([]byte("foobar:secret"))
	req.Header.Set(echo.HeaderAuthorization, auth)
	require.NoError(t, h(c))

	// Invalid credentials
	auth = basic + " " + base64.StdEncoding.EncodeToString([]byte("foobar:invalid-password"))
	req.Header.Set(echo.HeaderAuthorization, auth)
	he = h(c).(api.Error)
	require.Equal(t, http.StatusUnauthorized, he.Code)
	require.Equal(t, basic+` realm=datarhei-core`, res.Header().Get(echo.HeaderWWWAuthenticate))

	// Invalid base64 string
	auth = basic + " invalidString"
	req.Header.Set(echo.HeaderAuthorization, auth)
	he = h(c).(api.Error)
	require.Equal(t, http.StatusBadRequest, he.Code)

	// Missing Authorization header
	req.Header.Del(echo.HeaderAuthorization)
	he = h(c).(api.Error)
	require.Equal(t, http.StatusUnauthorized, he.Code)

	// Invalid Authorization header
	auth = base64.StdEncoding.EncodeToString([]byte("invalid"))
	req.Header.Set(echo.HeaderAuthorization, auth)
	he = h(c).(api.Error)
	require.Equal(t, http.StatusUnauthorized, he.Code)
}

func TestBasicAuthEncoding(t *testing.T) {
	iam, err := getIAM()
	require.NoError(t, err)

	iam.AddPolicy("foobar:2", "$none", []string{"fs"}, "/**", []string{"ANY"})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()
	c := e.NewContext(req, res)
	h := NewWithConfig(Config{
		IAM: iam,
	})(func(c echo.Context) error {
		return c.String(http.StatusOK, "test")
	})

	// Valid credentials
	auth := basic + " " + base64.StdEncoding.EncodeToString([]byte("foobar%3A2:secret"))
	req.Header.Set(echo.HeaderAuthorization, auth)
	require.NoError(t, h(c))

	// Invalid encoded credentials
	auth = basic + " " + base64.StdEncoding.EncodeToString([]byte("foobar%32:secret"))
	req.Header.Set(echo.HeaderAuthorization, auth)
	he := h(c).(api.Error)
	require.Equal(t, http.StatusUnauthorized, he.Code)
	require.Equal(t, basic+` realm=datarhei-core`, res.Header().Get(echo.HeaderWWWAuthenticate))

	// Invalid credentials
	auth = basic + " " + base64.StdEncoding.EncodeToString([]byte("foobar:2:secret"))
	req.Header.Set(echo.HeaderAuthorization, auth)
	he = h(c).(api.Error)
	require.Equal(t, http.StatusUnauthorized, he.Code)
	require.Equal(t, basic+` realm=datarhei-core`, res.Header().Get(echo.HeaderWWWAuthenticate))
}

func TestFindDomainFromFilesystem(t *testing.T) {
	iam, err := getIAM()
	require.NoError(t, err)

	iam.AddPolicy("$anon", "$none", []string{"fs"}, "/**", []string{"ANY"})
	iam.AddPolicy("foobar", "group", []string{"fs"}, "/group/**", []string{"ANY"})
	iam.AddPolicy("foobar", "anothergroup", []string{"fs"}, "/memfs/anothergroup/**", []string{"ANY"})

	mw := &iammiddleware{
		iam:    iam,
		mounts: []string{"/", "/memfs"},
	}

	domain := mw.findDomainFromFilesystem("/")
	require.Equal(t, "", domain)

	domain = mw.findDomainFromFilesystem("/group/bla")
	require.Equal(t, "group", domain)

	domain = mw.findDomainFromFilesystem("/anothergroup/bla")
	require.Equal(t, "anothergroup", domain)

	domain = mw.findDomainFromFilesystem("/memfs/anothergroup/bla")
	require.Equal(t, "anothergroup", domain)
}

func TestBasicAuthDomain(t *testing.T) {
	iam, err := getIAM()
	require.NoError(t, err)

	iam.AddPolicy("$anon", "$none", []string{"fs"}, "/**", []string{"ANY"})
	iam.AddPolicy("foobar", "group", []string{"fs"}, "/group/**", []string{"ANY"})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()
	c := e.NewContext(req, res)
	h := NewWithConfig(Config{
		IAM:    iam,
		Mounts: []string{"/"},
	})(func(c echo.Context) error {
		return c.String(http.StatusOK, "test")
	})

	// No credentials
	require.NoError(t, h(c))

	req = httptest.NewRequest(http.MethodGet, "/group/bla", nil)
	c = e.NewContext(req, res)

	err = h(c)
	require.Error(t, err)

	he := err.(api.Error)
	require.Equal(t, http.StatusUnauthorized, he.Code)
	require.Equal(t, basic+` realm=datarhei-core`, res.Header().Get(echo.HeaderWWWAuthenticate))

	// Valid credentials
	auth := basic + " " + base64.StdEncoding.EncodeToString([]byte("foobar:secret"))
	req.Header.Set(echo.HeaderAuthorization, auth)
	require.NoError(t, h(c))

	// Allow anonymous group read access
	iam.AddPolicy("$anon", "group", []string{"fs"}, "/group/**", []string{"GET"})

	req.Header.Del(echo.HeaderAuthorization)
	require.NoError(t, h(c))
}

func TestAPILoginAndRefresh(t *testing.T) {
	iam, err := getIAM()
	require.NoError(t, err)

	iam.AddPolicy("foobar", "$none", []string{"api"}, "/**", []string{"ANY"})

	jwthandler := apihandler.NewJWT(iam)

	e := echo.New()
	e.Validator = validator.New()
	res := httptest.NewRecorder()
	h := NewWithConfig(Config{
		IAM:    iam,
		Mounts: []string{"/"},
	})(func(c echo.Context) error {
		if c.Request().Method == http.MethodPost {
			if c.Request().URL.Path == "/api/login" {
				return jwthandler.Login(c)
			}
		}

		if c.Request().Method == http.MethodGet {
			if c.Request().URL.Path == "/api/login/refresh" {
				return jwthandler.Refresh(c)
			}
		}

		return c.String(http.StatusOK, "test")
	})

	req := httptest.NewRequest(http.MethodPost, "/api/login", nil)
	c := e.NewContext(req, res)

	// No credentials
	err = h(c)
	require.Error(t, err)

	he := err.(api.Error)
	require.Equal(t, http.StatusForbidden, he.Code)

	// Wrong password
	login := api.Login{
		Username: "foobar",
		Password: "nosecret",
	}

	data, err := json.Marshal(login)
	require.NoError(t, err)

	req = httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(data))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	c = e.NewContext(req, res)

	err = h(c)
	require.Error(t, err)

	he = err.(api.Error)
	require.Equal(t, http.StatusForbidden, he.Code)

	// Wrong username
	login = api.Login{
		Username: "foobaz",
		Password: "secret",
	}

	data, err = json.Marshal(login)
	require.NoError(t, err)

	req = httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(data))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	c = e.NewContext(req, res)

	err = h(c)
	require.Error(t, err)

	he = err.(api.Error)
	require.Equal(t, http.StatusForbidden, he.Code)

	// Correct credentials
	login = api.Login{
		Username: "foobar",
		Password: "secret",
	}

	data, err = json.Marshal(login)
	require.NoError(t, err)

	req = httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(data))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	c = e.NewContext(req, res)
	res.Body.Reset()

	err = h(c)
	require.NoError(t, err)

	data, err = io.ReadAll(res.Body)
	require.NoError(t, err)

	jwt := api.JWT{}
	err = json.Unmarshal(data, &jwt)
	require.NoError(t, err)

	// No JWT
	req = httptest.NewRequest(http.MethodGet, "/api/some/endpoint", nil)
	c = e.NewContext(req, res)

	err = h(c)
	require.Error(t, err)

	he = err.(api.Error)
	require.Equal(t, http.StatusForbidden, he.Code)

	// With invalid JWT
	req.Header.Set(echo.HeaderAuthorization, "Bearer invalid")
	err = h(c)
	require.Error(t, err)

	// With refresh JWT
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+jwt.RefreshToken)
	err = h(c)
	require.Error(t, err)

	// With access JWT
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+jwt.AccessToken)
	err = h(c)
	require.NoError(t, err)

	// Refresh JWT
	req = httptest.NewRequest(http.MethodGet, "/api/login/refresh", nil)
	c = e.NewContext(req, res)

	err = h(c)
	require.Error(t, err)

	he = err.(api.Error)
	require.Equal(t, http.StatusForbidden, he.Code)

	req.Header.Set(echo.HeaderAuthorization, "Bearer "+jwt.AccessToken)
	err = h(c)
	require.Error(t, err)

	he = err.(api.Error)
	require.Equal(t, http.StatusForbidden, he.Code)

	req.Header.Set(echo.HeaderAuthorization, "Bearer "+jwt.RefreshToken)
	res.Body.Reset()
	err = h(c)
	require.NoError(t, err)

	data, err = io.ReadAll(res.Body)
	require.NoError(t, err)

	jwtrefresh := api.JWTRefresh{}
	err = json.Unmarshal(data, &jwtrefresh)
	require.NoError(t, err)

	req = httptest.NewRequest(http.MethodGet, "/api/some/endpoint", nil)
	c = e.NewContext(req, res)

	// With new access JWT
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+jwtrefresh.AccessToken)
	err = h(c)
	require.NoError(t, err)
}
