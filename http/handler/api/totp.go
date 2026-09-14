package api

import (
	"net/http"

	"github.com/datarhei/core/v16/auth/totp"
	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/handler/util"

	"github.com/labstack/echo/v4"
)

// TOTPHandler provides handler functions for TOTP enrollment.
type TOTPHandler struct {
	store    *totp.Store
	username string
}

// NewTOTP returns a new TOTP handler.
func NewTOTP(store *totp.Store, username string) *TOTPHandler {
	return &TOTPHandler{
		store:    store,
		username: username,
	}
}

// Status returns the current TOTP enrollment state.
// @Summary TOTP enrollment status
// @Description Returns whether TOTP is enabled or pending setup
// @Tags v16.16.0
// @ID totp-status
// @Produce json
// @Success 200 {object} api.TOTPStatus
// @Security ApiKeyAuth
// @Router /api/v3/auth/totp [get]
func (p *TOTPHandler) Status(c echo.Context) error {
	status := p.store.Status()

	return c.JSON(http.StatusOK, api.TOTPStatus(status))
}

// Setup starts TOTP enrollment.
// @Summary Start TOTP enrollment
// @Description Generate a TOTP secret and otpauth URI for the authenticator app
// @Tags v16.16.0
// @ID totp-setup
// @Produce json
// @Success 200 {object} api.TOTPSetup
// @Failure 409 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/auth/totp/setup [post]
func (p *TOTPHandler) Setup(c echo.Context) error {
	setup, err := p.store.Setup(p.username)
	if err != nil {
		return api.Err(http.StatusConflict, err.Error())
	}

	return c.JSON(http.StatusOK, api.TOTPSetup(setup))
}

// Enable confirms TOTP enrollment with a valid code.
// @Summary Enable TOTP
// @Description Confirm TOTP enrollment with a code from the authenticator app
// @Tags v16.16.0
// @ID totp-enable
// @Accept json
// @Produce json
// @Param data body api.TOTPEnable true "TOTP confirmation code"
// @Success 200 {object} api.TOTPStatus
// @Failure 400 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/auth/totp/enable [post]
func (p *TOTPHandler) Enable(c echo.Context) error {
	var req api.TOTPEnable

	if err := util.ShouldBindJSON(c, &req); err != nil {
		return api.Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
	}

	if err := p.store.Enable(req.Code); err != nil {
		return api.Err(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, api.TOTPStatus(p.store.Status()))
}

// Disable removes TOTP enrollment.
// @Summary Disable TOTP
// @Description Disable TOTP after confirming with a valid authenticator code
// @Tags v16.16.0
// @ID totp-disable
// @Accept json
// @Produce json
// @Param data body api.TOTPDisable true "TOTP confirmation code"
// @Success 200 {object} api.TOTPStatus
// @Failure 400 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/auth/totp [delete]
func (p *TOTPHandler) Disable(c echo.Context) error {
	var req api.TOTPDisable

	if err := util.ShouldBindJSON(c, &req); err != nil {
		return api.Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
	}

	if err := p.store.Disable(req.Code); err != nil {
		return api.Err(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, api.TOTPStatus(p.store.Status()))
}
