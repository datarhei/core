package api

import (
	"net/http"

	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/handler/util"
	"github.com/datarhei/core/v16/iam"
	"github.com/datarhei/core/v16/iam/identity"

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

// AddIdentity adds a new user
// @Summary Add a new user
// @Description Add a new user
// @Tags v16.?.?
// @ID iam-3-add-user
// @Accept json
// @Produce json
// @Param config body api.IAMUser true "User definition"
// @Param domain query string false "Domain of the acting user"
// @Success 200 {object} api.IAMUser
// @Failure 400 {object} api.Error
// @Failure 403 {object} api.Error
// @Failure 500 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/iam/user [post]
func (h *IAMHandler) AddIdentity(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	superuser := util.DefaultContext(c, "superuser", false)
	domain := util.DefaultQuery(c, "domain", "$none")

	user := api.IAMUser{}

	if err := util.ShouldBindJSON(c, &user); err != nil {
		return api.Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	iamuser, iampolicies := user.Unmarshal()

	if !h.iam.Enforce(ctxuser, domain, "iam", iamuser.Name, "write") {
		return api.Err(http.StatusForbidden, "", "not allowed to create user '%s'", iamuser.Name)
	}

	for _, p := range iampolicies {
		if !h.iam.Enforce(ctxuser, p.Domain, "iam", iamuser.Name, "write") {
			return api.Err(http.StatusForbidden, "", "not allowed to write policy: %v", p)
		}
	}

	if !superuser && iamuser.Superuser {
		return api.Err(http.StatusForbidden, "", "only superusers can add superusers")
	}

	err := h.iam.CreateIdentity(iamuser)
	if err != nil {
		return api.Err(http.StatusBadRequest, "", "%s", err.Error())
	}

	for _, p := range iampolicies {
		h.iam.AddPolicy(p.Name, p.Domain, p.Types, p.Resource, p.Actions)
	}

	return c.JSON(http.StatusOK, user)
}

// RemoveIdentity deletes the user with the given name
// @Summary Delete an user by its name
// @Description Delete an user by its name
// @Tags v16.?.?
// @ID iam-3-delete-user
// @Produce json
// @Param name path string true "Username"
// @Param domain query string false "Domain of the acting user"
// @Success 200 {string} string
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Failure 500 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/iam/user/{name} [delete]
func (h *IAMHandler) RemoveIdentity(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	superuser := util.DefaultContext(c, "superuser", false)
	domain := util.DefaultQuery(c, "domain", "$none")
	name := util.PathParam(c, "name")

	if !h.iam.Enforce(ctxuser, domain, "iam", name, "write") {
		return api.Err(http.StatusForbidden, "", "Not allowed to delete this user")
	}

	iamuser, err := h.iam.GetIdentity(name)
	if err != nil {
		return api.Err(http.StatusNotFound, "", "%s", err.Error())
	}

	if !superuser && iamuser.Superuser {
		return api.Err(http.StatusForbidden, "", "Only superusers can remove superusers")
	}

	// Remove the user
	if err := h.iam.DeleteIdentity(name); err != nil {
		return api.Err(http.StatusBadRequest, "", "%s", err.Error())
	}

	// Remove all policies of that user
	if err := h.iam.RemovePolicy(name, "", nil, "", nil); err != nil {
		return api.Err(http.StatusBadRequest, "", "%s", err.Error())
	}

	return c.JSON(http.StatusOK, "OK")
}

// UpdateIdentity replaces an existing user
// @Summary Replace an existing user
// @Description Replace an existing user.
// @Tags v16.?.?
// @ID iam-3-update-user
// @Accept json
// @Produce json
// @Param name path string true "Username"
// @Param domain query string false "Domain of the acting user"
// @Param user body api.IAMUser true "User definition"
// @Success 200 {object} api.IAMUser
// @Failure 400 {object} api.Error
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Failure 500 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/iam/user/{name} [put]
func (h *IAMHandler) UpdateIdentity(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	superuser := util.DefaultContext(c, "superuser", false)
	domain := util.DefaultQuery(c, "domain", "$none")
	name := util.PathParam(c, "name")

	if !h.iam.Enforce(ctxuser, domain, "iam", name, "write") {
		return api.Err(http.StatusForbidden, "", "Not allowed to modify this user")
	}

	var iamuser identity.User
	var err error

	if name != "$anon" {
		iamuser, err = h.iam.GetIdentity(name)
		if err != nil {
			return api.Err(http.StatusNotFound, "", "%s", err.Error())
		}
	} else {
		iamuser = identity.User{
			Name: "$anon",
		}
	}

	iampolicies := h.iam.ListPolicies(name, "", nil, "", nil)

	user := api.IAMUser{}
	user.Marshal(iamuser, iampolicies)

	if err := util.ShouldBindJSON(c, &user); err != nil {
		return api.Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	iamuser, iampolicies = user.Unmarshal()

	if !h.iam.Enforce(ctxuser, domain, "iam", iamuser.Name, "write") {
		return api.Err(http.StatusForbidden, "", "Not allowed to create user '%s'", iamuser.Name)
	}

	for _, p := range iampolicies {
		if !h.iam.Enforce(ctxuser, p.Domain, "iam", iamuser.Name, "write") {
			return api.Err(http.StatusForbidden, "", "Not allowed to write policy: %v", p)
		}
	}

	if !superuser && iamuser.Superuser {
		return api.Err(http.StatusForbidden, "", "Only superusers can modify superusers")
	}

	if name != "$anon" {
		err = h.iam.UpdateIdentity(name, iamuser)
		if err != nil {
			return api.Err(http.StatusBadRequest, "", "%s", err.Error())
		}
	}

	if err := h.iam.RemovePolicy(name, "", nil, "", nil); err != nil {
		return api.Err(http.StatusBadRequest, "", "%s", err.Error())
	}

	for _, p := range iampolicies {
		h.iam.AddPolicy(p.Name, p.Domain, p.Types, p.Resource, p.Actions)
	}

	return c.JSON(http.StatusOK, user)
}

// UpdateIdentityPolicies replaces existing user policies
// @Summary Replace policies of an user
// @Description Replace policies of an user
// @Tags v16.?.?
// @ID iam-3-update-user-policies
// @Accept json
// @Produce json
// @Param name path string true "Username"
// @Param domain query string false "Domain of the acting user"
// @Param user body []api.IAMPolicy true "Policy definitions"
// @Success 200 {array} api.IAMPolicy
// @Failure 400 {object} api.Error
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Failure 500 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/iam/user/{name}/policy [put]
func (h *IAMHandler) UpdateIdentityPolicies(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	superuser := util.DefaultContext(c, "superuser", false)
	domain := util.DefaultQuery(c, "domain", "$none")
	name := util.PathParam(c, "name")

	if !h.iam.Enforce(ctxuser, domain, "iam", name, "write") {
		return api.Err(http.StatusForbidden, "", "Not allowed to modify this user")
	}

	var iamuser identity.User
	var err error

	if name != "$anon" {
		iamuser, err = h.iam.GetIdentity(name)
		if err != nil {
			return api.Err(http.StatusNotFound, "", "%s", err.Error())
		}
	} else {
		iamuser = identity.User{
			Name: "$anon",
		}
	}

	policies := []api.IAMPolicy{}

	if err := util.ShouldBindJSONValidation(c, &policies, false); err != nil {
		return api.Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	for _, p := range policies {
		err := c.Validate(p)
		if err != nil {
			return api.Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
		}
	}

	for _, p := range policies {
		if !h.iam.Enforce(ctxuser, p.Domain, "iam", iamuser.Name, "write") {
			return api.Err(http.StatusForbidden, "", "not allowed to write policy: %v", p)
		}
	}

	if !superuser && iamuser.Superuser {
		return api.Err(http.StatusForbidden, "", "only superusers can modify superusers")
	}

	if err := h.iam.RemovePolicy(name, "", nil, "", nil); err != nil {
		return api.Err(http.StatusBadRequest, "", "%s", err.Error())
	}

	for _, p := range policies {
		h.iam.AddPolicy(iamuser.Name, p.Domain, p.Types, p.Resource, p.Actions)
	}

	return c.JSON(http.StatusOK, policies)
}

// ListIdentities returns the list of identities stored in IAM
// @Summary List of identities in IAM
// @Description List of identities in IAM
// @Tags v16.?.?
// @ID iam-3-list-identities
// @Produce json
// @Success 200 {array} api.IAMUser
// @Security ApiKeyAuth
// @Router /api/v3/iam/user [get]
func (h *IAMHandler) ListIdentities(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	domain := util.DefaultQuery(c, "domain", "$none")

	identities := h.iam.ListIdentities()

	users := make([]api.IAMUser, len(identities)+1)

	for i, iamuser := range identities {
		if !h.iam.Enforce(ctxuser, domain, "iam", iamuser.Name, "read") {
			continue
		}

		if !h.iam.Enforce(ctxuser, domain, "iam", iamuser.Name, "write") {
			iamuser = identity.User{
				Name: iamuser.Name,
			}
		}

		policies := h.iam.ListPolicies(iamuser.Name, "", nil, "", nil)

		users[i].Marshal(iamuser, policies)
	}

	anon := identity.User{
		Name: "$anon",
	}

	policies := h.iam.ListPolicies("$anon", "", nil, "", nil)

	users[len(users)-1].Marshal(anon, policies)

	return c.JSON(http.StatusOK, users)
}

// GetIdentity returns the user with the given name
// @Summary List an user by its name
// @Description List aa user by its name
// @Tags v16.?.?
// @ID iam-3-get-user
// @Produce json
// @Param name path string true "Username"
// @Param domain query string false "Domain of the acting user"
// @Success 200 {object} api.IAMUser
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/iam/user/{name} [get]
func (h *IAMHandler) GetIdentity(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	domain := util.DefaultQuery(c, "domain", "$none")
	name := util.PathParam(c, "name")

	if !h.iam.Enforce(ctxuser, domain, "iam", name, "read") {
		return api.Err(http.StatusForbidden, "", "not allowed to access this user")
	}

	var iamuser identity.User
	var err error

	if name != "$anon" {
		iamuser, err = h.iam.GetIdentity(name)
		if err != nil {
			return api.Err(http.StatusNotFound, "", "%s", err.Error())
		}

		if ctxuser != iamuser.Name {
			if !h.iam.Enforce(ctxuser, domain, "iam", name, "write") {
				iamuser = identity.User{
					Name: iamuser.Name,
				}
			}
		}
	} else {
		iamuser = identity.User{
			Name: "$anon",
		}
	}

	iampolicies := h.iam.ListPolicies(name, "", nil, "", nil)

	user := api.IAMUser{}
	user.Marshal(iamuser, iampolicies)

	return c.JSON(http.StatusOK, user)
}
