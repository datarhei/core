package iam

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/handler/util"
	"github.com/datarhei/core/v16/iam"
	"github.com/datarhei/core/v16/log"

	jwtgo "github.com/golang-jwt/jwt/v4"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type Config struct {
	// Skipper defines a function to skip middleware.
	Skipper middleware.Skipper
	IAM     iam.IAM
	Logger  log.Logger
}

var DefaultConfig = Config{
	Skipper: middleware.DefaultSkipper,
	IAM:     nil,
	Logger:  nil,
}

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

	mw := iammiddleware{
		iam:    config.IAM,
		mounts: []string{"/", "/memfs"},
		logger: config.Logger,
	}

	// Sort the mounts from longest to shortest
	sort.Slice(mw.mounts, func(i, j int) bool {
		if len(mw.mounts[i]) > len(mw.mounts[j]) {
			return true
		}

		return false
	})

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if config.Skipper(c) {
				c.Set("user", "$anon")
				return next(c)
			}

			if config.IAM == nil {
				return api.Err(http.StatusForbidden, "Forbidden", "IAM is not provided")
			}

			var identity iam.IdentityVerifier = nil
			var err error

			resource := c.Request().URL.Path
			var domain string

			if resource == "/api" || strings.HasPrefix(resource, "/api/") {
				if resource == "/api/login" {
					identity, err = mw.findIdentityFromUserpass(c)
					if err != nil {
						time.Sleep(5 * time.Second)
						return api.Err(http.StatusForbidden, "Forbidden", "%s", err)
					}

					if identity == nil {
						identity, err = mw.findIdentityFromAuth0(c)
						if err != nil {
							time.Sleep(5 * time.Second)
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
								time.Sleep(5 * time.Second)
								return api.Err(http.StatusForbidden, "Forbidden", "invalid token")
							}
						} else {
							usefor, _ := c.Get("usefor").(string)
							if usefor != "access" {
								time.Sleep(5 * time.Second)
								return api.Err(http.StatusForbidden, "Forbidden", "invalid token")
							}
						}
					}
				}

				domain = c.QueryParam("group")
				resource = "api:" + resource
			} else {
				identity, err = mw.findIdentityFromBasicAuth(c)
				if err != nil {
					return api.Err(http.StatusForbidden, "Bad request", "%s", err)
				}

				domain = mw.findDomainFromFilesystem(resource)
				resource = "fs:" + resource
			}

			username := "$anon"
			if identity != nil {
				username = identity.Name()
			}

			c.Set("user", username)

			if identity != nil && identity.IsSuperuser() {
				username = "$superuser"
			}

			if len(domain) == 0 {
				domain = "$none"
			}

			action := c.Request().Method

			l := mw.logger.Debug().WithFields(log.Fields{
				"subject":  username,
				"domain":   domain,
				"resource": resource,
				"action":   action,
			})

			if ok, rule := config.IAM.Enforce(username, domain, resource, action); !ok {
				l.Log("access denied")
				return api.Err(http.StatusForbidden, "Forbidden", "access denied")
			} else {
				l.Log(rule)
			}

			return next(c)
		}
	}
}

func (m *iammiddleware) findIdentityFromBasicAuth(c echo.Context) (iam.IdentityVerifier, error) {
	basic := "basic"
	auth := c.Request().Header.Get(echo.HeaderAuthorization)
	l := len(basic)

	if len(auth) == 0 {
		return nil, nil
	}

	var username string
	var password string

	if len(auth) > l+1 && strings.EqualFold(auth[:l], basic) {
		// Invalid base64 shouldn't be treated as error
		// instead should be treated as invalid client input
		b, err := base64.StdEncoding.DecodeString(auth[l+1:])
		if err != nil {
			return nil, err
		}

		cred := string(b)
		for i := 0; i < len(cred); i++ {
			if cred[i] == ':' {
				username, password = cred[:i], cred[i+1:]
				break
			}
		}
	}

	identity, err := m.iam.GetIdentity(username)
	if err != nil {
		m.logger.Debug().WithFields(log.Fields{
			"path":   c.Request().URL.Path,
			"method": c.Request().Method,
		}).WithError(err).Log("identity not found")
		return nil, fmt.Errorf("invalid username or password")
	}

	if ok, err := identity.VerifyAPIPassword(password); !ok {
		m.logger.Debug().WithFields(log.Fields{
			"path":   c.Request().URL.Path,
			"method": c.Request().Method,
		}).WithError(err).Log("wrong password")
		return nil, fmt.Errorf("invalid username or password")
	}

	return identity, nil
}

func (m *iammiddleware) findIdentityFromJWT(c echo.Context) (iam.IdentityVerifier, error) {
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

	identity, err := m.iam.GetIdentity(subject)
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

func (m *iammiddleware) findIdentityFromUserpass(c echo.Context) (iam.IdentityVerifier, error) {
	var login api.Login

	if err := util.ShouldBindJSON(c, &login); err != nil {
		return nil, nil
	}

	identity, err := m.iam.GetIdentity(login.Username)
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

func (m *iammiddleware) findIdentityFromAuth0(c echo.Context) (iam.IdentityVerifier, error) {
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

	identity, err := m.iam.GetIdentityByAuth0(subject)
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
		prefix := filepath.Join(mount, "/")
		if strings.HasPrefix(path, prefix) {
			elements := strings.Split(strings.TrimPrefix(path, prefix), "/")
			if m.iam.IsDomain(elements[0]) {
				return elements[0]
			}
		}
	}

	return ""
}
