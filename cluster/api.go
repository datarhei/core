// @title datarhei Core Cluster API
// @version 1.0
// @description Internal REST API for the datarhei Core cluster

// @contact.name datarhei Core Support
// @contact.url https://www.datarhei.com
// @contact.email hello@datarhei.com

// @license.name Apache 2.0
// @license.url https://github.com/datarhei/core/v16/blob/main/LICENSE

// @BasePath /

package cluster

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/datarhei/core/v16/cluster/client"
	"github.com/datarhei/core/v16/cluster/store"
	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/http/handler/util"
	httplog "github.com/datarhei/core/v16/http/log"
	mwlog "github.com/datarhei/core/v16/http/middleware/log"
	"github.com/datarhei/core/v16/http/validator"
	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/restream/app"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	echoSwagger "github.com/swaggo/echo-swagger" // echo-swagger middleware

	_ "github.com/datarhei/core/v16/cluster/docs"
)

type api struct {
	id        string
	address   string
	router    *echo.Echo
	cluster   *cluster
	logger    log.Logger
	startedAt time.Time
}

type API interface {
	Start() error
	Shutdown(ctx context.Context) error
}

type APIConfig struct {
	ID      string
	Cluster *cluster
	Logger  log.Logger
}

func NewAPI(config APIConfig) (API, error) {
	a := &api{
		id:        config.ID,
		cluster:   config.Cluster,
		logger:    config.Logger,
		startedAt: time.Now(),
	}

	if a.logger == nil {
		a.logger = log.New("")
	}

	address, err := config.Cluster.ClusterAPIAddress("")
	if err != nil {
		return nil, err
	}

	a.address = address

	a.router = echo.New()
	a.router.JSONSerializer = &GoJSONSerializer{}
	a.router.HTTPErrorHandler = ErrorHandler
	a.router.Validator = validator.New()
	a.router.HideBanner = true
	a.router.HidePort = true

	a.router.Use(mwlog.NewWithConfig(mwlog.Config{
		Logger: a.logger,
	}))
	a.router.Use(middleware.RecoverWithConfig(middleware.RecoverConfig{
		LogErrorFunc: func(c echo.Context, err error, stack []byte) error {
			rows := strings.Split(string(stack), "\n")
			a.logger.Error().WithField("stack", rows).Log("Recovered from a panic")
			return nil
		},
	}))
	a.router.Logger.SetOutput(httplog.NewWrapper(a.logger))

	// Swagger API documentation router group
	doc := a.router.Group("/v1/swagger/*")
	doc.GET("", echoSwagger.EchoWrapHandler(echoSwagger.InstanceName("ClusterAPI")))

	a.router.GET("/", a.Version)
	a.router.GET("/v1/about", a.About)

	a.router.GET("/v1/barrier/:name", a.Barrier)

	a.router.POST("/v1/server", a.AddServer)
	a.router.DELETE("/v1/server/:id", a.RemoveServer)

	a.router.PUT("/v1/transfer/:id", a.TransferLeadership)

	a.router.GET("/v1/snaphot", a.Snapshot)

	a.router.POST("/v1/process", a.ProcessAdd)
	a.router.GET("/v1/process/:id", a.ProcessGet)
	a.router.DELETE("/v1/process/:id", a.ProcessRemove)
	a.router.PUT("/v1/process/:id", a.ProcessUpdate)
	a.router.PUT("/v1/process/:id/command", a.ProcessSetCommand)
	a.router.PUT("/v1/process/:id/metadata/:key", a.ProcessSetMetadata)

	a.router.PUT("/v1/relocate", a.ProcessesRelocate)

	a.router.POST("/v1/iam/user", a.IAMIdentityAdd)
	a.router.PUT("/v1/iam/user/:name", a.IAMIdentityUpdate)
	a.router.PUT("/v1/iam/user/:name/policies", a.IAMPoliciesSet)
	a.router.DELETE("/v1/iam/user/:name", a.IAMIdentityRemove)

	a.router.POST("/v1/lock", a.LockCreate)
	a.router.DELETE("/v1/lock/:name", a.LockDelete)

	a.router.POST("/v1/kv", a.KVSet)
	a.router.GET("/v1/kv/:key", a.KVGet)
	a.router.DELETE("/v1/kv/:key", a.KVUnset)

	a.router.PUT("/v1/node/:id/state", a.NodeSetState)

	a.router.GET("/v1/core", a.CoreAPIAddress)
	a.router.GET("/v1/core/config", a.CoreConfig)
	a.router.GET("/v1/core/skills", a.CoreSkills)

	return a, nil
}

