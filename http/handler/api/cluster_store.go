package api

import (
	"net/http"
	"time"

	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/handler/util"
	"github.com/datarhei/core/v16/iam/identity"
	"github.com/datarhei/core/v16/restream/app"

	"github.com/labstack/echo/v4"
)

// ListStoreProcesses returns the list of processes stored in the DB of the cluster
// @Summary List of processes in the cluster DB
// @Description List of processes in the cluster DB
// @Tags v16.?.?
// @ID cluster-3-db-list-processes
// @Produce json
// @Success 200 {array} api.Process
// @Security ApiKeyAuth
// @Router /api/v3/cluster/db/process [get]
func (h *ClusterHandler) ListStoreProcesses(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")

	procs := h.cluster.ListProcesses()

	processes := []api.Process{}

	for _, p := range procs {
		if !h.iam.Enforce(ctxuser, p.Config.Domain, "process", p.Config.ID, "read") {
			continue
		}

		process := h.convertStoreProcessToAPIProcess(p, newFilter(""))

		processes = append(processes, process)
	}

	return c.JSON(http.StatusOK, processes)
}

// GerStoreProcess returns a process stored in the DB of the cluster
// @Summary Get a process in the cluster DB
// @Description Get a process in the cluster DB
// @Tags v16.?.?
// @ID cluster-3-db-get-process
// @Produce json
// @Param id path string true "Process ID"
// @Param domain query string false "Domain to act on"
// @Success 200 {object} api.Process
// @Security ApiKeyAuth
// @Router /api/v3/cluster/db/process/:id [get]
func (h *ClusterHandler) GetStoreProcess(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	domain := util.DefaultQuery(c, "domain", "")
	id := util.PathParam(c, "id")

	pid := app.ProcessID{
		ID:     id,
		Domain: domain,
	}

	if !h.iam.Enforce(ctxuser, domain, "process", id, "read") {
		return api.Err(http.StatusForbidden, "", "API user %s is not allowed to read this process", ctxuser)
	}

	p, err := h.cluster.GetProcess(pid)
	if err != nil {
		return api.Err(http.StatusNotFound, "", "process not found: %s in domain '%s'", pid.ID, pid.Domain)
	}

	process := h.convertStoreProcessToAPIProcess(p, newFilter(""))

	return c.JSON(http.StatusOK, process)
}

// GetStoreProcessNodeMap returns a map of which process is running on which node
// @Summary Retrieve a map of which process is running on which node
// @Description Retrieve a map of which process is running on which node
// @Tags v16.?.?
// @ID cluster-3-db-process-node-map
// @Produce json
// @Success 200 {object} api.ClusterProcessMap
// @Security ApiKeyAuth
// @Router /api/v3/cluster/map/process [get]
func (h *ClusterHandler) GetStoreProcessNodeMap(c echo.Context) error {
	m := h.cluster.GetProcessNodeMap()

	return c.JSON(http.StatusOK, m)
}

