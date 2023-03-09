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

// AddUser adds a new user
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
	ctxuser := util.DefaultContext(c, "user", "")

	if !h.iam.Enforce(ctxuser, "$none", "iam:/user", "write") {
		return api.Err(http.StatusForbidden, "Forbidden")
	}

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
		if len(p.Domain) != 0 {
			continue
		}

		h.iam.AddPolicy(p.Name, "$none", p.Resource, p.Actions)
	}

	err = h.iam.SaveIdentities()
	if err != nil {
		return api.Err(http.StatusInternalServerError, "Internal server error", "%s", err)
	}

	return c.JSON(http.StatusOK, user)
}

// RemoveUser deletes the user with the given name
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
	ctxuser := util.DefaultContext(c, "user", "")
	name := util.PathParam(c, "name")

	if !h.iam.Enforce(ctxuser, "$none", "iam:/user", "write") {
		return api.Err(http.StatusForbidden, "Forbidden")
	}

	// Remove all policies of that user
	h.iam.RemovePolicy(name, "", "", nil)

	// Remove the user
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

// UpdateUser replaces an existing user
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
	ctxuser := util.DefaultContext(c, "user", "")
	name := util.PathParam(c, "name")

	if !h.iam.Enforce(ctxuser, "$none", "iam:/user", "write") {
		return api.Err(http.StatusForbidden, "Forbidden")
	}

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

	h.iam.RemovePolicy(name, "$none", "", nil)

	for _, p := range iampolicies {
		if len(p.Domain) != 0 {
			continue
		}

		h.iam.AddPolicy(p.Name, "$none", p.Resource, p.Actions)
	}

	err = h.iam.SaveIdentities()
	if err != nil {
		return api.Err(http.StatusInternalServerError, "Internal server error", "%s", err)
	}

	return c.JSON(http.StatusOK, user)
}

// GetUser returns the user with the given name
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
	ctxuser := util.DefaultContext(c, "user", "")
	name := util.PathParam(c, "name")

	if !h.iam.Enforce(ctxuser, "$none", "iam:/user", "read") {
		return api.Err(http.StatusForbidden, "Forbidden")
	}

	iamuser, err := h.iam.GetIdentity(name)
	if err != nil {
		return api.Err(http.StatusNotFound, "Not found", "%s", err)
	}

	iampolicies := h.iam.ListPolicies(name, "$none", "", nil)

	user := api.IAMUser{}
	user.Marshal(iamuser, iampolicies)

	return c.JSON(http.StatusOK, user)
}

// AddGroup creates a group with admins
// @Summary Create a group with admins
// @Description Create a group with admins
// @Tags v16.?.?
// @ID iam-3-add-group
// @Produce json
// @Param config body api.IAMGroup true "Group to add"
// @Success 200 {object} api.IAMGroup
// @Failure 400 {object} api.Error
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Failure 409 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/iam/group [post]
func (h *IAMHandler) AddGroup(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")

	if !h.iam.Enforce(ctxuser, "$none", "iam:/group", "write") {
		return api.Err(http.StatusForbidden, "Forbidden")
	}

	group := api.IAMGroup{}

	if err := util.ShouldBindJSON(c, &group); err != nil {
		return api.Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
	}

	if h.iam.HasDomain(group.Name) {
		return api.Err(http.StatusConflict, "Conflict", "this group already exists")
	}

	if len(group.Admins) == 0 {
		return api.Err(http.StatusBadRequest, "Bad request", "at leas one admin must be defined")
	}

	for _, admin := range group.Admins {
		_, err := h.iam.GetIdentity(admin)
		if err != nil {
			return api.Err(http.StatusBadRequest, "Bad request", "the user %s doesn't exist", admin)
		}
	}

	for _, admin := range group.Admins {
		h.iam.AddPolicy(admin, group.Name, "api:/api/**", []string{"get", "options", "head"})
		h.iam.AddPolicy(admin, group.Name, "api:/api/v3/process", []string{"ANY"})
		h.iam.AddPolicy(admin, group.Name, "api:/api/v3/process/**", []string{"ANY"})
		h.iam.AddPolicy(admin, group.Name, "process:*", []string{"ANY"})
		h.iam.AddPolicy(admin, group.Name, "fs:/"+group.Name+"/**", []string{"ANY"})
		h.iam.AddPolicy(admin, group.Name, "fs:/memfs/"+group.Name+"/**", []string{"ANY"})
		h.iam.AddPolicy(admin, group.Name, "rtmp:/"+group.Name+"/**", []string{"ANY"})
		h.iam.AddPolicy(admin, group.Name, "srt:"+group.Name+"/**", []string{"ANY"})
		h.iam.AddPolicy(admin, group.Name, "iam:/group/"+group.Name, []string{"ANY"})
	}

	return c.JSON(http.StatusOK, group)
}

