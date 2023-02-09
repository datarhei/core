package jwt

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/datarhei/core/v16/app"
	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/handler/util"
	"github.com/datarhei/core/v16/iam"

	jwtgo "github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// The Config type holds information that is required to create a new JWT provider
type Config struct {
	Realm         string
	Secret        string
	SkipLocalhost bool
	IAM           iam.IAM
}

// JWT provides access to a JWT provider
type JWT interface {
	// Middleware returns an echo middleware
	Middleware() echo.MiddlewareFunc

	// LoginHandler is an echo route handler for retrieving a JWT
	LoginHandler(c echo.Context) error

	// RefreshHandle is an echo route handler for refreshing a JWT
	RefreshHandler(c echo.Context) error
}

type jwt struct {
	realm           string
	skipLocalhost   bool
	secret          []byte
	accessValidFor  time.Duration
	refreshValidFor time.Duration
	config          middleware.JWTConfig
	middleware      echo.MiddlewareFunc
	iam             iam.IAM
}

// New returns a new JWT provider
func New(config Config) (JWT, error) {
	j := &jwt{
		realm:           config.Realm,
		skipLocalhost:   config.SkipLocalhost,
		secret:          []byte(config.Secret),
		accessValidFor:  time.Minute * 10,
		refreshValidFor: time.Hour * 24,
	}

	if len(j.secret) == 0 {
		return nil, fmt.Errorf("the JWT secret must not be empty")
	}

	skipperFunc := func(c echo.Context) bool {
		if j.skipLocalhost {
			ip := c.RealIP()

			if ip == "127.0.0.1" || ip == "::1" {
				return true
			}
		}

		return false
	}

	j.config = middleware.JWTConfig{
		Skipper:                 skipperFunc,
		SigningMethod:           middleware.AlgorithmHS256,
		ContextKey:              "user",
		TokenLookup:             "header:" + echo.HeaderAuthorization,
		AuthScheme:              "Bearer",
		Claims:                  jwtgo.MapClaims{},
		ErrorHandlerWithContext: j.ErrorHandler,
		SuccessHandler: func(c echo.Context) {
			token := c.Get("user").(*jwtgo.Token)

			var subject string
			if claims, ok := token.Claims.(jwtgo.MapClaims); ok {
				if sub, ok := claims["sub"]; ok {
					subject = sub.(string)
				}
			}

			c.Set("user", subject)

			var usefor string
			if claims, ok := token.Claims.(jwtgo.MapClaims); ok {
				if sub, ok := claims["usefor"]; ok {
					usefor = sub.(string)
				}
			}

			c.Set("usefor", usefor)
		},
		ParseTokenFunc: j.parseToken,
	}

	return j, nil
}

func (j *jwt) parseToken(auth string, c echo.Context) (interface{}, error) {
	keyFunc := func(*jwtgo.Token) (interface{}, error) { return j.secret, nil }

	var token *jwtgo.Token
	var err error

	token, err = jwtgo.Parse(auth, keyFunc)
	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, errors.New("invalid token")
	}

	if _, ok := token.Claims.(jwtgo.MapClaims)["sub"]; !ok {
		return nil, fmt.Errorf("sub claim is required")
	}

	if _, ok := token.Claims.(jwtgo.MapClaims)["usefor"]; !ok {
		return nil, fmt.Errorf("usefor claim is required")
	}

	return token, nil
}

func (j *jwt) ErrorHandler(err error, c echo.Context) error {
	if c.Request().URL.Path == "/api" {
		return c.JSON(http.StatusOK, api.MinimalAbout{
			App:   app.Name,
			Auths: []string{},
			Version: api.VersionMinimal{
				Number: app.Version.MajorString(),
			},
		})
	}

	return api.Err(http.StatusUnauthorized, "Missing or invalid JWT token")
}

func (j *jwt) Middleware() echo.MiddlewareFunc {
	if j.middleware == nil {
		j.middleware = middleware.JWTWithConfig(j.config)
	}

	return j.middleware
}

// LoginHandler returns an access token and a refresh token
// @Summary Retrieve an access and a refresh token
// @Description Retrieve valid JWT access and refresh tokens to use for accessing the API. Login either by username/password or Auth0 token
// @ID jwt-login
// @Produce json
// @Param data body api.Login true "Login data"
// @Success 200 {object} api.JWT
// @Failure 400 {object} api.Error
// @Failure 403 {object} api.Error
// @Failure 500 {object} api.Error
// @Security Auth0KeyAuth
// @Router /api/login [post]
func (j *jwt) LoginHandler(c echo.Context) error {
	ok, subject, err := j.validateLogin(c)

	if ok {
		if err != nil {
			time.Sleep(5 * time.Second)
			return api.Err(http.StatusUnauthorized, "Invalid authorization credentials", "%s", err)
		}
	} else {
		time.Sleep(5 * time.Second)
		return api.Err(http.StatusBadRequest, "Missing authorization credentials")
	}

	at, rt, err := j.createToken(subject)
	if err != nil {
		return api.Err(http.StatusInternalServerError, "Failed to create JWT", "%s", err)
	}

	return c.JSON(http.StatusOK, api.JWT{
		AccessToken:  at,
		RefreshToken: rt,
	})
}

