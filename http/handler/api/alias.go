package api

import (
	"errors"
	"net/http"

	"github.com/datarhei/core/v16/alias"
	"github.com/datarhei/core/v16/http/api"
	apierrors "github.com/datarhei/core/v16/http/errors"
	"github.com/datarhei/core/v16/http/handler/util"

	"github.com/labstack/echo/v4"
)

// The AliasHandler type provides handler functions for managing aliases.
type AliasHandler struct {
	aliases alias.Manager
}

// NewAlias returns a new AliasHandler type. You have to provide an alias manager.
func NewAlias(aliases alias.Manager) *AliasHandler {
	return &AliasHandler{
		aliases: aliases,
	}
}

// List returns all aliases
// @Summary List all aliases
// @Description List all aliases in the order they are evaluated. The first matching alias is applied.
// @Tags v16.?.?
// @ID alias-3-list
// @Produce json
// @Success 200 {array} api.Alias
// @Security ApiKeyAuth
// @Router /api/v3/alias [get]
func (h *AliasHandler) List(c echo.Context) error {
	rules := h.aliases.List()

	aliases := make([]api.Alias, 0, len(rules))
	for _, r := range rules {
		a := api.Alias{}
		a.Marshal(r)
		aliases = append(aliases, a)
	}

	return c.JSON(http.StatusOK, aliases)
}

