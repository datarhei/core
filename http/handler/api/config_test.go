package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/datarhei/core/v16/config"
	"github.com/datarhei/core/v16/http/mock"
	"github.com/labstack/echo/v4"
)

func getDummyConfigRouter() *echo.Echo {
	router := mock.DummyEcho()

	config := config.NewDummyStore()

	handler := NewConfig(config)

	router.Add("GET", "/", handler.Get)
	router.Add("PUT", "/", handler.Set)

	return router
}

func TestConfigGet(t *testing.T) {
	router := getDummyConfigRouter()

	mock.Request(t, http.StatusOK, router, "GET", "/", nil)

	//validate(t, &api.RestreamerConfig{}, response.Data)
}

func TestConfigSetConflict(t *testing.T) {
	router := getDummyConfigRouter()

	var data bytes.Buffer

	encoder := json.NewEncoder(&data)
	encoder.Encode(config.New())

	mock.Request(t, http.StatusConflict, router, "PUT", "/", &data)
}

func TestConfigSet(t *testing.T) {
	router := getDummyConfigRouter()

	var data bytes.Buffer

	cfg := config.New()
	cfg.FFmpeg.Binary = "true"
	cfg.DB.Dir = "."
	cfg.Storage.Disk.Dir = "."
	cfg.Storage.MimeTypes = ""

	encoder := json.NewEncoder(&data)
	encoder.Encode(cfg)

	mock.Request(t, http.StatusOK, router, "PUT", "/", &data)
}
