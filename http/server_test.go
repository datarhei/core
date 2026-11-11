package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/http/errorhandler"
	"github.com/datarhei/core/v16/http/errors"
	"github.com/datarhei/core/v16/log"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func TestRecoverFromPanic(t *testing.T) {
	router := echo.New()
	router.HTTPErrorHandler = errorhandler.HTTPErrorHandler
	router.Use(newRecoverMiddleware(log.New("")))

	router.GET("/panic", func(c echo.Context) error {
		panic("foobar")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()

	require.NotPanics(t, func() {
		router.ServeHTTP(rec, req)
	})

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.NotContains(t, rec.Body.String(), "foobar")

	e := errors.Error{}
	err := json.Unmarshal(rec.Body.Bytes(), &e)
	require.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, e.Code)
}