func (a *api) Start() error {
	a.logger.Debug().WithField("address", a.address).Log("Starting api")
	return a.router.Start(a.address)
}

func (a *api) Shutdown(ctx context.Context) error {
	a.logger.Debug().WithField("address", a.address).Log("Shutting down api")
	return a.router.Shutdown(ctx)
}

// Version returns the version of the cluster
// @Summary The cluster version
// @Description The cluster version
// @Tags v1.0.0
// @ID cluster-1-version
// @Produce json
// @Success 200 {string} string
// @Success 500 {object} Error
// @Router / [get]
func (a *api) Version(c echo.Context) error {
	return c.JSON(http.StatusOK, Version.String())
}

// About returns the version of the cluster
// @Summary The cluster version
// @Description The cluster version
// @Tags v1.0.0
// @ID cluster-1-about
// @Produce json
// @Success 200 {string} About
// @Success 500 {object} Error
// @Router /v1/about [get]
func (a *api) About(c echo.Context) error {
	resources, err := a.cluster.Resources()

	about := client.AboutResponse{
		ID:        a.id,
		Version:   Version.String(),
		Address:   a.cluster.Address(),
		StartedAt: a.startedAt,
		Resources: client.AboutResponseResources{
			IsThrottling: resources.CPU.Throttling,
			NCPU:         resources.CPU.NCPU,
			CPU:          (100 - resources.CPU.Idle) * resources.CPU.NCPU,
			CPULimit:     resources.CPU.Limit * resources.CPU.NCPU,
			Mem:          resources.Mem.Total - resources.Mem.Available,
			MemLimit:     resources.Mem.Total,
		},
	}

	if err != nil {
		about.Resources.Error = err.Error()
	}

	return c.JSON(http.StatusOK, about)
}

// Barrier returns if the barrier already has been passed
// @Summary Has the barrier already has been passed
// @Description Has the barrier already has been passed
// @Tags v1.0.0
// @ID cluster-1-barrier
// @Produce json
// @Param name path string true "Barrier name"
// @Success 200 {string} string
// @Success 404 {object} Error
// @Router /v1/barrier/{name} [get]
func (a *api) Barrier(c echo.Context) error {
	name := util.PathParam(c, "name")
	return c.JSON(http.StatusOK, a.cluster.GetBarrier(name))
}

