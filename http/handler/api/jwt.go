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

// Login Creates a JWT from username/password
// @Summary Create a JWT
// @Description Create a JWT from username/password
// @ID login
// @Accept json
// @Produce json
// @Param user body api.Login true "User definition"
// @Success 200 {object} api.JWT
// @Failure 403 {object} api.Error
// @Router /api/login [post]
func (j *JWTHandler) Login(c echo.Context) error {
	subject, ok := c.Get("user").(string)
	if !ok {
		return api.Err(http.StatusForbidden, "Invalid user")
	}

	at, rt, err := j.iam.CreateJWT(subject)
	if err != nil {
		return api.Err(http.StatusForbidden, "", "failed to create JWT: %s", err)
	}

	return c.JSON(http.StatusOK, api.JWT{
		AccessToken:  at,
		RefreshToken: rt,
	})
}

// Refresh Refreshes a JWT from refresh JWT
// @Summary Refresh a JWT
// @Description Refresh a JWT
// @ID refresh
// @Accept json
// @Produce json
// @Success 200 {object} api.JWTRefresh
// @Failure 403 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/login/refresh [get]
func (j *JWTHandler) Refresh(c echo.Context) error {
	subject, ok := c.Get("user").(string)
	if !ok {
		return api.Err(http.StatusForbidden, "", "invalid token")
	}

	at, _, err := j.iam.CreateJWT(subject)
	if err != nil {
		return api.Err(http.StatusForbidden, "", "failed to create JWT: %s", err.Error())
	}

	return c.JSON(http.StatusOK, api.JWTRefresh{
		AccessToken: at,
	})
}