// ListGroups lists all groups
// @Summary List all groups
// @Description List all groups
// @Tags v16.?.?
// @ID iam-3-list-groups
// @Produce json
// @Success 200 {array} string
// @Failure 403 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/iam/group [get]
func (h *IAMHandler) ListGroups(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")

	if !h.iam.Enforce(ctxuser, "$none", "iam:/group", "read") {
		return api.Err(http.StatusForbidden, "Forbidden")
	}

	return c.JSON(http.StatusOK, h.iam.ListDomains())
}

// RemoveGroup removes a group
// @Summary Remove a group
// @Description Remove a group
// @Tags v16.?.?
// @ID iam-3-remove-group
// @Produce json
// @Param group path string true "group name"
// @Success 200 {object} api.IAMGroup
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/iam/group/{group} [delete]
func (h *IAMHandler) RemoveGroup(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	group := util.PathParam(c, "group")

	if !h.iam.Enforce(ctxuser, group, "iam:/group/"+group, "admin") {
		return api.Err(http.StatusForbidden, "Forbidden", "only group admin can remove a group")
	}

	if !h.iam.HasDomain(group) {
		return api.Err(http.StatusNotFound, "Not found")
	}

	h.iam.RemovePolicy("", group, "", nil)

	return c.JSON(http.StatusOK, "OK")
}

// GetGroup returns details of a group
// @Summary Get details of a group
// @Description Get details of a group
// @Tags v16.?.?
// @ID iam-3-get-group
// @Produce json
// @Param group path string true "group name"
// @Success 200 {object} api.IAMGroup
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/iam/group/{group} [get]
func (h *IAMHandler) GetGroup(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	group := util.PathParam(c, "group")

	if !h.iam.Enforce(ctxuser, group, "iam:/group/"+group, "read") {
		return api.Err(http.StatusForbidden, "Forbidden")
	}

	if !h.iam.HasDomain(group) {
		return api.Err(http.StatusNotFound, "Not found")
	}

	g := api.IAMGroup{
		Name: group,
	}

	admins := map[string]struct{}{}

	policies := h.iam.ListPolicies("", group, "iam:/group/"+g.Name, []string{"any"})
	for _, p := range policies {
		admins[p.Name] = struct{}{}
	}

	for name := range admins {
		g.Admins = append(g.Admins, name)
	}

	return c.JSON(http.StatusOK, g)
}

// ListGroupUsers lists all users of a group
// @Summary List all users in a group
// @Description List all users in a group
// @Tags v16.?.?
// @ID iam-3-get-group-users
// @Produce json
// @Param group path string true "group name"
// @Success 200 {array} string
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/iam/group/{group}/user [get]
func (h *IAMHandler) ListGroupUsers(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	group := util.PathParam(c, "group")

	if !h.iam.Enforce(ctxuser, group, "iam:/group/"+group, "read") {
		return api.Err(http.StatusForbidden, "Forbidden")
	}

	if !h.iam.HasDomain(group) {
		return api.Err(http.StatusNotFound, "Not found")
	}

	members := map[string]struct{}{}

	policies := h.iam.ListPolicies("", group, "", nil)
	for _, p := range policies {
		members[p.Name] = struct{}{}
	}

	list := []string{}

	for name := range members {
		list = append(list, name)
	}

	return c.JSON(http.StatusOK, list)
}