// AddServer adds a new server to the cluster
// @Summary Add a new server
// @Description Add a new server to the cluster
// @Tags v1.0.0
// @ID cluster-1-add-server
// @Accept json
// @Produce json
// @Param config body client.JoinRequest true "Server ID and address"
// @Param X-Cluster-Origin header string false "Origin ID of request"
// @Success 200 {string} string
// @Failure 400 {object} Error
// @Failure 500 {object} Error
// @Failure 508 {object} Error
// @Router /v1/server [post]
func (a *api) AddServer(c echo.Context) error {
	r := client.JoinRequest{}

	if err := util.ShouldBindJSON(c, &r); err != nil {
		return Err(http.StatusBadRequest, "", "Invalid JSON: %s", err.Error())
	}

	a.logger.Debug().WithFields(log.Fields{
		"id":      r.ID,
		"request": r,
	}).Log("Join request: %+v", r)

	origin := c.Request().Header.Get("X-Cluster-Origin")

	if origin == a.id {
		return Err(http.StatusLoopDetected, "", "breaking circuit")
	}

	err := a.cluster.Join(origin, r.ID, r.RaftAddress, "")
	if err != nil {
		a.logger.Debug().WithError(err).WithField("id", r.ID).Log("Unable to join cluster")
		return ErrFromClusterError(err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// RemoveServer removes a server from the cluster
// @Summary Remove a server
// @Description Remove a server from the cluster
// @Tags v1.0.0
// @ID cluster-1-remove-server
// @Produce json
// @Param id path string true "Server ID"
// @Param X-Cluster-Origin header string false "Origin ID of request"
// @Success 200 {string} string
// @Failure 500 {object} Error
// @Failure 508 {object} Error
// @Router /v1/server/{id} [delete]
func (a *api) RemoveServer(c echo.Context) error {
	id := util.PathParam(c, "id")

	a.logger.Debug().WithFields(log.Fields{
		"id": id,
	}).Log("Leave request")

	origin := c.Request().Header.Get("X-Cluster-Origin")

	if origin == a.id {
		return Err(http.StatusLoopDetected, "", "breaking circuit")
	}

	err := a.cluster.Leave(origin, id)
	if err != nil {
		a.logger.Debug().WithError(err).WithField("id", id).Log("Unable to leave cluster")
		return ErrFromClusterError(err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// TransferLeadership transfers the leadership to another node
// @Summary Transfer leadership
// @Description Transfer leadership
// @Tags v1.0.0
// @ID cluster-1-transfer-leadership
// @Accept json
// @Produce json
// @Param X-Cluster-Origin header string false "Origin ID of request"
// @Success 200 {string} string
// @Failure 500 {object} Error
// @Failure 508 {object} Error
// @Router /v1/transfer/{id} [put]
func (a *api) TransferLeadership(c echo.Context) error {
	id := util.PathParam(c, "id")

	a.logger.Debug().WithFields(log.Fields{
		"id": id,
	}).Log("Transfer request")

	origin := c.Request().Header.Get("X-Cluster-Origin")

	if origin == a.id {
		return Err(http.StatusLoopDetected, "", "breaking circuit")
	}

	err := a.cluster.TransferLeadership(origin, id)
	if err != nil {
		a.logger.Debug().WithError(err).WithField("id", id).Log("Unable to transfer leadership")
		return ErrFromClusterError(err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// Snapshot returns a current snapshot of the cluster DB
// @Summary Cluster DB snapshot
// @Description Current snapshot of the clusterDB
// @Tags v1.0.0
// @ID cluster-1-snapshot
// @Produce application/octet-stream
// @Success 200 {file} byte
// @Success 500 {object} Error
// @Router /v1/snapshot [get]
func (a *api) Snapshot(c echo.Context) error {
	origin := c.Request().Header.Get("X-Cluster-Origin")

	if origin == a.id {
		return Err(http.StatusLoopDetected, "", "breaking circuit")
	}

	data, err := a.cluster.Snapshot(origin)
	if err != nil {
		a.logger.Debug().WithError(err).Log("Unable to create snaphot")
		return ErrFromClusterError(err)
	}

	defer data.Close()

	return c.Stream(http.StatusOK, "application/octet-stream", data)
}

// ProcessAdd adds a process to the cluster DB
// @Summary Add a process
// @Description Add a process to the cluster DB
// @Tags v1.0.0
// @ID cluster-1-add-process
// @Accept json
// @Produce json
// @Param config body client.AddProcessRequest true "Process config"
// @Param X-Cluster-Origin header string false "Origin ID of request"
// @Success 200 {string} string
// @Failure 400 {object} Error
// @Failure 500 {object} Error
// @Failure 508 {object} Error
// @Router /v1/process [post]
func (a *api) ProcessAdd(c echo.Context) error {
	r := client.AddProcessRequest{}

	if err := util.ShouldBindJSON(c, &r); err != nil {
		return Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	origin := c.Request().Header.Get("X-Cluster-Origin")

	if origin == a.id {
		return Err(http.StatusLoopDetected, "", "breaking circuit")
	}

	a.logger.Debug().WithField("id", r.Config.ID).Log("Add process request")

	err := a.cluster.ProcessAdd(origin, &r.Config)
	if err != nil {
		a.logger.Debug().WithError(err).WithField("id", r.Config.ID).Log("Unable to add process")
		return ErrFromClusterError(err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// ProcessGet gets a process from the cluster DB
// @Summary Get a process
// @Description Get a process from the cluster DB
// @Tags v1.0.0
// @ID cluster-1-get-process
// @Produce json
// @Param id path string true "Process ID"
// @Param domain query string false "Domain to act on"
// @Param X-Cluster-Origin header string false "Origin ID of request"
// @Success 200 {string} string
// @Failure 404 {object} Error
// @Failure 500 {object} Error
// @Failure 508 {object} Error
// @Router /v1/process/{id} [get]
func (a *api) ProcessGet(c echo.Context) error {
	id := util.PathParam(c, "id")
	domain := util.DefaultQuery(c, "domain", "")

	origin := c.Request().Header.Get("X-Cluster-Origin")

	if origin == a.id {
		return Err(http.StatusLoopDetected, "", "breaking circuit")
	}

	pid := app.ProcessID{ID: id, Domain: domain}

	process, nodeid, err := a.cluster.Store().ProcessGet(pid)
	if err != nil {
		return ErrFromClusterError(err)
	}

	res := client.GetProcessResponse{
		Process: process,
		NodeID:  nodeid,
	}

	return c.JSON(http.StatusOK, res)
}

// ProcessRemove removes a process from the cluster DB
// @Summary Remove a process
// @Description Remove a process from the cluster DB
// @Tags v1.0.0
// @ID cluster-1-remove-process
// @Produce json
// @Param id path string true "Process ID"
// @Param domain query string false "Domain to act on"
// @Param X-Cluster-Origin header string false "Origin ID of request"
// @Success 200 {string} string
// @Failure 500 {object} Error
// @Failure 508 {object} Error
// @Router /v1/process/{id} [delete]
func (a *api) ProcessRemove(c echo.Context) error {
	id := util.PathParam(c, "id")
	domain := util.DefaultQuery(c, "domain", "")

	origin := c.Request().Header.Get("X-Cluster-Origin")

	if origin == a.id {
		return Err(http.StatusLoopDetected, "", "breaking circuit")
	}

	pid := app.ProcessID{ID: id, Domain: domain}

	a.logger.Debug().WithField("id", pid).Log("Remove process request")

	err := a.cluster.ProcessRemove(origin, pid)
	if err != nil {
		a.logger.Debug().WithError(err).WithField("id", pid).Log("Unable to remove process")
		return ErrFromClusterError(err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// ProcessUpdate replaces an existing process in the cluster DB
// @Summary Replace an existing process
// @Description Replace an existing process in the cluster DB
// @Tags v1.0.0
// @ID cluster-1-update-process
// @Accept json
// @Produce json
// @Param id path string true "Process ID"
// @Param domain query string false "Domain to act on"
// @Param config body client.UpdateProcessRequest true "Process config"
// @Success 200 {string} string
// @Failure 500 {object} Error
// @Failure 508 {object} Error
// @Router /v1/process/{id} [put]
func (a *api) ProcessUpdate(c echo.Context) error {
	id := util.PathParam(c, "id")
	domain := util.DefaultQuery(c, "domain", "")

	r := client.UpdateProcessRequest{}

	if err := util.ShouldBindJSON(c, &r); err != nil {
		return Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	origin := c.Request().Header.Get("X-Cluster-Origin")

	if origin == a.id {
		return Err(http.StatusLoopDetected, "", "breaking circuit")
	}

	pid := app.ProcessID{ID: id, Domain: domain}

	a.logger.Debug().WithFields(log.Fields{
		"old_id": pid,
		"new_id": r.Config.ProcessID(),
	}).Log("Update process request")

	err := a.cluster.ProcessUpdate(origin, pid, &r.Config)
	if err != nil {
		a.logger.Debug().WithError(err).WithField("id", pid).Log("Unable to update process")
		return ErrFromClusterError(err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// ProcessSetCommand sets the order for a process
// @Summary Set the order for a process
// @Description Set the order for a process.
// @Tags v1.0.0
// @ID cluster-3-set-process-order
// @Produce json
// @Param id path string true "Process ID"
// @Param domain query string false "Domain to act on"
// @Param data body client.SetProcessCommandRequest true "Process order"
// @Success 200 {string} string
// @Failure 500 {object} Error
// @Failure 508 {object} Error
// @Router /v1/process/{id}/command [put]
func (a *api) ProcessSetCommand(c echo.Context) error {
	id := util.PathParam(c, "id")
	domain := util.DefaultQuery(c, "domain", "")

	r := client.SetProcessCommandRequest{}

	if err := util.ShouldBindJSON(c, &r); err != nil {
		return Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	origin := c.Request().Header.Get("X-Cluster-Origin")

	if origin == a.id {
		return Err(http.StatusLoopDetected, "", "breaking circuit")
	}

	pid := app.ProcessID{ID: id, Domain: domain}

	err := a.cluster.ProcessSetCommand(origin, pid, r.Command)
	if err != nil {
		a.logger.Debug().WithError(err).WithField("id", pid).Log("Unable to set order")
		return ErrFromClusterError(err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// ProcessSetMetadata stores metadata with a process
// @Summary Add JSON metadata with a process under the given key
// @Description Add arbitrary JSON metadata under the given key. If the key exists, all already stored metadata with this key will be overwritten. If the key doesn't exist, it will be created.
// @Tags v1.0.0
// @ID cluster-3-set-process-metadata
// @Produce json
// @Param id path string true "Process ID"
// @Param key path string true "Key for data store"
// @Param domain query string false "Domain to act on"
// @Param data body client.SetProcessMetadataRequest true "Arbitrary JSON data. The null value will remove the key and its contents"
// @Success 200 {string} string
// @Failure 500 {object} Error
// @Failure 508 {object} Error
// @Router /v1/process/{id}/metadata/{key} [put]
func (a *api) ProcessSetMetadata(c echo.Context) error {
	id := util.PathParam(c, "id")
	key := util.PathParam(c, "key")
	domain := util.DefaultQuery(c, "domain", "")

	r := client.SetProcessMetadataRequest{}

	if err := util.ShouldBindJSON(c, &r); err != nil {
		return Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	origin := c.Request().Header.Get("X-Cluster-Origin")

	if origin == a.id {
		return Err(http.StatusLoopDetected, "", "breaking circuit")
	}

	pid := app.ProcessID{ID: id, Domain: domain}

	err := a.cluster.ProcessSetMetadata(origin, pid, key, r.Metadata)
	if err != nil {
		a.logger.Debug().WithError(err).WithField("id", pid).Log("Unable to update metadata")
		return ErrFromClusterError(err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// ProcessesRelocate relocates processes to another node
// @Summary Relocate processes to another node
// @Description Relocate processes to another node.
// @Tags v1.0.0
// @ID cluster-3-relocate-processes
// @Produce json
// @Param data body client.RelocateProcessesRequest true "List of processes to relocate"
// @Success 200 {string} string
// @Failure 500 {object} Error
// @Failure 508 {object} Error
// @Router /v1/relocate [put]
func (a *api) ProcessesRelocate(c echo.Context) error {
	r := client.RelocateProcessesRequest{}

	if err := util.ShouldBindJSON(c, &r); err != nil {
		return Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	origin := c.Request().Header.Get("X-Cluster-Origin")

	if origin == a.id {
		return Err(http.StatusLoopDetected, "", "breaking circuit")
	}

	err := a.cluster.ProcessesRelocate(origin, r.Map)
	if err != nil {
		a.logger.Debug().WithError(err).Log("Unable to apply process relocation request")
		return ErrFromClusterError(err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// IAMIdentityAdd adds an identity to the cluster DB
// @Summary Add an identity
// @Description Add an identity to the cluster DB
// @Tags v1.0.0
// @ID cluster-1-add-identity
// @Accept json
// @Produce json
// @Param config body client.AddIdentityRequest true "Identity config"
// @Param X-Cluster-Origin header string false "Origin ID of request"
// @Success 200 {string} string
// @Failure 400 {object} Error
// @Failure 500 {object} Error
// @Failure 508 {object} Error
// @Router /v1/iam/user [post]
func (a *api) IAMIdentityAdd(c echo.Context) error {
	r := client.AddIdentityRequest{}

	if err := util.ShouldBindJSON(c, &r); err != nil {
		return Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	origin := c.Request().Header.Get("X-Cluster-Origin")

	if origin == a.id {
		return Err(http.StatusLoopDetected, "", "breaking circuit")
	}

	a.logger.Debug().WithField("identity", r.Identity).Log("Add identity request")

	err := a.cluster.IAMIdentityAdd(origin, r.Identity)
	if err != nil {
		a.logger.Debug().WithError(err).WithField("identity", r.Identity).Log("Unable to add identity")
		return ErrFromClusterError(err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// IAMIdentityUpdate replaces an existing identity in the cluster DB
// @Summary Replace an existing identity
// @Description Replace an existing identity in the cluster DB
// @Tags v1.0.0
// @ID cluster-1-update-identity
// @Accept json
// @Produce json
// @Param name path string true "Process ID"
// @Param config body client.UpdateIdentityRequest true "Identity config"
// @Success 200 {string} string
// @Failure 500 {object} Error
// @Failure 508 {object} Error
// @Router /v1/iam/user/{name} [put]
func (a *api) IAMIdentityUpdate(c echo.Context) error {
	name := util.PathParam(c, "name")

	r := client.UpdateIdentityRequest{}

	if err := util.ShouldBindJSON(c, &r); err != nil {
		return Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	origin := c.Request().Header.Get("X-Cluster-Origin")

	if origin == a.id {
		return Err(http.StatusLoopDetected, "", "breaking circuit")
	}

	a.logger.Debug().WithFields(log.Fields{
		"name":     name,
		"identity": r.Identity,
	}).Log("Update identity request")

	err := a.cluster.IAMIdentityUpdate(origin, name, r.Identity)
	if err != nil {
		a.logger.Debug().WithError(err).WithFields(log.Fields{
			"name":     name,
			"identity": r.Identity,
		}).Log("Unable to add identity")
		return ErrFromClusterError(err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// IAMPoliciesSet set policies for an identity in the cluster DB
// @Summary Set identity policies
// @Description Set policies for an identity in the cluster DB. Any existing policies will be replaced.
// @Tags v1.0.0
// @ID cluster-3-set-identity-policies
// @Produce json
// @Param id path string true "Process ID" SetPoliciesRequest
// @Param data body client.SetPoliciesRequest true "Policies for that user"
// @Success 200 {string} string
// @Failure 400 {object} Error
// @Failure 500 {object} Error
// @Failure 508 {object} Error
// @Router /v1/iam/user/{name}/policies [put]
func (a *api) IAMPoliciesSet(c echo.Context) error {
	name := util.PathParam(c, "name")

	r := client.SetPoliciesRequest{}

	if err := util.ShouldBindJSON(c, &r); err != nil {
		return Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	origin := c.Request().Header.Get("X-Cluster-Origin")

	if origin == a.id {
		return Err(http.StatusLoopDetected, "", "breaking circuit")
	}

	a.logger.Debug().WithField("policies", r.Policies).Log("Set policiesrequest")

	err := a.cluster.IAMPoliciesSet(origin, name, r.Policies)
	if err != nil {
		a.logger.Debug().WithError(err).WithField("policies", r.Policies).Log("Unable to set policies")
		return ErrFromClusterError(err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// IAMIdentityRemove removes an identity from the cluster DB
// @Summary Remove an identity
// @Description Remove an identity from the cluster DB
// @Tags v1.0.0
// @ID cluster-1-remove-identity
// @Produce json
// @Param name path string true "Identity name"
// @Param X-Cluster-Origin header string false "Origin ID of request"
// @Success 200 {string} string
// @Failure 500 {object} Error
// @Failure 508 {object} Error
// @Router /v1/iam/user/{name} [delete]
func (a *api) IAMIdentityRemove(c echo.Context) error {
	name := util.PathParam(c, "name")

	origin := c.Request().Header.Get("X-Cluster-Origin")

	if origin == a.id {
		return Err(http.StatusLoopDetected, "", "breaking circuit")
	}

	a.logger.Debug().WithField("identity", name).Log("Remove identity request")

	err := a.cluster.IAMIdentityRemove(origin, name)
	if err != nil {
		a.logger.Debug().WithError(err).WithField("identity", name).Log("Unable to remove identity")
		return ErrFromClusterError(err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// CoreAPIAddress returns the Core API address and login of this node
// @Summary Core API address and login
// @Description Core API address and login of this node
// @Tags v1.0.0
// @ID cluster-1-core-api-address
// @Produce json
// @Success 200 {string} string
// @Router /v1/core [get]
func (a *api) CoreAPIAddress(c echo.Context) error {
	address, _ := a.cluster.CoreAPIAddress("")
	return c.JSON(http.StatusOK, address)
}

// CoreConfig returns the Core config of this node
// @Summary Core config
// @Description Core config of this node
// @Tags v1.0.0
// @ID cluster-1-core-config
// @Produce json
// @Success 200 {object} config.Config
// @Router /v1/core/config [get]
func (a *api) CoreConfig(c echo.Context) error {
	config := a.cluster.CoreConfig()
	return c.JSON(http.StatusOK, config)
}

// CoreSkills returns the Core skills of this node
// @Summary Core skills
// @Description Core skills of this node
// @Tags v1.0.0
// @ID cluster-1-core-skills
// @Produce json
// @Success 200 {object} skills.Skills
// @Router /v1/core/skills [get]
func (a *api) CoreSkills(c echo.Context) error {
	skills := a.cluster.CoreSkills()
	return c.JSON(http.StatusOK, skills)
}

// LockCreate tries to acquire a named lock
// @Summary Acquire a named lock
// @Description Acquire a named lock
// @Tags v1.0.0
// @ID cluster-1-lock
// @Produce json
// @Param data body client.LockRequest true "LockCreate request"
// @Param X-Cluster-Origin header string false "Origin ID of request"
// @Success 200 {string} string
// @Failure 500 {object} Error
// @Failure 508 {object} Error
// @Router /v1/lock [post]
func (a *api) LockCreate(c echo.Context) error {
	r := client.LockRequest{}

	if err := util.ShouldBindJSON(c, &r); err != nil {
		return Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	origin := c.Request().Header.Get("X-Cluster-Origin")

	if origin == a.id {
		return Err(http.StatusLoopDetected, "", "breaking circuit")
	}

	a.logger.Debug().WithField("name", r.Name).Log("Acquire lock")

	_, err := a.cluster.LockCreate(origin, r.Name, r.ValidUntil)
	if err != nil {
		a.logger.Debug().WithError(err).WithField("name", r.Name).Log("Unable to acquire lock")
		return ErrFromClusterError(err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// LockDelete removes a named lock
// @Summary Remove a lock
// @Description Remove a lock
// @Tags v1.0.0
// @ID cluster-1-unlock
// @Produce json
// @Param name path string true "Lock name"
// @Param X-Cluster-Origin header string false "Origin ID of request"
// @Success 200 {string} string
// @Failure 404 {object} Error
// @Failure 500 {object} Error
// @Failure 508 {object} Error
// @Router /v1/lock/{name} [delete]
func (a *api) LockDelete(c echo.Context) error {
	name := util.PathParam(c, "name")

	origin := c.Request().Header.Get("X-Cluster-Origin")

	if origin == a.id {
		return Err(http.StatusLoopDetected, "", "breaking circuit")
	}

	a.logger.Debug().WithField("name", name).Log("Remove lock request")

	err := a.cluster.LockDelete(origin, name)
	if err != nil {
		a.logger.Debug().WithError(err).WithField("name", name).Log("Unable to remove lock")
		return ErrFromClusterError(err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// KVSet stores the value under key
// @Summary Store value under key
// @Description Store value under key
// @Tags v1.0.0
// @ID cluster-1-kv-set
// @Produce json
// @Param data body client.SetKVRequest true "Set KV request"
// @Param X-Cluster-Origin header string false "Origin ID of request"
// @Success 200 {string} string
// @Failure 500 {object} Error
// @Failure 508 {object} Error
// @Router /v1/kv [post]
func (a *api) KVSet(c echo.Context) error {
	r := client.SetKVRequest{}

	if err := util.ShouldBindJSON(c, &r); err != nil {
		return Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	origin := c.Request().Header.Get("X-Cluster-Origin")

	if origin == a.id {
		return Err(http.StatusLoopDetected, "", "breaking circuit")
	}

	a.logger.Debug().WithField("key", r.Key).Log("Store value")

	err := a.cluster.KVSet(origin, r.Key, r.Value)
	if err != nil {
		a.logger.Debug().WithError(err).WithField("key", r.Key).Log("Unable to store value")
		return ErrFromClusterError(err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// KVUnset removes a key
// @Summary Remove a key
// @Description Remove a key
// @Tags v1.0.0
// @ID cluster-1-kv-unset
// @Produce json
// @Param name path string true "Key name"
// @Param X-Cluster-Origin header string false "Origin ID of request"
// @Success 200 {string} string
// @Failure 404 {object} Error
// @Failure 500 {object} Error
// @Failure 508 {object} Error
// @Router /v1/kv/{key} [delete]
func (a *api) KVUnset(c echo.Context) error {
	key := util.PathParam(c, "key")

	origin := c.Request().Header.Get("X-Cluster-Origin")

	if origin == a.id {
		return Err(http.StatusLoopDetected, "", "breaking circuit")
	}

	a.logger.Debug().WithField("key", key).Log("Delete key")

	err := a.cluster.KVUnset(origin, key)
	if err != nil {
		a.logger.Debug().WithError(err).WithField("key", key).Log("Unable to remove key")
		return ErrFromClusterError(err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// KVGet fetches a key
// @Summary Fetch a key
// @Description Fetch a key
// @Tags v1.0.0
// @ID cluster-1-kv-get
// @Produce json
// @Param name path string true "Key name"
// @Param X-Cluster-Origin header string false "Origin ID of request"
// @Success 200 {string} string
// @Failure 404 {object} Error
// @Failure 500 {object} Error
// @Failure 508 {object} Error
// @Router /v1/kv/{key} [get]
func (a *api) KVGet(c echo.Context) error {
	key := util.PathParam(c, "key")

	origin := c.Request().Header.Get("X-Cluster-Origin")

	if origin == a.id {
		return Err(http.StatusLoopDetected, "", "breaking circuit")
	}

	a.logger.Debug().WithField("key", key).Log("Get key")

	value, updatedAt, err := a.cluster.KVGet(origin, key, false)
	if err != nil {
		a.logger.Debug().WithError(err).WithField("key", key).Log("Unable to retrieve key")
		return ErrFromClusterError(err)
	}

	res := client.GetKVResponse{
		Value:     value,
		UpdatedAt: updatedAt,
	}

	return c.JSON(http.StatusOK, res)
}

// NodeSetState sets a state for a node
// @Summary Set a state for a node
// @Description Set a state for a node
// @Tags v1.0.0
// @ID cluster-1-node-set-state
// @Produce json
// @Param data body client.SetNodeStateRequest true "Set node state request"
// @Param X-Cluster-Origin header string false "Origin ID of request"
// @Success 200 {string} string
// @Failure 400 {object} Error
// @Failure 500 {object} Error
// @Failure 508 {object} Error
// @Router /v1/node/{id}/state [get]
func (a *api) NodeSetState(c echo.Context) error {
	nodeid := util.PathParam(c, "id")

	r := client.SetNodeStateRequest{}

	if err := util.ShouldBindJSON(c, &r); err != nil {
		return Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	origin := c.Request().Header.Get("X-Cluster-Origin")

	if origin == a.id {
		return Err(http.StatusLoopDetected, "", "breaking circuit")
	}

	a.logger.Debug().WithFields(log.Fields{
		"node":  nodeid,
		"state": r.State,
	}).Log("Set node state")

	err := a.cluster.NodeSetState(origin, nodeid, r.State)
	if err != nil {
		a.logger.Debug().WithError(err).WithFields(log.Fields{
			"node":  nodeid,
			"state": r.State,
		}).Log("Unable to set state")

		if errors.Is(err, ErrUnsupportedNodeState) {
			return Err(http.StatusBadRequest, "", "%s: %s", err.Error(), r.State)
		}
		return ErrFromClusterError(err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// Error represents an error response of the API
type Error struct {
	Code    int      `json:"code" jsonschema:"required" format:"int"`
	Message string   `json:"message" jsonschema:""`
	Details []string `json:"details" jsonschema:""`
}

// Error returns the string representation of the error
func (e Error) Error() string {
	return fmt.Sprintf("code=%d, message=%s, details=%s", e.Code, e.Message, strings.Join(e.Details, " "))
}

// Err creates a new API error with the given HTTP status code. If message is empty, the default message
// for the given code is used. If the first entry in args is a string, it is interpreted as a format string
// for the remaining entries in args, that is used for fmt.Sprintf. Otherwise the args are ignored.
func Err(code int, message string, args ...interface{}) Error {
	if len(message) == 0 {
		message = http.StatusText(code)
	}

	e := Error{
		Code:    code,
		Message: message,
		Details: []string{},
	}

	if len(args) >= 1 {
		if format, ok := args[0].(string); ok {
			e.Details = strings.Split(fmt.Sprintf(format, args[1:]...), "\n")
		}
	}

	return e
}

func ErrFromClusterError(err error) Error {
	status := http.StatusInternalServerError
	if errors.Is(err, store.ErrNotFound) {
		status = http.StatusNotFound
	} else if errors.Is(err, store.ErrBadRequest) {
		status = http.StatusBadRequest
	}

	return Err(status, "", "%s", err.Error())
}

// ErrorHandler is a genral handler for echo handler errors
func ErrorHandler(err error, c echo.Context) {
	var code int = 0
	var details []string
	message := ""

	if he, ok := err.(Error); ok {
		code = he.Code
		message = he.Message
		details = he.Details
	} else if he, ok := err.(*echo.HTTPError); ok {
		if he.Internal != nil {
			if herr, ok := he.Internal.(*echo.HTTPError); ok {
				he = herr
			}
		}

		code = he.Code
		message = http.StatusText(he.Code)
		if len(message) == 0 {
			switch code {
			case 509:
				message = "Bandwith limit exceeded"
			default:
			}
		}
		details = strings.Split(fmt.Sprintf("%v", he.Message), "\n")
	} else {
		code = http.StatusInternalServerError
		message = http.StatusText(http.StatusInternalServerError)
		details = strings.Split(err.Error(), "\n")
	}

	// Send response
	if !c.Response().Committed {
		if c.Request().Method == http.MethodHead {
			c.NoContent(code)
		} else {
			c.JSON(code, Error{
				Code:    code,
				Message: message,
				Details: details,
			})
		}
	}
}

// GoJSONSerializer implements JSON encoding using encoding/json.
type GoJSONSerializer struct{}

// Serialize converts an interface into a json and writes it to the response.
// You can optionally use the indent parameter to produce pretty JSONs.
func (d GoJSONSerializer) Serialize(c echo.Context, i interface{}, indent string) error {
	enc := json.NewEncoder(c.Response())
	if indent != "" {
		enc.SetIndent("", indent)
	}
	return enc.Encode(i)
}

// Deserialize reads a JSON from a request body and converts it into an interface.
func (d GoJSONSerializer) Deserialize(c echo.Context, i interface{}) error {
	err := json.NewDecoder(c.Request().Body).Decode(i)
	if ute, ok := err.(*json.UnmarshalTypeError); ok {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Unmarshal type error: expected=%v, got=%v, field=%v, offset=%v", ute.Type, ute.Value, ute.Field, ute.Offset)).SetInternal(err)
	} else if se, ok := err.(*json.SyntaxError); ok {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Syntax error: offset=%v, error=%v", se.Offset, se.Error())).SetInternal(err)
	}
	return err
}