func (j *jwt) validateLogin(c echo.Context) (bool, string, error) {
	ok, subject, err := j.validateUserpassLogin(c)
	if ok {
		return ok, subject, err
	}

	return j.validateAuth0Login(c)
}

func (j *jwt) validateUserpassLogin(c echo.Context) (bool, string, error) {
	var login api.Login

	if err := util.ShouldBindJSON(c, &login); err != nil {
		return false, "", nil
	}

	identity, err := j.iam.GetIdentity(login.Username)
	if err != nil {
		return true, "", fmt.Errorf("invalid username or password")
	}

	if !identity.VerifyAPIPassword(login.Password) {
		return true, "", fmt.Errorf("invalid username or password")
	}

	return true, identity.Name(), nil
}

func (j *jwt) validateAuth0Login(c echo.Context) (bool, string, error) {
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
		return false, "", nil
	}

	p := &jwtgo.Parser{}
	token, _, err := p.ParseUnverified(auth, jwtgo.MapClaims{})
	if err != nil {
		return false, "", nil
	}

	var subject string
	if claims, ok := token.Claims.(jwtgo.MapClaims); ok {
		if sub, ok := claims["sub"]; ok {
			subject = sub.(string)
		}
	}

	identity, err := j.iam.GetIdentityByAuth0(subject)
	if err != nil {
		return true, "", fmt.Errorf("invalid token")
	}

	if !identity.VerifyAPIAuth0(auth) {
		return true, "", fmt.Errorf("invalid token")
	}

	return true, identity.Name(), nil
}

// RefreshHandler returns a new refresh token
// @Summary Retrieve a new access token
// @Description Retrieve a new access token by providing the refresh token
// @ID jwt-refresh
// @Produce json
// @Success 200 {object} api.JWTRefresh
// @Failure 500 {object} api.Error
// @Security ApiRefreshKeyAuth
// @Router /api/login/refresh [get]
func (j *jwt) RefreshHandler(c echo.Context) error {
	subject, ok := c.Get("user").(string)
	if !ok {
		return api.Err(http.StatusForbidden, "Invalid token")
	}

	usefor, ok := c.Get("usefor").(string)
	if !ok {
		return api.Err(http.StatusForbidden, "Invalid token")
	}

	if usefor != "refresh" {
		return api.Err(http.StatusForbidden, "Invalid token")
	}

	at, _, err := j.createToken(subject)
	if err != nil {
		return api.Err(http.StatusInternalServerError, "Failed to create JWT", "%s", err)
	}

	return c.JSON(http.StatusOK, api.JWTRefresh{
		AccessToken: at,
	})
}

// Already assigned claims: https://www.iana.org/assignments/jwt/jwt.xhtml

func (j *jwt) createToken(username string) (string, string, error) {
	now := time.Now()
	accessExpires := now.Add(j.accessValidFor)
	refreshExpires := now.Add(j.refreshValidFor)

	// Create access token
	accessToken := jwtgo.NewWithClaims(jwtgo.SigningMethodHS256, jwtgo.MapClaims{
		"iss":    j.realm,
		"sub":    username,
		"usefor": "access",
		"iat":    now.Unix(),
		"exp":    accessExpires.Unix(),
		"exi":    uint64(accessExpires.Sub(now).Seconds()),
		"jti":    uuid.New().String(),
	})

	// Generate encoded access token
	at, err := accessToken.SignedString(j.secret)
	if err != nil {
		return "", "", err
	}

	// Create refresh token
	refreshToken := jwtgo.NewWithClaims(jwtgo.SigningMethodHS256, jwtgo.MapClaims{
		"iss":    j.realm,
		"sub":    username,
		"usefor": "refresh",
		"iat":    now.Unix(),
		"exp":    refreshExpires.Unix(),
		"exi":    uint64(refreshExpires.Sub(now).Seconds()),
		"jti":    uuid.New().String(),
	})

	// Generate encoded refresh token
	rt, err := refreshToken.SignedString(j.secret)
	if err != nil {
		return "", "", err
	}

	return at, rt, nil
}
