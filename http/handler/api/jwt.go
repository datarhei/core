package api

import (
	"net/http"

	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/iam"

	"github.com/labstack/echo/v4"
)

type JWTHandler struct {
	iam iam.IAM
}

func NewJWT(iam iam.IAM) *JWTHandler {
	return &JWTHandler{
		iam: iam,
	}
}

func (j *JWTHandler) Login(c echo.Context) error {
	subject, ok := c.Get("user").(string)
	if !ok {
		return api.Err(http.StatusForbidden, "Invalid user")
	}

	at, rt, err := j.iam.CreateJWT(subject)
	if err != nil {
		return api.Err(http.StatusForbidden, "Failed to create JWT", "%s", err)
	}

	return c.JSON(http.StatusOK, api.JWT{
		AccessToken:  at,
		RefreshToken: rt,
	})
}

func (j *JWTHandler) Refresh(c echo.Context) error {
	subject, ok := c.Get("user").(string)
	if !ok {
		return api.Err(http.StatusForbidden, "Invalid token")
	}

	at, _, err := j.iam.CreateJWT(subject)
	if err != nil {
		return api.Err(http.StatusForbidden, "Failed to create JWT", "%s", err)
	}

	return c.JSON(http.StatusOK, api.JWTRefresh{
		AccessToken: at,
	})
}
