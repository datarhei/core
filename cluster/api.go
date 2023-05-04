package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

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

		a.logger.Debug().WithField("id", r.ID).Log("got join request: %+v", r)

		if r.Origin == a.id {
			return httpapi.Err(http.StatusLoopDetected, "", "breaking circuit")
		}

		err := a.cluster.Join(r.Origin, r.ID, r.RaftAddress, r.APIAddress, "")
		if err != nil {
			a.logger.Debug().WithError(err).WithField("id", r.ID).Log("unable to join cluster")
			return httpapi.Err(http.StatusInternalServerError, "unable to join cluster", "%s", err)
		}

		return c.JSON(http.StatusOK, "OK")
	})

	a.router.POST("/v1/leave", func(c echo.Context) error {
		r := LeaveRequest{}

		if err := util.ShouldBindJSON(c, &r); err != nil {
			return httpapi.Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
		}

		a.logger.Debug().WithField("id", r.ID).Log("got leave request: %+v", r)

		if r.Origin == a.id {
			return httpapi.Err(http.StatusLoopDetected, "", "breaking circuit")
		}

		err := a.cluster.Leave(r.Origin, r.ID)
		if err != nil {
			a.logger.Debug().WithError(err).WithField("id", r.ID).Log("unable to leave cluster")
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

		defer data.Close()

		return c.Stream(http.StatusOK, "application/octet-stream", data)
	})

	a.router.POST("/v1/process", func(c echo.Context) error {
		return httpapi.Err(http.StatusNotImplemented, "")
	})

	a.router.DELETE("/v1/process/:id", func(c echo.Context) error {
		return httpapi.Err(http.StatusNotImplemented, "")
	})

	a.router.GET("/v1/core", func(c echo.Context) error {
		address, _ := a.cluster.CoreAPIAddress("")
		return c.JSON(http.StatusOK, address)
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

type APIClient struct {
	Address string
	Client  *http.Client
}

func (c *APIClient) CoreAPIAddress() (string, error) {
	data, err := c.call(http.MethodGet, "/core", "", nil)
	if err != nil {
		return "", err
	}

	var address string
	err = json.Unmarshal(data, &address)
	if err != nil {
		return "", err
	}

	return address, nil
}

func (c *APIClient) Join(r JoinRequest) error {
	data, err := json.Marshal(&r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPost, "/join", "application/json", bytes.NewReader(data))

	return err
}

func (c *APIClient) Leave(r LeaveRequest) error {
	data, err := json.Marshal(&r)
	if err != nil {
		return err
	}

	_, err = c.call(http.MethodPost, "/leave", "application/json", bytes.NewReader(data))

	return err
}

func (c *APIClient) Snapshot() (io.ReadCloser, error) {
	return c.stream(http.MethodGet, "/snapshot", "", nil)
}

func (c *APIClient) stream(method, path, contentType string, data io.Reader) (io.ReadCloser, error) {
	if len(c.Address) == 0 {
		return nil, fmt.Errorf("no address defined")
	}

	address := "http://" + c.Address + "/v1" + path

	req, err := http.NewRequest(method, address, data)
	if err != nil {
		return nil, err
	}

	if method == "POST" || method == "PUT" {
		req.Header.Add("Content-Type", contentType)
	}

	status, body, err := c.request(req)
	if err != nil {
		return nil, err
	}

	if status < 200 || status >= 300 {
		e := httpapi.Error{}

		defer body.Close()

		x, _ := io.ReadAll(body)

		json.Unmarshal(x, &e)

		return nil, e
	}

	return body, nil
}

func (c *APIClient) call(method, path, contentType string, data io.Reader) ([]byte, error) {
	body, err := c.stream(method, path, contentType, data)
	if err != nil {
		return nil, err
	}

	defer body.Close()

	x, _ := io.ReadAll(body)

	return x, nil
}

func (c *APIClient) request(req *http.Request) (int, io.ReadCloser, error) {
	if c.Client == nil {
		tr := &http.Transport{
			MaxIdleConns:    10,
			IdleConnTimeout: 30 * time.Second,
		}

		c.Client = &http.Client{
			Transport: tr,
			Timeout:   5 * time.Second,
		}
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return -1, nil, err
	}

	return resp.StatusCode, resp.Body, nil
}
