package api

import (
	"errors"
	"net/http"

	"github.com/datarhei/core/v16/cluster/store"
	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/handler/util"
	"github.com/datarhei/core/v16/iam/identity"
	"github.com/datarhei/core/v16/iam/policy"
	"github.com/labstack/echo/v4"
)

// Add adds a new identity to the cluster
// @Summary Add a new identiy
// @Description Add a new identity
// @Tags v16.?.?
// @ID cluster-3-add-identity
// @Accept json
// @Produce json
// @Param config body api.IAMUser true "Identity"
// @Param domain query string false "Domain of the acting user"
// @Success 200 {object} api.IAMUser
// @Failure 400 {object} api.Error
// @Failure 403 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/iam/user [post]
func (h *ClusterHandler) IAMIdentityAdd(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	superuser := util.DefaultContext(c, "superuser", false)
	domain := util.DefaultQuery(c, "domain", "")

	user := api.IAMUser{}

	if err := util.ShouldBindJSON(c, &user); err != nil {
		return api.Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	iamuser, iampolicies := user.Unmarshal()

	if !h.iam.Enforce(ctxuser, domain, "iam", iamuser.Name, "write") {
		return api.Err(http.StatusForbidden, "", "Not allowed to create user '%s'", iamuser.Name)
	}

	for _, p := range iampolicies {
		if !h.iam.Enforce(ctxuser, p.Domain, "iam", iamuser.Name, "write") {
			return api.Err(http.StatusForbidden, "", "Not allowed to write policy: %v", p)
		}
	}

	if !superuser && iamuser.Superuser {
		return api.Err(http.StatusForbidden, "", "Only superusers can add superusers")
	}

	if err := h.cluster.IAMIdentityAdd("", iamuser); err != nil {
		return api.Err(http.StatusBadRequest, "", "invalid identity: %s", err.Error())
	}

	if err := h.cluster.IAMPoliciesSet("", iamuser.Name, iampolicies); err != nil {
		return api.Err(http.StatusBadRequest, "", "Invalid policies: %s", err.Error())
	}

	return c.JSON(http.StatusOK, user)
}

// IAMIdentityUpdate replaces an existing user
// @Summary Replace an existing user
// @Description Replace an existing user.
// @Tags v16.?.?
// @ID cluster-3-update-identity
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
// @Router /api/v3/cluster/iam/user/{name} [put]
func (h *ClusterHandler) IAMIdentityUpdate(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	superuser := util.DefaultContext(c, "superuser", false)
	domain := util.DefaultQuery(c, "domain", "")
	name := util.PathParam(c, "name")

	if !h.iam.Enforce(ctxuser, domain, "iam", name, "write") {
		return api.Err(http.StatusForbidden, "", "not allowed to modify this user")
	}

	var iamuser identity.User
	var err error

	if name != "$anon" {
		iamuser, err = h.iam.GetIdentity(name)
		if err != nil {
			return api.Err(http.StatusNotFound, "", "user not found: %s", err.Error())
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
		return api.Err(http.StatusForbidden, "", "not allowed to create user '%s'", iamuser.Name)
	}

	for _, p := range iampolicies {
		if !h.iam.Enforce(ctxuser, p.Domain, "iam", iamuser.Name, "write") {
			return api.Err(http.StatusForbidden, "", "not allowed to write policy: %v", p)
		}
	}

	if !superuser && iamuser.Superuser {
		return api.Err(http.StatusForbidden, "", "only superusers can modify superusers")
	}

	if name != "$anon" {
		err = h.cluster.IAMIdentityUpdate("", name, iamuser)
		if err != nil {
			return api.Err(http.StatusBadRequest, "", "%s", err.Error())
		}
	}

	err = h.cluster.IAMPoliciesSet("", iamuser.Name, iampolicies)
	if err != nil {
		return api.Err(http.StatusInternalServerError, "", "set policies: %s", err.Error())
	}

	return c.JSON(http.StatusOK, user)
}

// IAMIdentityUpdatePolicies replaces existing user policies
// @Summary Replace policies of an user
// @Description Replace policies of an user
// @Tags v16.?.?
// @ID cluster-3-update-user-policies
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
// @Router /api/v3/cluster/iam/user/{name}/policy [put]
func (h *ClusterHandler) IAMIdentityUpdatePolicies(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	superuser := util.DefaultContext(c, "superuser", false)
	domain := util.DefaultQuery(c, "domain", "")
	name := util.PathParam(c, "name")

	if !h.iam.Enforce(ctxuser, domain, "iam", name, "write") {
		return api.Err(http.StatusForbidden, "", "not allowed to modify this user")
	}

	var iamuser identity.User
	var err error

	if name != "$anon" {
		iamuser, err = h.iam.GetIdentity(name)
		if err != nil {
			return api.Err(http.StatusNotFound, "", "unknown identity: %s", err.Error())
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

	accessPolicies := []policy.Policy{}

	for _, p := range policies {
		if !h.iam.Enforce(ctxuser, p.Domain, "iam", iamuser.Name, "write") {
			return api.Err(http.StatusForbidden, "", "not allowed to write policy: %v", p)
		}

		accessPolicies = append(accessPolicies, policy.Policy{
			Name:     name,
			Domain:   p.Domain,
			Types:    p.Types,
			Resource: p.Resource,
			Actions:  p.Actions,
		})
	}

	if !superuser && iamuser.Superuser {
		return api.Err(http.StatusForbidden, "", "only superusers can modify superusers")
	}

	err = h.cluster.IAMPoliciesSet("", name, accessPolicies)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return api.Err(http.StatusNotFound, "", "set policies: %s", err.Error())
		}
		return api.Err(http.StatusInternalServerError, "", "set policies: %s", err.Error())
	}

	return c.JSON(http.StatusOK, policies)
}

// IAMReload reloads the identities and policies from the cluster store to IAM
// @Summary Reload identities and policies
// @Description Reload identities and policies
// @Tags v16.?.?
// @ID cluster-3-iam-reload
// @Produce json
// @Success 200 {string} string
// @Success 500 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/iam/reload [get]
func (h *ClusterHandler) IAMReload(c echo.Context) error {
	err := h.iam.ReloadIndentities()
	if err != nil {
		return api.Err(http.StatusInternalServerError, "", "reload identities: %w", err.Error())
	}

	err = h.iam.ReloadPolicies()
	if err != nil {
		return api.Err(http.StatusInternalServerError, "", "reload policies: %w", err.Error())
	}

	return c.JSON(http.StatusOK, "OK")
}

// IAMIdentityList returns the list of identities stored in IAM
// @Summary List of identities in IAM
// @Description List of identities in IAM
// @Tags v16.?.?
// @ID cluster-3-iam-list-identities
// @Produce json
// @Param domain query string false "Domain of the acting user"
// @Success 200 {array} api.IAMUser
// @Security ApiKeyAuth
// @Router /api/v3/cluster/iam/user [get]
func (h *ClusterHandler) IAMIdentityList(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	domain := util.DefaultQuery(c, "domain", "")

	identities := h.iam.ListIdentities()

	users := []api.IAMUser{}

	for _, iamuser := range identities {
		if !h.iam.Enforce(ctxuser, domain, "iam", iamuser.Name, "read") {
			continue
		}

		if !h.iam.Enforce(ctxuser, domain, "iam", iamuser.Name, "write") {
			iamuser = identity.User{
				Name: iamuser.Name,
			}
		}

		policies := h.iam.ListPolicies(iamuser.Name, "", nil, "", nil)

		user := api.IAMUser{}
		user.Marshal(iamuser, policies)
		users = append(users, user)
	}

	anon := identity.User{
		Name: "$anon",
	}

	policies := h.iam.ListPolicies("$anon", "", nil, "", nil)

	user := api.IAMUser{}
	user.Marshal(anon, policies)
	users = append(users, user)

	return c.JSON(http.StatusOK, users)
}

// IAMIdentityGet returns the identity stored in IAM
// @Summary Identity in IAM
// @Description Identity in IAM
// @Tags v16.?.?
// @ID cluster-3-iam-list-identity
// @Produce json
// @Param domain query string false "Domain of the acting user"
// @Success 200 {object} api.IAMUser
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/iam/user/{name} [get]
func (h *ClusterHandler) IAMIdentityGet(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	domain := util.DefaultQuery(c, "domain", "")
	name := util.PathParam(c, "name")

	if !h.iam.Enforce(ctxuser, domain, "iam", name, "read") {
		return api.Err(http.StatusForbidden, "", "Not allowed to access this user")
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

	iampolicies := h.iam.ListPolicies(iamuser.Name, "", nil, "", nil)

	user := api.IAMUser{}
	user.Marshal(iamuser, iampolicies)

	return c.JSON(http.StatusOK, user)
}

// IAMPolicyList returns the list of policies stored in IAM
// @Summary List of policies in IAM
// @Description List of policies IAM
// @Tags v16.?.?
// @ID cluster-3-iam-list-policies
// @Produce json
// @Param domain query string false "Domain of the acting user"
// @Success 200 {array} api.IAMPolicy
// @Security ApiKeyAuth
// @Router /api/v3/cluster/iam/policies [get]
func (h *ClusterHandler) IAMPolicyList(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	domain := util.DefaultQuery(c, "domain", "")

	iampolicies := h.iam.ListPolicies("", "", nil, "", nil)

	policies := []api.IAMPolicy{}

	for _, pol := range iampolicies {
		if !h.iam.Enforce(ctxuser, domain, "iam", pol.Name, "read") {
			continue
		}

		policies = append(policies, api.IAMPolicy{
			Name:     pol.Name,
			Domain:   pol.Domain,
			Types:    pol.Types,
			Resource: pol.Resource,
			Actions:  pol.Actions,
		})
	}

	return c.JSON(http.StatusOK, policies)
}

// Delete deletes the identity with the given name from the cluster
// @Summary Delete an identity by its name
// @Description Delete an identity by its name
// @Tags v16.?.?
// @ID cluster-3-delete-identity
// @Produce json
// @Param name path string true "Identity name"
// @Param domain query string false "Domain of the acting user"
// @Success 200 {string} string
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/iam/user/{name} [delete]
func (h *ClusterHandler) IAMIdentityRemove(c echo.Context) error {
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

	if err := h.cluster.IAMIdentityRemove("", name); err != nil {
		return api.Err(http.StatusBadRequest, "", "invalid identity: %s", err.Error())
	}

	return c.JSON(http.StatusOK, "OK")
}