// AddGroupUser adds an user to a group
// @Summary Add an user to a group
// @Description Add an user to a group
// @Tags v16.?.?
// @ID iam-3-add-group-user
// @Produce json
// @Param group path string true "group name"
// @Param config body api.IAMGroupUser true "User to add"
// @Success 200 {array} string
// @Failure 400 {object} api.Error
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Failure 409 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/iam/group/{group}/user [post]
func (h *IAMHandler) AddGroupUser(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	group := util.PathParam(c, "group")

	if !h.iam.Enforce(ctxuser, group, "iam:/group/"+group, "write") {
		return api.Err(http.StatusForbidden, "Forbidden")
	}

	if !h.iam.HasDomain(group) {
		return api.Err(http.StatusNotFound, "Not found", "this group doesn't exists")
	}

	user := api.IAMGroupUser{}

	if err := util.ShouldBindJSON(c, &user); err != nil {
		return api.Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
	}

	policies := h.iam.ListPolicies(user.Name, group, "", nil)
	if len(policies) != 0 {
		return api.Err(http.StatusConflict, "Conflict", "this user is already in the group")
	}

	// Check if admin and add admin rights if required
	if user.Admin {
		if !h.iam.Enforce(ctxuser, group, "iam:/group/"+group, "admin") {
			return api.Err(http.StatusForbidden, "Forbidden", "you can't add admins to this group")
		}

		h.iam.AddPolicy(user.Name, group, "iam:/group/"+group, []string{"ANY"})
	}

	for _, p := range user.Policies {
		if p.Domain != group {
			continue
		}

		h.iam.AddPolicy(user.Name, group, p.Resource, p.Actions)
	}

	return c.JSON(http.StatusOK, "OK")
}

// GetGroupUser returns the details of a user in a group
// @Summary Get the details of a user in a group
// @Description Get the details of a user in a group
// @Tags v16.?.?
// @ID iam-3-get-group-user
// @Produce json
// @Param group path string true "group name"
// @Param name path string true "user name"
// @Success 200 {array} string
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/iam/group/{group}/user/{name} [get]
func (h *IAMHandler) GetGroupUser(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	group := util.PathParam(c, "group")
	name := util.PathParam(c, "name")

	if !h.iam.Enforce(ctxuser, group, "iam:/group/"+group, "read") {
		return api.Err(http.StatusForbidden, "Forbidden")
	}

	if !h.iam.HasDomain(group) {
		return api.Err(http.StatusNotFound, "Not found")
	}

	policies := h.iam.ListPolicies(name, group, "", nil)
	if len(policies) == 0 {
		return api.Err(http.StatusNotFound, "Not found")
	}

	user := api.IAMGroupUser{
		Name:  name,
		Admin: h.iam.HasPolicy(name, group, "iam:/group/"+group, []string{"any"}),
	}

	for _, p := range policies {
		user.Policies = append(user.Policies, api.IAMPolicy{
			Domain:   group,
			Resource: p.Resource,
			Actions:  p.Actions,
		})
	}

	return c.JSON(http.StatusOK, user)
}

// UpdateGroupUser sets the policies of a user in a group
// @Summary Set the policies of a user in a group
// @Description Set the policies of a user in a group
// @Tags v16.?.?
// @ID iam-3-update-group-user
// @Produce json
// @Param group path string true "group name"
// @Param name path string true "user name"
// @Param config body api.IAMPolicy true "User to add"
// @Success 200 {array} string
// @Failure 400 {object} api.Error
// @Failure 403 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/iam/group/{group}/user/{name} [put]
func (h *IAMHandler) UpdateGroupUser(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	group := util.PathParam(c, "group")
	//name := util.PathParam(c, "name")

	if !h.iam.Enforce(ctxuser, group, "iam:/group/"+group, "write") {
		return api.Err(http.StatusForbidden, "Forbidden")
	}

	policies := []api.IAMPolicy{}

	if err := util.ShouldBindJSON(c, &policies); err != nil {
		return api.Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// RemoveGroupUser removes a user from a group
// @Summary Remove a user from a group
// @Description Remove a user from a group
// @Tags v16.?.?
// @ID iam-3-remove-group-user
// @Produce json
// @Param group path string true "group name"
// @Param name path string true "user name"
// @Success 200 {array} string
// @Failure 400 {object} api.Error
// @Failure 403 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/iam/group/{group}/user/{name} [delete]
func (h *IAMHandler) RemoveGroupUser(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	group := util.PathParam(c, "group")
	name := util.PathParam(c, "name")

	if !h.iam.Enforce(ctxuser, group, "iam:/group/"+group, "write") {
		return api.Err(http.StatusForbidden, "Forbidden")
	}

	// Check if the user to be deleted is an admin. If yes, you have to be an admin too.
	if h.iam.HasPolicy(name, group, "iam:/group/"+group, []string{"any"}) {
		if !h.iam.Enforce(ctxuser, group, "iam:/group/"+group, "admin") {
			return api.Err(http.StatusForbidden, "Forbidden")
		}
	}

	if len(h.iam.ListPolicies(name, group, "", nil)) == 0 {
		return api.Err(http.StatusNotFound, "Not found")
	}

	h.iam.RemovePolicy(name, group, "", nil)

	return c.JSON(http.StatusOK, "OK")
}
