package api

import (
	"net/http"
	"testing"

	"github.com/datarhei/core/http/api"
	"github.com/datarhei/core/http/mock"
	"github.com/labstack/echo/v4"
)

func getDummyLogRouter() *echo.Echo {
	router := mock.DummyEcho()

	handler := NewLog(nil)

	router.Add("GET", "/", handler.Log)

	return router
}

func TestLog(t *testing.T) {
	router := getDummyLogRouter()

	response := mock.Request(t, http.StatusOK, router, "GET", "/", nil)

	mock.Validate(t, []api.LogEvent{}, response.Data)
}
