package api

import (
	"net/http"
	"testing"

	"github.com/datarhei/core/http/api"
	"github.com/datarhei/core/http/mock"
	"github.com/labstack/echo/v4"
)

func getDummyAboutRouter() *echo.Echo {
	router := mock.DummyEcho()

	rs := mock.DummyRestreamer()

	handler := NewAbout(rs, []string{})

	router.Add("GET", "/", handler.About)

	return router
}

func TestAbout(t *testing.T) {
	router := getDummyAboutRouter()

	response := mock.Request(t, http.StatusOK, router, "GET", "/", nil)

	mock.Validate(t, &api.About{}, response.Data)
}
