// Package iam implements an identity and access management middleware
//
// Four information are required in order to decide to grant access.
// - identity
// - domain
// - resource
// - action
//
// The identity of the requester can be obtained by different means:
// - JWT
// - Username and password in the body as JSON
// - Auth0 access token
// - Basic auth
//
// The path prefix /api/login is treated specially in order to accommodate
// different ways of identification (UserPass, Auth0). All other /api paths
// only allow JWT as authentication method.
//
// If the identity can't be detected, the identity of "$anon" is given, representing
// an anonmyous user. If the Skipper function returns true for the request and the
// API is accessed, the username will be the one of the IAM superuser.
//
// The domain is provided as query parameter "group" for all API requests or the
// first path element after a mountpoint for filesystem requests.
//
// If the domain can't be detected, the domain "$none" will be used.
//
// The resource is the path of the request. For API requests it's prepended with
// the "api:" prefix. For all other requests it's prepended with the "fs:" prefix.
//
// The action is the requests HTTP method (e.g. GET, POST, ...).
package iam

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/handler/util"
	"github.com/datarhei/core/v16/iam"
	iamidentity "github.com/datarhei/core/v16/iam/identity"
	"github.com/datarhei/core/v16/log"

	jwtgo "github.com/golang-jwt/jwt/v4"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type Config struct {
	// Skipper defines a function to skip middleware.
	Skipper              middleware.Skipper
	Mounts               []string
	IAM                  iam.IAM
	WaitAfterFailedLogin bool
	Logger               log.Logger
}

var DefaultConfig = Config{
	Skipper:              middleware.DefaultSkipper,
	Mounts:               []string{},
	IAM:                  nil,
	WaitAfterFailedLogin: false,
	Logger:               nil,
}

var realm = "datarhei-core"

type iammiddleware struct {
	iam    iam.IAM
	mounts []string
	logger log.Logger
}

func New() echo.MiddlewareFunc {
	return NewWithConfig(DefaultConfig)
}

func NewWithConfig(config Config) echo.MiddlewareFunc {
	if config.Skipper == nil {
		config.Skipper = DefaultConfig.Skipper
	}

	if len(config.Mounts) == 0 {
		config.Mounts = append(config.Mounts, "/")
	}

	if config.Logger == nil {
		config.Logger = log.New("")
	}

	mw := iammiddleware{
		iam:    config.IAM,
		mounts: config.Mounts,
		logger: config.Logger,
	}

	// Sort the mounts from longest to shortest
	sort.Slice(mw.mounts, func(i, j int) bool {
		return len(mw.mounts[i]) > len(mw.mounts[j])
	})

	mw.logger.Debug().WithField("mounts", mw.mounts).Log("")

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if config.IAM == nil {
				return api.Err(http.StatusForbidden, "Forbidden", "IAM is not provided")
			}

			isAPISuperuser := false
			if config.Skipper(c) {
				isAPISuperuser = true
			}

			var identity iamidentity.Verifier = nil
			var err error

			username := "$anon"
			resource := c.Request().URL.Path
			var domain string

			if resource == "/ping" || resource == "/profiling" {
				return next(c)
			}

			if resource == "/api" || strings.HasPrefix(resource, "/api/") {
				if resource == "/api/login" {
					identity, err = mw.findIdentityFromUserpass(c)
					if err != nil {
						if config.WaitAfterFailedLogin {
							time.Sleep(5 * time.Second)
						}
						return api.Err(http.StatusForbidden, "Forbidden", "%s", err)
					}

					if identity == nil {
						identity, err = mw.findIdentityFromAuth0(c)
						if err != nil {
							if config.WaitAfterFailedLogin {
								time.Sleep(5 * time.Second)
							}
							return api.Err(http.StatusForbidden, "Forbidden", "%s", err)
						}
					}
				} else {
					identity, err = mw.findIdentityFromJWT(c)
					if err != nil {
						return api.Err(http.StatusForbidden, "Forbidden", "%s", err)
					}

					if identity != nil {
						if resource == "/api/login/refresh" {
							usefor, _ := c.Get("usefor").(string)
							if usefor != "refresh" {
								if config.WaitAfterFailedLogin {
									time.Sleep(5 * time.Second)
								}
								return api.Err(http.StatusForbidden, "Forbidden", "invalid token")
							}
						} else {
							usefor, _ := c.Get("usefor").(string)
							if usefor != "access" {
								if config.WaitAfterFailedLogin {
									time.Sleep(5 * time.Second)
								}
								return api.Err(http.StatusForbidden, "Forbidden", "invalid token")
							}
						}
					}
				}

				if isAPISuperuser {
					username = config.IAM.GetDefaultVerifier().Name()
				}

				domain = c.QueryParam("group")
				resource = "api:" + resource
			} else {
				identity, err = mw.findIdentityFromBasicAuth(c)
				if err != nil {
					if err == ErrAuthRequired {
						c.Response().Header().Set(echo.HeaderWWWAuthenticate, "Basic realm="+realm)
						return api.Err(http.StatusUnauthorized, "Unauthorized", "%s", err)
					} else {
						if config.WaitAfterFailedLogin {
							time.Sleep(5 * time.Second)
						}

						if err == ErrBadRequest {
							return api.Err(http.StatusBadRequest, "Bad request", "%s", err)
						} else if err == ErrUnauthorized {
							c.Response().Header().Set(echo.HeaderWWWAuthenticate, "Basic realm="+realm)
							return api.Err(http.StatusUnauthorized, "Unauthorized", "%s", err)
						} else {
							return api.Err(http.StatusForbidden, "Forbidden", "%s", err)
						}
					}
				}

				domain = mw.findDomainFromFilesystem(resource)
				resource = "fs:" + resource
			}

			superuser := false

			if identity != nil {
				username = identity.Name()
				superuser = identity.IsSuperuser()
			}

			c.Set("user", username)
			c.Set("superuser", superuser)

			if len(domain) == 0 {
				domain = "$none"
			}

			action := c.Request().Method

			if !config.IAM.Enforce(username, domain, resource, action) {
				return api.Err(http.StatusForbidden, "Forbidden", "access denied")
			}

			return next(c)
		}
	}
}

