package cluster

import (
	"bytes"
	"context"
	"net/http"
	"strings"

	httpapi "github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/errorhandler"
	"github.com/datarhei/core/v16/http/handler/util"
	httplog "github.com/datarhei/core/v16/http/log"
	mwlog "github.com/datarhei/core/v16/http/middleware/log"
	"github.com/datarhei/core/v16/http/validator"
	"github.com/datarhei/core/v16/log"

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

type JoinRequest struct {
	Origin      string `json:"origin"`
	ID          string `json:"id"`
	RaftAddress string `json:"raft_address"`
	APIAddress  string `json:"api_address"`
	APIUsername string `json:"api_username"`
	APIPassword string `json:"api_password"`
}

type LeaveRequest struct {
	Origin string `json:"origin"`
	ID     string `json:"id"`
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

	address, err := config.Cluster.APIAddr("")
	if err != nil {
		return nil, err
	}

	a.address = address

	a.router = echo.New()
	a.router.Debug = true
	a.router.HTTPErrorHandler = errorhandler.HTTPErrorHandler
	a.router.Validator = validator.New()
	a.router.HideBanner = false
	a.router.HidePort = false

	mwlog.NewWithConfig(mwlog.Config{
		Logger: a.logger,
	})

	a.router.Use(mwlog.NewWithConfig(mwlog.Config{
		Logger: a.logger,
	}))
	a.router.Use(middleware.RecoverWithConfig(middleware.RecoverConfig{
		LogErrorFunc: func(c echo.Context, err error, stack []byte) error {
			rows := strings.Split(string(stack), "\n")
			a.logger.Error().WithField("stack", rows).Log("recovered from a panic")
			return nil
		},
	}))
	a.router.Logger.SetOutput(httplog.NewWrapper(a.logger))

	a.router.POST("/v1/join", func(c echo.Context) error {
		r := JoinRequest{}

		if err := util.ShouldBindJSON(c, &r); err != nil {
			return httpapi.Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
		}

		a.logger.Debug().Log("got join request: %+v", r)

		if r.Origin == a.id {
			return httpapi.Err(http.StatusLoopDetected, "", "breaking circuit")
		}

		err := a.cluster.Join(r.Origin, r.ID, r.RaftAddress, r.APIAddress, r.APIUsername, r.APIPassword)
		if err != nil {
			a.logger.Debug().WithError(err).Log("unable to join cluster")
			return httpapi.Err(http.StatusInternalServerError, "unable to join cluster", "%s", err)
		}

		return c.JSON(http.StatusOK, "OK")
	})

	a.router.POST("/v1/leave", func(c echo.Context) error {
		r := LeaveRequest{}

		if err := util.ShouldBindJSON(c, &r); err != nil {
			return httpapi.Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
		}

		a.logger.Debug().Log("got leave request: %+v", r)

		if r.Origin == a.id {
			return httpapi.Err(http.StatusLoopDetected, "", "breaking circuit")
		}

		err := a.cluster.Leave(r.Origin, r.ID)
		if err != nil {
			a.logger.Debug().WithError(err).Log("unable to leave cluster")
			return httpapi.Err(http.StatusInternalServerError, "unable to leave cluster", "%s", err)
		}

		return c.JSON(http.StatusOK, "OK")
	})

	a.router.GET("/v1/snaphot", func(c echo.Context) error {
		data, err := a.cluster.Snapshot()
		if err != nil {
			a.logger.Debug().WithError(err).Log("unable to create snaphot")
			return httpapi.Err(http.StatusInternalServerError, "unable to create snapshot", "%s", err)
		}

		return c.Stream(http.StatusOK, "application/octet-stream", bytes.NewReader(data))
	})

	a.router.POST("/v1/process", func(c echo.Context) error {
		return httpapi.Err(http.StatusNotImplemented, "")
	})

	a.router.DELETE("/v1/process/:id", func(c echo.Context) error {
		return httpapi.Err(http.StatusNotImplemented, "")
	})

	return a, nil
}

func (a *api) Start() error {
	a.logger.Debug().Log("starting api at %s", a.address)
	return a.router.Start(a.address)
}

func (a *api) Shutdown(ctx context.Context) error {
	return a.router.Shutdown(ctx)
}
