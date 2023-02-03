package jwt

import (
	"fmt"
	"strings"

	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/handler/util"
	"github.com/datarhei/core/v16/iam"

	jwtgo "github.com/golang-jwt/jwt/v4"
	"github.com/labstack/echo/v4"
)

type Validator interface {
	String() string

	// Validate returns true if it identified itself as validator for
	// that request. False if it doesn't handle this request. The string
	// is the username. An error is only returned if it identified itself
	// as validator but there was an error during validation.
	Validate(c echo.Context) (bool, string, error)
	Cancel()
}

type localValidator struct {
	iam iam.IAM
}

func NewLocalValidator(iam iam.IAM) (Validator, error) {
	v := &localValidator{
		iam: iam,
	}

	return v, nil
}

func (v *localValidator) String() string {
	return "localjwt"
}

func (v *localValidator) Validate(c echo.Context) (bool, string, error) {
	var login api.Login

	if err := util.ShouldBindJSON(c, &login); err != nil {
		return false, "", nil
	}

	identity := v.iam.GetIdentity(login.Username)
	if identity == nil {
		return true, "", fmt.Errorf("invalid username or password")
	}

	if !identity.VerifyAPIPassword(login.Password) {
		return true, "", fmt.Errorf("invalid username or password")
	}

	return true, login.Username, nil
}

func (v *localValidator) Cancel() {}

type auth0Validator struct {
	iam iam.IAM
}

func NewAuth0Validator(iam iam.IAM) (Validator, error) {
	v := &auth0Validator{
		iam: iam,
	}

	return v, nil
}

func (v auth0Validator) String() string {
	return fmt.Sprintf("auth0 domain=%s audience=%s clientid=%s", "", "", "")
}

func (v *auth0Validator) Validate(c echo.Context) (bool, string, error) {
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

	identity := v.iam.GetIdentityByAuth0(subject)
	if identity == nil {
		return true, "", fmt.Errorf("invalid token")
	}

	if !identity.VerifyAPIAuth0(auth) {
		return true, "", fmt.Errorf("invalid token")
	}

	return true, subject, nil
}

func (v *auth0Validator) Cancel() {}