var ErrAuthRequired = errors.New("unauthorized")
var ErrUnauthorized = errors.New("unauthorized")
var ErrBadRequest = errors.New("bad request")

func (m *iammiddleware) findIdentityFromBasicAuth(c echo.Context) (iamidentity.Verifier, error) {
	basic := "basic"
	auth := c.Request().Header.Get(echo.HeaderAuthorization)
	l := len(basic)

	if len(auth) == 0 {
		path := c.Request().URL.Path
		domain := m.findDomainFromFilesystem(path)
		if len(domain) == 0 {
			domain = "$none"
		}

		if !m.iam.Enforce("$anon", domain, "fs:"+path, c.Request().Method) {
			return nil, ErrAuthRequired
		}

		return nil, nil
	}

	var username string
	var password string

	if len(auth) > l+1 && strings.EqualFold(auth[:l], basic) {
		// Invalid base64 shouldn't be treated as error
		// instead should be treated as invalid client input
		b, err := base64.StdEncoding.DecodeString(auth[l+1:])
		if err != nil {
			return nil, ErrBadRequest
		}

		cred := string(b)
		for i := 0; i < len(cred); i++ {
			if cred[i] == ':' {
				username, password = cred[:i], cred[i+1:]
				break
			}
		}
	}

	identity, err := m.iam.GetVerifier(username)
	if err != nil {
		m.logger.Debug().WithFields(log.Fields{
			"path":   c.Request().URL.Path,
			"method": c.Request().Method,
		}).WithError(err).Log("identity not found")
		return nil, ErrUnauthorized
	}

	if ok, err := identity.VerifyServiceBasicAuth(password); !ok {
		m.logger.Debug().WithFields(log.Fields{
			"path":   c.Request().URL.Path,
			"method": c.Request().Method,
		}).WithError(err).Log("wrong password")
		return nil, ErrUnauthorized
	}

	return identity, nil
}

