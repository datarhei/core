package api

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/mock"
	"github.com/datarhei/core/v16/restream"
	"github.com/stretchr/testify/require"

	"github.com/labstack/echo/v4"
)

func getDummyWidgetHandler(rs restream.Restreamer) (*WidgetHandler, error) {
	handler := NewWidget(WidgetConfig{
		Restream: rs,
		Registry: nil,
	})

	return handler, nil
}

func getDummyWidgetRouter(rs restream.Restreamer) (*echo.Echo, error) {
	router := mock.DummyEcho()

	widget, err := getDummyWidgetHandler(rs)
	if err != nil {
		return nil, err
	}

	router.GET("/:id", widget.Get)

	return router, nil
}

func TestWidget(t *testing.T) {
	rs, err := mock.DummyRestreamer("../../mock")
	require.NoError(t, err)

	router, err := getDummyWidgetRouter(rs)
	require.NoError(t, err)

	data, err := io.ReadAll(mock.Read(t, "./fixtures/addProcess.json"))
	require.NoError(t, err)

	process := api.ProcessConfig{}
	err = json.Unmarshal(data, &process)
	require.NoError(t, err)

	err = rs.AddProcess(process.Marshal())
	require.NoError(t, err)

	response := mock.Request(t, http.StatusOK, router, "GET", "/test", nil)

	mock.Validate(t, &api.WidgetProcess{}, response.Data)
}