// Set replaces all aliases
// @Summary Replace all aliases
// @Description Replace all aliases. The aliases are evaluated in the given order. Only superusers are allowed to modify aliases.
// @Tags v16.?.?
// @ID alias-3-set
// @Accept json
// @Produce json
// @Param aliases body []api.Alias true "List of aliases"
// @Success 200 {array} api.Alias
// @Failure 400 {object} apierrors.Error
// @Failure 403 {object} apierrors.Error
// @Failure 409 {object} apierrors.Error
// @Failure 500 {object} apierrors.Error
// @Security ApiKeyAuth
// @Router /api/v3/alias [put]
func (h *AliasHandler) Set(c echo.Context) error {
	if err := checkSuperuser(c); err != nil {
		return err
	}

	aliases := []api.Alias{}

	// The validator only supports structs, the elements are validated individually.
	if err := util.ShouldBindJSONValidation(c, &aliases, false); err != nil {
		return apierrors.Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	rules := make([]alias.Rule, 0, len(aliases))
	for i, a := range aliases {
		if err := c.Validate(&a); err != nil {
			return apierrors.Err(http.StatusBadRequest, "", "invalid JSON: alias %d: %s", i, err.Error())
		}

		rules = append(rules, a.Unmarshal())
	}

	if err := h.aliases.Set(rules); err != nil {
		return aliasError(err)
	}

	return c.JSON(http.StatusOK, aliases)
}

// Add adds an alias
// @Summary Add an alias
// @Description Add an alias. The alias is appended to the list of aliases. Only superusers are allowed to modify aliases.
// @Tags v16.?.?
// @ID alias-3-add
// @Accept json
// @Produce json
// @Param alias body api.Alias true "Alias"
// @Success 200 {object} api.Alias
// @Failure 400 {object} apierrors.Error
// @Failure 403 {object} apierrors.Error
// @Failure 409 {object} apierrors.Error
// @Failure 500 {object} apierrors.Error
// @Security ApiKeyAuth
// @Router /api/v3/alias [post]
func (h *AliasHandler) Add(c echo.Context) error {
	if err := checkSuperuser(c); err != nil {
		return err
	}

	a := api.Alias{}

	if err := util.ShouldBindJSON(c, &a); err != nil {
		return apierrors.Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	if err := h.aliases.Add(a.Unmarshal()); err != nil {
		return aliasError(err)
	}

	return c.JSON(http.StatusOK, a)
}

// Get returns an alias
// @Summary Get an alias
// @Description Get the alias with the given pattern
// @Tags v16.?.?
// @ID alias-3-get
// @Produce json
// @Param pattern query string true "Pattern of the alias"
// @Success 200 {object} api.Alias
// @Failure 400 {object} apierrors.Error
// @Failure 404 {object} apierrors.Error
// @Security ApiKeyAuth
// @Router /api/v3/alias/rule [get]
func (h *AliasHandler) Get(c echo.Context) error {
	pattern := util.DefaultQuery(c, "pattern", "")
	if len(pattern) == 0 {
		return apierrors.Err(http.StatusBadRequest, "", "the pattern is missing")
	}

	r, err := h.aliases.Get(pattern)
	if err != nil {
		return aliasError(err)
	}

	a := api.Alias{}
	a.Marshal(r)

	return c.JSON(http.StatusOK, a)
}

// Update replaces an alias
// @Summary Replace an alias
// @Description Replace the alias with the given pattern. The position of the alias is kept. Only superusers are allowed to modify aliases.
// @Tags v16.?.?
// @ID alias-3-update
// @Accept json
// @Produce json
// @Param pattern query string true "Pattern of the alias"
// @Param alias body api.Alias true "Alias"
// @Success 200 {object} api.Alias
// @Failure 400 {object} apierrors.Error
// @Failure 403 {object} apierrors.Error
// @Failure 404 {object} apierrors.Error
// @Failure 409 {object} apierrors.Error
// @Failure 500 {object} apierrors.Error
// @Security ApiKeyAuth
// @Router /api/v3/alias/rule [put]
func (h *AliasHandler) Update(c echo.Context) error {
	if err := checkSuperuser(c); err != nil {
		return err
	}

	pattern := util.DefaultQuery(c, "pattern", "")
	if len(pattern) == 0 {
		return apierrors.Err(http.StatusBadRequest, "", "the pattern is missing")
	}

	a := api.Alias{}

	if err := util.ShouldBindJSON(c, &a); err != nil {
		return apierrors.Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	if err := h.aliases.Update(pattern, a.Unmarshal()); err != nil {
		return aliasError(err)
	}

	return c.JSON(http.StatusOK, a)
}

// Delete removes an alias
// @Summary Delete an alias
// @Description Delete the alias with the given pattern. Only superusers are allowed to modify aliases.
// @Tags v16.?.?
// @ID alias-3-delete
// @Produce json
// @Param pattern query string true "Pattern of the alias"
// @Success 200 {string} string
// @Failure 400 {object} apierrors.Error
// @Failure 403 {object} apierrors.Error
// @Failure 404 {object} apierrors.Error
// @Failure 500 {object} apierrors.Error
// @Security ApiKeyAuth
// @Router /api/v3/alias/rule [delete]
func (h *AliasHandler) Delete(c echo.Context) error {
	if err := checkSuperuser(c); err != nil {
		return err
	}

	pattern := util.DefaultQuery(c, "pattern", "")
	if len(pattern) == 0 {
		return apierrors.Err(http.StatusBadRequest, "", "the pattern is missing")
	}

	if err := h.aliases.Remove(pattern); err != nil {
		return aliasError(err)
	}

	return c.JSON(http.StatusOK, "OK")
}

func checkSuperuser(c echo.Context) error {
	if !util.DefaultContext(c, "superuser", false) {
		return apierrors.Err(http.StatusForbidden, "", "only superusers are allowed to modify aliases")
	}

	return nil
}

func aliasError(err error) error {
	switch {
	case errors.Is(err, alias.ErrInvalid), errors.Is(err, alias.ErrReadOnly):
		return apierrors.Err(http.StatusBadRequest, "", "%s", err.Error())
	case errors.Is(err, alias.ErrExists):
		return apierrors.Err(http.StatusConflict, "", "%s", err.Error())
	case errors.Is(err, alias.ErrNotFound):
		return apierrors.Err(http.StatusNotFound, "", "%s", err.Error())
	}

	return apierrors.Err(http.StatusInternalServerError, "", "%s", err.Error())
}
