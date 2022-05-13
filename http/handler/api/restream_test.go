package api

import (
	"net/http"
	"testing"

	"github.com/datarhei/core/http/api"
	"github.com/datarhei/core/http/mock"

	"github.com/labstack/echo/v4"
)

type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    interface{}
}

func getDummyRestreamHandler() *RestreamHandler {
	rs := mock.DummyRestreamer()

	handler := NewRestream(rs)

	return handler
}

func getDummyRestreamRouter() *echo.Echo {
	router := mock.DummyEcho()

	restream := getDummyRestreamHandler()

	router.GET("/", restream.GetAll)
	router.POST("/", restream.Add)
	router.GET("/:id", restream.Get)
	router.GET("/:id/report", restream.GetReport)
	router.PUT("/:id", restream.Update)
	router.DELETE("/:id", restream.Delete)
	router.PUT("/:id/command", restream.Command)

	return router
}

func TestAddProcessMissingField(t *testing.T) {
	router := getDummyRestreamRouter()

	data := mock.Read(t, "./fixtures/addProcessMissingField.json")

	mock.Request(t, http.StatusBadRequest, router, "POST", "/", data)
}

func TestAddProcessInvalidType(t *testing.T) {
	router := getDummyRestreamRouter()

	data := mock.Read(t, "./fixtures/addProcessInvalidType.json")

	mock.Request(t, http.StatusBadRequest, router, "POST", "/", data)
}

func TestAddProcess(t *testing.T) {
	router := getDummyRestreamRouter()

	data := mock.Read(t, "./fixtures/addProcess.json")

	response := mock.Request(t, http.StatusOK, router, "POST", "/", data)

	mock.Validate(t, &api.ProcessConfig{}, response.Data)
}

func TestRemoveUnknownProcess(t *testing.T) {
	router := getDummyRestreamRouter()

	mock.Request(t, http.StatusNotFound, router, "DELETE", "/foobar", nil)
}

func TestRemoveProcess(t *testing.T) {
	router := getDummyRestreamRouter()

	data := mock.Read(t, "./fixtures/removeProcess.json")

	mock.Request(t, http.StatusOK, router, "POST", "/", data)
	mock.Request(t, http.StatusOK, router, "DELETE", "/test", nil)
}

func TestProcessInfo(t *testing.T) {
	router := getDummyRestreamRouter()

	data := mock.Read(t, "./fixtures/addProcess.json")

	mock.Request(t, http.StatusOK, router, "POST", "/", data)
	response := mock.Request(t, http.StatusOK, router, "GET", "/test", nil)

	mock.Validate(t, &api.Process{}, response.Data)
}

func TestProcessReportNotFound(t *testing.T) {
	router := getDummyRestreamRouter()

	mock.Request(t, http.StatusNotFound, router, "GET", "/test/report", nil)
}

func TestProcessReport(t *testing.T) {
	router := getDummyRestreamRouter()

	data := mock.Read(t, "./fixtures/addProcess.json")

	mock.Request(t, http.StatusOK, router, "POST", "/", data)
	response := mock.Request(t, http.StatusOK, router, "GET", "/test/report", nil)

	mock.Validate(t, &api.ProcessReport{}, response.Data)
}

func TestProcessCommandNotFound(t *testing.T) {
	router := getDummyRestreamRouter()

	command := mock.Read(t, "./fixtures/commandStart.json")
	mock.Request(t, http.StatusBadRequest, router, "PUT", "/test/command", command)
}

func TestProcessCommandInvalid(t *testing.T) {
	router := getDummyRestreamRouter()

	data := mock.Read(t, "./fixtures/addProcess.json")

	mock.Request(t, http.StatusOK, router, "POST", "/", data)

	command := mock.Read(t, "./fixtures/commandInvalid.json")
	mock.Request(t, http.StatusBadRequest, router, "PUT", "/test/command", command)
}

func TestProcessCommand(t *testing.T) {
	router := getDummyRestreamRouter()

	data := mock.Read(t, "./fixtures/addProcess.json")

	mock.Request(t, http.StatusOK, router, "POST", "/", data)

	command := mock.Read(t, "./fixtures/commandStart.json")
	mock.Request(t, http.StatusOK, router, "PUT", "/test/command", command)
	mock.Request(t, http.StatusOK, router, "GET", "/test", data)

	command = mock.Read(t, "./fixtures/commandStop.json")
	mock.Request(t, http.StatusOK, router, "PUT", "/test/command", command)
	mock.Request(t, http.StatusOK, router, "GET", "/test", data)
}
