package api

import (
	"net/http"

	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/handler/util"
	"github.com/datarhei/core/v16/iam"

	"github.com/labstack/echo/v4"
)

type IAMHandler struct {
	iam iam.IAM
}

func NewIAM(iam iam.IAM) *IAMHandler {
	return &IAMHandler{
		iam: iam,
	}
}

// Add adds a new user
// @Summary Add a new user
// @Description Add a new user
// @Tags v16.?.?
// @ID iam-3-add-user
// @Accept json
// @Produce json
// @Param config body api.IAMUser true "User definition"
// @Success 200 {object} api.IAMUser
// @Failure 400 {object} api.Error
// @Failure 500 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/iam/user [post]
func (h *IAMHandler) AddUser(c echo.Context) error {
	//user := util.DefaultContext(c, "user", "")

	user := api.IAMUser{}

	if err := util.ShouldBindJSON(c, &user); err != nil {
		return api.Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
	}

	iamuser, iampolicies := user.Unmarshal()

	err := h.iam.CreateIdentity(iamuser)
	if err != nil {
		return api.Err(http.StatusBadRequest, "Bad request", "%s", err)
	}

	for _, p := range iampolicies {
		h.iam.AddPolicy(p.Name, p.Domain, p.Resource, p.Actions)
	}

	err = h.iam.SaveIdentities()
	if err != nil {
		return api.Err(http.StatusInternalServerError, "Internal server error", "%s", err)
	}

	return c.JSON(http.StatusOK, user)
}

// Delete deletes the user with the given name
// @Summary Delete an user by its name
// @Description Delete an user by its name
// @Tags v16.?.?
// @ID iam-3-delete-user
// @Produce json
// @Param name path string true "Username"
// @Success 200 {string} string
// @Failure 404 {object} api.Error
// @Failure 500 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/iam/user/{name} [delete]
func (h *IAMHandler) RemoveUser(c echo.Context) error {
	name := util.PathParam(c, "name")

	err := h.iam.DeleteIdentity(name)
	if err != nil {
		return api.Err(http.StatusBadRequest, "Bad request", "%s", err)
	}

	err = h.iam.SaveIdentities()
	if err != nil {
		return api.Err(http.StatusInternalServerError, "Internal server error", "%s", err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// Update replaces an existing user
// @Summary Replace an existing user
// @Description Replace an existing user.
// @Tags v16.?.?
// @ID iam-3-update-user
// @Accept json
// @Produce json
// @Param name path string true "Username"
// @Param user body api.IAMUser true "User definition"
// @Success 200 {object} api.IAMUser
// @Failure 400 {object} api.Error
// @Failure 404 {object} api.Error
// @Failure 500 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/iam/user/{name} [put]
func (h *IAMHandler) UpdateUser(c echo.Context) error {
	name := util.PathParam(c, "name")

	iamuser, err := h.iam.GetIdentity(name)
	if err != nil {
		return api.Err(http.StatusNotFound, "Not found", "%s", err)
	}

	iampolicies := h.iam.ListPolicies(name, "", "", nil)

	user := api.IAMUser{}
	user.Marshal(iamuser, iampolicies)

	if err := util.ShouldBindJSON(c, &user); err != nil {
		return api.Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
	}

	iamuser, iampolicies = user.Unmarshal()

	err = h.iam.UpdateIdentity(name, iamuser)
	if err != nil {
		return api.Err(http.StatusBadRequest, "Bad request", "%s", err)
	}

	h.iam.RemovePolicy(name, "", "", nil)

	for _, p := range iampolicies {
		h.iam.AddPolicy(p.Name, p.Domain, p.Resource, p.Actions)
	}

	err = h.iam.SaveIdentities()
	if err != nil {
		return api.Err(http.StatusInternalServerError, "Internal server error", "%s", err)
	}

	return c.JSON(http.StatusOK, user)
}

// Get returns the user with the given name
// @Summary List an user by its name
// @Description List aa user by its name
// @Tags v16.?.?
// @ID iam-3-get-user
// @Produce json
// @Param name path string true "Username"
// @Success 200 {object} api.IAMUser
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/iam/user/{name} [get]
func (h *IAMHandler) GetUser(c echo.Context) error {
	name := util.PathParam(c, "name")

	iamuser, err := h.iam.GetIdentity(name)
	if err != nil {
		return api.Err(http.StatusNotFound, "Not found", "%s", err)
	}

	iampolicies := h.iam.ListPolicies(name, "", "", nil)

	user := api.IAMUser{}
	user.Marshal(iamuser, iampolicies)

	return c.JSON(http.StatusOK, user)
}
