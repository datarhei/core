package api

import (
	"errors"
	"net/http"

	"github.com/datarhei/core/v16/alias"
	"github.com/datarhei/core/v16/cluster/store"
	"github.com/datarhei/core/v16/http/api"
	apierrors "github.com/datarhei/core/v16/http/errors"
	"github.com/datarhei/core/v16/http/handler/util"

	"github.com/labstack/echo/v4"
)

// AliasList returns all aliases from the cluster DB
// @Summary List all aliases in the cluster
// @Description List all aliases in the cluster in the order they are evaluated. The first matching alias is applied.
// @Tags v16.?.?
// @ID cluster-3-alias-list
// @Produce json
// @Success 200 {array} api.Alias
// @Security ApiKeyAuth
// @Router /api/v3/cluster/alias [get]
func (h *ClusterHandler) AliasList(c echo.Context) error {
	rules := h.cluster.Store().AliasList().Rules

	aliases := make([]api.Alias, 0, len(rules))
	for _, r := range rules {
		a := api.Alias{}
		a.Marshal(r)
		aliases = append(aliases, a)
	}

	return c.JSON(http.StatusOK, aliases)
}

// AliasesSet replaces all aliases in the cluster
// @Summary Replace all aliases in the cluster
// @Description Replace all aliases in the cluster. The aliases are evaluated in the given order. Only superusers are allowed to modify aliases.
// @Tags v16.?.?
// @ID cluster-3-alias-set
// @Accept json
// @Produce json
// @Param aliases body []api.Alias true "List of aliases"
// @Success 200 {array} api.Alias
// @Failure 400 {object} apierrors.Error
// @Failure 403 {object} apierrors.Error
// @Failure 409 {object} apierrors.Error
// @Failure 500 {object} apierrors.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/alias [put]
func (h *ClusterHandler) AliasesSet(c echo.Context) error {
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

	if err := h.cluster.AliasesSet("", rules); err != nil {
		return clusterAliasError(err)
	}

	return c.JSON(http.StatusOK, aliases)
}

// AliasAdd adds an alias to the cluster
// @Summary Add an alias to the cluster
// @Description Add an alias to the cluster. The alias is appended to the list of aliases. Only superusers are allowed to modify aliases.
// @Tags v16.?.?
// @ID cluster-3-alias-add
// @Accept json
// @Produce json
// @Param alias body api.Alias true "Alias"
// @Success 200 {object} api.Alias
// @Failure 400 {object} apierrors.Error
// @Failure 403 {object} apierrors.Error
// @Failure 409 {object} apierrors.Error
// @Failure 500 {object} apierrors.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/alias [post]
func (h *ClusterHandler) AliasAdd(c echo.Context) error {
	if err := checkSuperuser(c); err != nil {
		return err
	}

	a := api.Alias{}

	if err := util.ShouldBindJSON(c, &a); err != nil {
		return apierrors.Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	if err := h.cluster.AliasAdd("", a.Unmarshal()); err != nil {
		return clusterAliasError(err)
	}

	return c.JSON(http.StatusOK, a)
}

// AliasGet returns an alias from the cluster DB
// @Summary Get an alias from the cluster
// @Description Get the alias with the given pattern from the cluster
// @Tags v16.?.?
// @ID cluster-3-alias-get
// @Produce json
// @Param pattern query string true "Pattern of the alias"
// @Success 200 {object} api.Alias
// @Failure 400 {object} apierrors.Error
// @Failure 404 {object} apierrors.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/alias/rule [get]
func (h *ClusterHandler) AliasGet(c echo.Context) error {
	pattern, err := patternParam(c)
	if err != nil {
		return err
	}

	r, err := h.cluster.Store().AliasGet(pattern)
	if err != nil {
		return clusterAliasError(err)
	}

	a := api.Alias{}
	a.Marshal(r)

	return c.JSON(http.StatusOK, a)
}

// AliasUpdate replaces an alias in the cluster
// @Summary Replace an alias in the cluster
// @Description Replace the alias with the given pattern in the cluster. The position of the alias is kept. Only superusers are allowed to modify aliases.
// @Tags v16.?.?
// @ID cluster-3-alias-update
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
// @Router /api/v3/cluster/alias/rule [put]
func (h *ClusterHandler) AliasUpdate(c echo.Context) error {
	if err := checkSuperuser(c); err != nil {
		return err
	}

	pattern, err := patternParam(c)
	if err != nil {
		return err
	}

	a := api.Alias{}

	if err := util.ShouldBindJSON(c, &a); err != nil {
		return apierrors.Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	if err := h.cluster.AliasUpdate("", pattern, a.Unmarshal()); err != nil {
		return clusterAliasError(err)
	}

	return c.JSON(http.StatusOK, a)
}

// AliasRemove removes an alias from the cluster
// @Summary Delete an alias from the cluster
// @Description Delete the alias with the given pattern from the cluster. Only superusers are allowed to modify aliases.
// @Tags v16.?.?
// @ID cluster-3-alias-delete
// @Produce json
// @Param pattern query string true "Pattern of the alias"
// @Success 200 {string} string
// @Failure 400 {object} apierrors.Error
// @Failure 403 {object} apierrors.Error
// @Failure 404 {object} apierrors.Error
// @Failure 500 {object} apierrors.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/alias/rule [delete]
func (h *ClusterHandler) AliasRemove(c echo.Context) error {
	if err := checkSuperuser(c); err != nil {
		return err
	}

	pattern, err := patternParam(c)
	if err != nil {
		return err
	}

	if err := h.cluster.AliasRemove("", pattern); err != nil {
		return clusterAliasError(err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// clusterAliasError maps an error from the cluster to a status code. Errors that
// have been forwarded from the leader only carry the store errors.
func clusterAliasError(err error) error {
	switch {
	case errors.Is(err, store.ErrBadRequest), errors.Is(err, alias.ErrInvalid):
		return apierrors.Err(http.StatusBadRequest, "", "%s", err.Error())
	case errors.Is(err, store.ErrConflict), errors.Is(err, alias.ErrExists):
		return apierrors.Err(http.StatusConflict, "", "%s", err.Error())
	case errors.Is(err, store.ErrNotFound), errors.Is(err, alias.ErrNotFound):
		return apierrors.Err(http.StatusNotFound, "", "%s", err.Error())
	}

	return apierrors.Err(http.StatusInternalServerError, "", "%s", err.Error())
}
