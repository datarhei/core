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
	"fmt"
	"net/http"
	"strings"

	"github.com/datarhei/core/v16/cluster/client"
	"github.com/datarhei/core/v16/http/errorhandler"
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
	id      string
	address string
	router  *echo.Echo
	cluster Cluster
	logger  log.Logger
}

type API interface {
	Start() error
	Shutdown(ctx context.Context) error
}

type APIConfig struct {
	ID      string
	Cluster Cluster
	Logger  log.Logger
}

func NewAPI(config APIConfig) (API, error) {
	a := &api{
		id:      config.ID,
		cluster: config.Cluster,
		logger:  config.Logger,
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
	a.router.Debug = true
	a.router.HTTPErrorHandler = errorhandler.HTTPErrorHandler
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

	a.router.GET("/", func(c echo.Context) error {
		return c.JSON(http.StatusOK, Version.String())
	})

	a.router.GET("/v1/about", func(c echo.Context) error {
		return c.JSON(http.StatusOK, Version.String())
	})

	a.router.POST("/v1/server", a.AddServer)
	a.router.DELETE("/v1/server/:id", a.RemoveServer)

	a.router.GET("/v1/snaphot", a.Snapshot)

	a.router.POST("/v1/process", a.AddProcess)
	a.router.DELETE("/v1/process/:id", a.RemoveProcess)
	a.router.PUT("/v1/process/:id", a.UpdateProcess)
	a.router.PUT("/v1/process/:id/metadata/:key", a.SetProcessMetadata)

	a.router.POST("/v1/iam/user", a.AddIdentity)
	a.router.PUT("/v1/iam/user/:name", a.UpdateIdentity)
	a.router.PUT("/v1/iam/user/:name/policies", a.SetIdentityPolicies)
	a.router.DELETE("/v1/iam/user/:name", a.RemoveIdentity)

	a.router.GET("/v1/core", a.CoreAPIAddress)

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
		return Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
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
		return Err(http.StatusInternalServerError, "unable to join cluster", "%s", err)
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
		return Err(http.StatusInternalServerError, "unable to leave cluster", "%s", err)
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
// @Success 500 {array} Error
// @Router /v1/snapshot [get]
func (a *api) Snapshot(c echo.Context) error {
	data, err := a.cluster.Snapshot()
	if err != nil {
		a.logger.Debug().WithError(err).Log("Unable to create snaphot")
		return Err(http.StatusInternalServerError, "unable to create snapshot", "%s", err)
	}

	defer data.Close()

	return c.Stream(http.StatusOK, "application/octet-stream", data)
}

// AddProcess adds a process to the cluster DB
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
func (a *api) AddProcess(c echo.Context) error {
	r := client.AddProcessRequest{}

	if err := util.ShouldBindJSON(c, &r); err != nil {
		return Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
	}

	origin := c.Request().Header.Get("X-Cluster-Origin")

	if origin == a.id {
		return Err(http.StatusLoopDetected, "", "breaking circuit")
	}

	a.logger.Debug().WithField("id", r.Config.ID).Log("Add process request")

	err := a.cluster.AddProcess(origin, &r.Config)
	if err != nil {
		a.logger.Debug().WithError(err).WithField("id", r.Config.ID).Log("Unable to add process")
		return Err(http.StatusInternalServerError, "unable to add process", "%s", err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// RemoveProcess removes a process from the cluster DB
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
func (a *api) RemoveProcess(c echo.Context) error {
	id := util.PathParam(c, "id")
	domain := util.DefaultQuery(c, "domain", "")

	origin := c.Request().Header.Get("X-Cluster-Origin")

	if origin == a.id {
		return Err(http.StatusLoopDetected, "", "breaking circuit")
	}

	pid := app.ProcessID{ID: id, Domain: domain}

	a.logger.Debug().WithField("id", pid).Log("Remove process request")

	err := a.cluster.RemoveProcess(origin, pid)
	if err != nil {
		a.logger.Debug().WithError(err).WithField("id", pid).Log("Unable to remove process")
		return Err(http.StatusInternalServerError, "unable to remove process", "%s", err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// UpdateProcess replaces an existing process in the cluster DB
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
func (a *api) UpdateProcess(c echo.Context) error {
	id := util.PathParam(c, "id")
	domain := util.DefaultQuery(c, "domain", "")

	r := client.UpdateProcessRequest{}

	if err := util.ShouldBindJSON(c, &r); err != nil {
		return Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
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

	err := a.cluster.UpdateProcess(origin, pid, &r.Config)
	if err != nil {
		a.logger.Debug().WithError(err).WithField("id", pid).Log("Unable to update process")
		return Err(http.StatusInternalServerError, "unable to update process", "%s", err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// SetProcessMetadata stores metadata with a process
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
func (a *api) SetProcessMetadata(c echo.Context) error {
	id := util.PathParam(c, "id")
	key := util.PathParam(c, "key")
	domain := util.DefaultQuery(c, "domain", "")

	r := client.SetProcessMetadataRequest{}

	if err := util.ShouldBindJSON(c, &r); err != nil {
		return Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
	}

	origin := c.Request().Header.Get("X-Cluster-Origin")

	if origin == a.id {
		return Err(http.StatusLoopDetected, "", "breaking circuit")
	}

	pid := app.ProcessID{ID: id, Domain: domain}

	err := a.cluster.SetProcessMetadata(origin, pid, key, r.Metadata)
	if err != nil {
		a.logger.Debug().WithError(err).WithField("id", pid).Log("Unable to update metadata")
		return Err(http.StatusInternalServerError, "unable to update metadata", "%s", err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// AddIdentity adds an identity to the cluster DB
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
func (a *api) AddIdentity(c echo.Context) error {
	r := client.AddIdentityRequest{}

	if err := util.ShouldBindJSON(c, &r); err != nil {
		return Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
	}

	origin := c.Request().Header.Get("X-Cluster-Origin")

	if origin == a.id {
		return Err(http.StatusLoopDetected, "", "breaking circuit")
	}

	a.logger.Debug().WithField("identity", r.Identity).Log("Add identity request")

	err := a.cluster.AddIdentity(origin, r.Identity)
	if err != nil {
		a.logger.Debug().WithError(err).WithField("identity", r.Identity).Log("Unable to add identity")
		return Err(http.StatusInternalServerError, "unable to add identity", "%s", err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// UpdateIdentity replaces an existing identity in the cluster DB
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
func (a *api) UpdateIdentity(c echo.Context) error {
	name := util.PathParam(c, "name")

	r := client.UpdateIdentityRequest{}

	if err := util.ShouldBindJSON(c, &r); err != nil {
		return Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
	}

	origin := c.Request().Header.Get("X-Cluster-Origin")

	if origin == a.id {
		return Err(http.StatusLoopDetected, "", "breaking circuit")
	}

	a.logger.Debug().WithFields(log.Fields{
		"name":     name,
		"identity": r.Identity,
	}).Log("Update identity request")

	err := a.cluster.UpdateIdentity(origin, name, r.Identity)
	if err != nil {
		a.logger.Debug().WithError(err).WithFields(log.Fields{
			"name":     name,
			"identity": r.Identity,
		}).Log("Unable to add identity")
		return Err(http.StatusInternalServerError, "unable to update identity", "%s", err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// SetIdentityPolicies set policies for an identity in the cluster DB
// @Summary Set identity policies
// @Description Set policies for an identity in the cluster DB. Any existing policies will be replaced.
// @Tags v1.0.0
// @ID cluster-3-set-identity-policies
// @Produce json
// @Param id path string true "Process ID"SetPoliciesRequest
// @Param data body client.SetPoliciesRequest true "Policies for that user"
// @Success 200 {string} string
// @Failure 400 {object} Error
// @Failure 500 {object} Error
// @Failure 508 {object} Error
// @Router /v1/iam/user/{name}/policies [put]
func (a *api) SetIdentityPolicies(c echo.Context) error {
	name := util.PathParam(c, "name")

	r := client.SetPoliciesRequest{}

	if err := util.ShouldBindJSON(c, &r); err != nil {
		return Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
	}

	origin := c.Request().Header.Get("X-Cluster-Origin")

	if origin == a.id {
		return Err(http.StatusLoopDetected, "", "breaking circuit")
	}

	a.logger.Debug().WithField("policies", r.Policies).Log("Set policiesrequest")

	err := a.cluster.SetPolicies(origin, name, r.Policies)
	if err != nil {
		a.logger.Debug().WithError(err).WithField("policies", r.Policies).Log("Unable to set policies")
		return Err(http.StatusInternalServerError, "unable to add identity", "%s", err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// RemoveIdentity removes an identity from the cluster DB
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
func (a *api) RemoveIdentity(c echo.Context) error {
	name := util.PathParam(c, "name")

	origin := c.Request().Header.Get("X-Cluster-Origin")

	if origin == a.id {
		return Err(http.StatusLoopDetected, "", "breaking circuit")
	}

	a.logger.Debug().WithField("identity", name).Log("Remove identity request")

	err := a.cluster.RemoveIdentity(origin, name)
	if err != nil {
		a.logger.Debug().WithError(err).WithField("identity", name).Log("Unable to remove identity")
		return Err(http.StatusInternalServerError, "unable to remove identity", "%s", err)
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
// @Success 500 {array} Error
// @Router /v1/core [get]
func (a *api) CoreAPIAddress(c echo.Context) error {
	address, _ := a.cluster.CoreAPIAddress("")
	return c.JSON(http.StatusOK, address)
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
