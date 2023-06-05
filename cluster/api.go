package cluster

import (
	"context"
	"net/http"
	"strings"

	"github.com/datarhei/core/v16/cluster/client"
	httpapi "github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/errorhandler"
	"github.com/datarhei/core/v16/http/handler/util"
	httplog "github.com/datarhei/core/v16/http/log"
	mwlog "github.com/datarhei/core/v16/http/middleware/log"
	"github.com/datarhei/core/v16/http/validator"
	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/restream/app"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
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

	a.router.POST("/v1/server", func(c echo.Context) error {
		r := client.JoinRequest{}

		if err := util.ShouldBindJSON(c, &r); err != nil {
			return httpapi.Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
		}

		a.logger.Debug().WithFields(log.Fields{
			"id":      r.ID,
			"request": r,
		}).Log("Join request: %+v", r)

		origin := c.Request().Header.Get("X-Cluster-Origin")

		if origin == a.id {
			return httpapi.Err(http.StatusLoopDetected, "", "breaking circuit")
		}

		err := a.cluster.Join(origin, r.ID, r.RaftAddress, "")
		if err != nil {
			a.logger.Debug().WithError(err).WithField("id", r.ID).Log("Unable to join cluster")
			return httpapi.Err(http.StatusInternalServerError, "unable to join cluster", "%s", err)
		}

		return c.JSON(http.StatusOK, "OK")
	})

	a.router.DELETE("/v1/server/:id", func(c echo.Context) error {
		id := util.PathParam(c, "id")

		a.logger.Debug().WithFields(log.Fields{
			"id": id,
		}).Log("Leave request")

		origin := c.Request().Header.Get("X-Cluster-Origin")

		if origin == a.id {
			return httpapi.Err(http.StatusLoopDetected, "", "breaking circuit")
		}

		err := a.cluster.Leave(origin, id)
		if err != nil {
			a.logger.Debug().WithError(err).WithField("id", id).Log("Unable to leave cluster")
			return httpapi.Err(http.StatusInternalServerError, "unable to leave cluster", "%s", err)
		}

		return c.JSON(http.StatusOK, "OK")
	})

	a.router.GET("/v1/snaphot", func(c echo.Context) error {
		data, err := a.cluster.Snapshot()
		if err != nil {
			a.logger.Debug().WithError(err).Log("Unable to create snaphot")
			return httpapi.Err(http.StatusInternalServerError, "unable to create snapshot", "%s", err)
		}

		defer data.Close()

		return c.Stream(http.StatusOK, "application/octet-stream", data)
	})

	a.router.POST("/v1/process", func(c echo.Context) error {
		r := client.AddProcessRequest{}

		if err := util.ShouldBindJSON(c, &r); err != nil {
			return httpapi.Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
		}

		origin := c.Request().Header.Get("X-Cluster-Origin")

		if origin == a.id {
			return httpapi.Err(http.StatusLoopDetected, "", "breaking circuit")
		}

		a.logger.Debug().WithField("id", r.Config.ID).Log("Add process request")

		err := a.cluster.AddProcess(origin, &r.Config)
		if err != nil {
			a.logger.Debug().WithError(err).WithField("id", r.Config.ID).Log("Unable to add process")
			return httpapi.Err(http.StatusInternalServerError, "unable to add process", "%s", err)
		}

		return c.JSON(http.StatusOK, "OK")
	})

	a.router.DELETE("/v1/process/:id", func(c echo.Context) error {
		id := util.PathParam(c, "id")
		domain := util.DefaultQuery(c, "domain", "")

		origin := c.Request().Header.Get("X-Cluster-Origin")

		if origin == a.id {
			return httpapi.Err(http.StatusLoopDetected, "", "breaking circuit")
		}

		pid := app.ProcessID{ID: id, Domain: domain}

		a.logger.Debug().WithField("id", pid).Log("Remove process request")

		err := a.cluster.RemoveProcess(origin, pid)
		if err != nil {
			a.logger.Debug().WithError(err).WithField("id", pid).Log("Unable to remove process")
			return httpapi.Err(http.StatusInternalServerError, "unable to remove process", "%s", err)
		}

		return c.JSON(http.StatusOK, "OK")
	})

	a.router.PUT("/v1/process/:id", func(c echo.Context) error {
		id := util.PathParam(c, "id")
		domain := util.DefaultQuery(c, "domain", "")

		r := client.UpdateProcessRequest{}

		if err := util.ShouldBindJSON(c, &r); err != nil {
			return httpapi.Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
		}

		origin := c.Request().Header.Get("X-Cluster-Origin")

		if origin == a.id {
			return httpapi.Err(http.StatusLoopDetected, "", "breaking circuit")
		}

		pid := app.ProcessID{ID: id, Domain: domain}

		if !pid.Equals(r.ID) {
			return httpapi.Err(http.StatusBadRequest, "Invalid data", "the ID in the path and the request do not match")
		}

		a.logger.Debug().WithFields(log.Fields{
			"old_id": r.ID,
			"new_id": r.Config.ProcessID(),
		}).Log("Update process request")

		err := a.cluster.UpdateProcess(origin, r.ID, &r.Config)
		if err != nil {
			a.logger.Debug().WithError(err).WithField("id", r.ID).Log("Unable to update process")
			return httpapi.Err(http.StatusInternalServerError, "unable to update process", "%s", err)
		}

		return c.JSON(http.StatusOK, "OK")
	})

	a.router.PUT("/v1/process/:id/metadata/:key", func(c echo.Context) error {
		id := util.PathParam(c, "id")
		key := util.PathParam(c, "key")
		domain := util.DefaultQuery(c, "domain", "")

		r := client.SetProcessMetadataRequest{}

		if err := util.ShouldBindJSON(c, &r); err != nil {
			return httpapi.Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
		}

		origin := c.Request().Header.Get("X-Cluster-Origin")

		if origin == a.id {
			return httpapi.Err(http.StatusLoopDetected, "", "breaking circuit")
		}

		pid := app.ProcessID{ID: id, Domain: domain}

		err := a.cluster.SetProcessMetadata(origin, pid, key, r.Metadata)
		if err != nil {
			a.logger.Debug().WithError(err).WithField("id", r.ID).Log("Unable to update metadata")
			return httpapi.Err(http.StatusInternalServerError, "unable to update metadata", "%s", err)
		}

		return c.JSON(http.StatusOK, "OK")
	})

	a.router.POST("/v1/iam/user", func(c echo.Context) error {
		r := client.AddIdentityRequest{}

		if err := util.ShouldBindJSON(c, &r); err != nil {
			return httpapi.Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
		}

		origin := c.Request().Header.Get("X-Cluster-Origin")

		if origin == a.id {
			return httpapi.Err(http.StatusLoopDetected, "", "breaking circuit")
		}

		a.logger.Debug().WithField("identity", r.Identity).Log("Add identity request")

		err := a.cluster.AddIdentity(origin, r.Identity)
		if err != nil {
			a.logger.Debug().WithError(err).WithField("identity", r.Identity).Log("Unable to add identity")
			return httpapi.Err(http.StatusInternalServerError, "unable to add identity", "%s", err)
		}

		return c.JSON(http.StatusOK, "OK")
	})

	a.router.PUT("/v1/iam/user/:name", func(c echo.Context) error {
		name := util.PathParam(c, "name")

		r := client.UpdateIdentityRequest{}

		if err := util.ShouldBindJSON(c, &r); err != nil {
			return httpapi.Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
		}

		origin := c.Request().Header.Get("X-Cluster-Origin")

		if origin == a.id {
			return httpapi.Err(http.StatusLoopDetected, "", "breaking circuit")
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
			return httpapi.Err(http.StatusInternalServerError, "unable to update identity", "%s", err)
		}

		return c.JSON(http.StatusOK, "OK")
	})

	a.router.PUT("/v1/iam/user/:name/policies", func(c echo.Context) error {
		name := util.PathParam(c, "name")

		r := client.SetPoliciesRequest{}

		if err := util.ShouldBindJSON(c, &r); err != nil {
			return httpapi.Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
		}

		origin := c.Request().Header.Get("X-Cluster-Origin")

		if origin == a.id {
			return httpapi.Err(http.StatusLoopDetected, "", "breaking circuit")
		}

		a.logger.Debug().WithField("policies", r.Policies).Log("Set policiesrequest")

		err = a.cluster.SetPolicies(origin, name, r.Policies)
		if err != nil {
			a.logger.Debug().WithError(err).WithField("policies", r.Policies).Log("Unable to set policies")
			return httpapi.Err(http.StatusInternalServerError, "unable to add identity", "%s", err)
		}

		return c.JSON(http.StatusOK, "OK")
	})

	a.router.DELETE("/v1/iam/user/:name", func(c echo.Context) error {
		name := util.PathParam(c, "name")

		origin := c.Request().Header.Get("X-Cluster-Origin")

		if origin == a.id {
			return httpapi.Err(http.StatusLoopDetected, "", "breaking circuit")
		}

		a.logger.Debug().WithField("identity", name).Log("Remove identity request")

		err := a.cluster.RemoveIdentity(origin, name)
		if err != nil {
			a.logger.Debug().WithError(err).WithField("identity", name).Log("Unable to remove identity")
			return httpapi.Err(http.StatusInternalServerError, "unable to remove identity", "%s", err)
		}

		return c.JSON(http.StatusOK, "OK")
	})

	a.router.GET("/v1/core", func(c echo.Context) error {
		address, _ := a.cluster.CoreAPIAddress("")
		return c.JSON(http.StatusOK, address)
	})

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