// ListStoreIdentities returns the list of identities stored in the DB of the cluster
// @Summary List of identities in the cluster
// @Description List of identities in the cluster
// @Tags v16.?.?
// @ID cluster-3-db-list-identities
// @Produce json
// @Success 200 {array} api.IAMUser
// @Security ApiKeyAuth
// @Router /api/v3/cluster/db/user [get]
func (h *ClusterHandler) ListStoreIdentities(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	domain := util.DefaultQuery(c, "domain", "")

	updatedAt, identities := h.cluster.ListIdentities()

	users := make([]api.IAMUser, len(identities))

	for i, iamuser := range identities {
		if !h.iam.Enforce(ctxuser, domain, "iam", iamuser.Name, "read") {
			continue
		}

		if !h.iam.Enforce(ctxuser, domain, "iam", iamuser.Name, "write") {
			iamuser = identity.User{
				Name: iamuser.Name,
			}
		}

		_, policies := h.cluster.ListUserPolicies(iamuser.Name)
		users[i].Marshal(iamuser, policies)
	}

	c.Response().Header().Set("Last-Modified", updatedAt.UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT"))

	return c.JSON(http.StatusOK, users)
}

// ListStoreIdentity returns the list of identities stored in the DB of the cluster
// @Summary List of identities in the cluster
// @Description List of identities in the cluster
// @Tags v16.?.?
// @ID cluster-3-db-list-identity
// @Produce json
// @Success 200 {object} api.IAMUser
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/db/user/{name} [get]
func (h *ClusterHandler) ListStoreIdentity(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	domain := util.DefaultQuery(c, "domain", "")
	name := util.PathParam(c, "name")

	if !h.iam.Enforce(ctxuser, domain, "iam", name, "read") {
		return api.Err(http.StatusForbidden, "", "Not allowed to access this user")
	}

	var updatedAt time.Time
	var iamuser identity.User
	var err error

	if name != "$anon" {
		updatedAt, iamuser, err = h.cluster.ListIdentity(name)
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

	policiesUpdatedAt, policies := h.cluster.ListUserPolicies(name)
	if updatedAt.IsZero() {
		updatedAt = policiesUpdatedAt
	}

	user := api.IAMUser{}
	user.Marshal(iamuser, policies)

	c.Response().Header().Set("Last-Modified", updatedAt.UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT"))

	return c.JSON(http.StatusOK, user)
}

// ListStorePolicies returns the list of policies stored in the DB of the cluster
// @Summary List of policies in the cluster
// @Description List of policies in the cluster
// @Tags v16.?.?
// @ID cluster-3-db-list-policies
// @Produce json
// @Success 200 {array} api.IAMPolicy
// @Security ApiKeyAuth
// @Router /api/v3/cluster/db/policies [get]
func (h *ClusterHandler) ListStorePolicies(c echo.Context) error {
	updatedAt, clusterpolicies := h.cluster.ListPolicies()

	policies := []api.IAMPolicy{}

	for _, pol := range clusterpolicies {
		policies = append(policies, api.IAMPolicy{
			Name:     pol.Name,
			Domain:   pol.Domain,
			Resource: pol.Resource,
			Actions:  pol.Actions,
		})
	}

	c.Response().Header().Set("Last-Modified", updatedAt.UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT"))

	return c.JSON(http.StatusOK, policies)
}

// ListStoreLocks returns the list of currently stored locks
// @Summary List locks in the cluster DB
// @Description List of locks in the cluster DB
// @Tags v16.?.?
// @ID cluster-3-db-list-locks
// @Produce json
// @Success 200 {array} api.ClusterLock
// @Security ApiKeyAuth
// @Router /api/v3/cluster/db/locks [get]
func (h *ClusterHandler) ListStoreLocks(c echo.Context) error {
	clusterlocks := h.cluster.ListLocks()

	locks := []api.ClusterLock{}

	for name, validUntil := range clusterlocks {
		locks = append(locks, api.ClusterLock{
			Name:       name,
			ValidUntil: validUntil,
		})
	}

	return c.JSON(http.StatusOK, locks)
}

// ListStoreKV returns the list of currently stored key/value pairs
// @Summary List KV in the cluster DB
// @Description List of KV in the cluster DB
// @Tags v16.?.?
// @ID cluster-3-db-list-kv
// @Produce json
// @Success 200 {object} api.ClusterKVS
// @Security ApiKeyAuth
// @Router /api/v3/cluster/db/kv [get]
func (h *ClusterHandler) ListStoreKV(c echo.Context) error {
	clusterkv := h.cluster.ListKV("")

	kvs := api.ClusterKVS{}

	for key, v := range clusterkv {
		kvs[key] = api.ClusterKVSValue{
			Value:     v.Value,
			UpdatedAt: v.UpdatedAt,
		}
	}

	return c.JSON(http.StatusOK, kvs)
}