func (m *iammiddleware) findIdentityFromJWT(c echo.Context) (iamidentity.Verifier, error) {
	// Look for an Auth header
	values := c.Request().Header.Values("Authorization")
	prefix := "Bearer "

	auth := ""
	for _, value := range values {
		if !strings.HasPrefix(value, prefix) {
			continue
		}

		auth = value[len(prefix):]

		break
	}

	if len(auth) == 0 {
		return nil, nil
	}

	p := &jwtgo.Parser{}
	token, _, err := p.ParseUnverified(auth, jwtgo.MapClaims{})
	if err != nil {
		m.logger.Debug().WithFields(log.Fields{
			"path":   c.Request().URL.Path,
			"method": c.Request().Method,
		}).WithError(err).Log("identity not found")
		return nil, err
	}

	var subject string
	if claims, ok := token.Claims.(jwtgo.MapClaims); ok {
		if sub, ok := claims["sub"]; ok {
			subject = sub.(string)
		}
	}

	var usefor string
	if claims, ok := token.Claims.(jwtgo.MapClaims); ok {
		if sub, ok := claims["usefor"]; ok {
			usefor = sub.(string)
		}
	}

	identity, err := m.iam.GetVerifier(subject)
	if err != nil {
		m.logger.Debug().WithFields(log.Fields{
			"path":   c.Request().URL.Path,
			"method": c.Request().Method,
		}).WithError(err).Log("identity not found")
		return nil, fmt.Errorf("invalid token")
	}

	if ok, err := identity.VerifyJWT(auth); !ok {
		m.logger.Debug().WithFields(log.Fields{
			"path":   c.Request().URL.Path,
			"method": c.Request().Method,
		}).WithError(err).Log("identity not found")
		return nil, fmt.Errorf("invalid token")
	}

	c.Set("usefor", usefor)

	return identity, nil
}

func (m *iammiddleware) findIdentityFromUserpass(c echo.Context) (iamidentity.Verifier, error) {
	var login api.Login

	if err := util.ShouldBindJSON(c, &login); err != nil {
		return nil, nil
	}

	identity, err := m.iam.GetVerifier(login.Username)
	if err != nil {
		m.logger.Debug().WithFields(log.Fields{
			"path":   c.Request().URL.Path,
			"method": c.Request().Method,
		}).WithError(err).Log("identity not found")
		return nil, fmt.Errorf("invalid username or password")
	}

	if ok, err := identity.VerifyAPIPassword(login.Password); !ok {
		m.logger.Debug().WithFields(log.Fields{
			"path":   c.Request().URL.Path,
			"method": c.Request().Method,
		}).WithError(err).Log("identity not found")
		return nil, fmt.Errorf("invalid username or password")
	}

	return identity, nil
}

func (m *iammiddleware) findIdentityFromAuth0(c echo.Context) (iamidentity.Verifier, error) {
	// Look for an Auth header
	values := c.Request().Header.Values("Authorization")
	prefix := "Bearer "

	auth := ""
	for _, value := range values {
		if !strings.HasPrefix(value, prefix) {
			continue
		}

		auth = value[len(prefix):]

		break
	}

	if len(auth) == 0 {
		return nil, nil
	}

	p := &jwtgo.Parser{}
	token, _, err := p.ParseUnverified(auth, jwtgo.MapClaims{})
	if err != nil {
		m.logger.Debug().WithFields(log.Fields{
			"path":   c.Request().URL.Path,
			"method": c.Request().Method,
		}).WithError(err).Log("identity not found")
		return nil, nil
	}

	var subject string
	if claims, ok := token.Claims.(jwtgo.MapClaims); ok {
		if sub, ok := claims["sub"]; ok {
			subject = sub.(string)
		}
	}

	identity, err := m.iam.GetVerifierFromAuth0(subject)
	if err != nil {
		m.logger.Debug().WithFields(log.Fields{
			"path":   c.Request().URL.Path,
			"method": c.Request().Method,
		}).WithError(err).Log("identity not found")
		return nil, fmt.Errorf("invalid token")
	}

	if ok, err := identity.VerifyAPIAuth0(auth); !ok {
		m.logger.Debug().WithFields(log.Fields{
			"path":   c.Request().URL.Path,
			"method": c.Request().Method,
		}).WithError(err).Log("identity not found")
		return nil, fmt.Errorf("invalid token")
	}

	return identity, nil
}

func (m *iammiddleware) findDomainFromFilesystem(path string) string {
	path = filepath.Clean(path)

	// Longest prefix search. The slice is assumed to be sorted accordingly.
	// Assume path is /memfs/foobar/file.txt
	// The longest prefix that matches is /memfs/
	// Remove it from the path and split it into components: foobar file.txt
	// Check if foobar a known domain. If yes, return it. If not, return empty domain.
	for _, mount := range m.mounts {
		prefix := filepath.Clean(mount)
		if prefix != "/" {
			prefix += "/"
		}

		if strings.HasPrefix(path, prefix) {
			elements := strings.Split(strings.TrimPrefix(path, prefix), "/")
			if m.iam.HasDomain(elements[0]) {
				return elements[0]
			}
		}
	}

	return ""
}
