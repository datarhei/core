package cache

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/datarhei/core/v16/http/cache"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func TestCache(t *testing.T) {
	c, err := cache.NewLRUCache(cache.LRUConfig{
		TTL:             300 * time.Second,
		MaxSize:         0,
		MaxFileSize:     16,
		AllowExtensions: []string{".js"},
		BlockExtensions: []string{".ts"},
		Logger:          nil,
	})

	require.NoError(t, err)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/found.js", nil)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)

	handler := NewWithConfig(Config{
		Cache: c,
	})(func(c echo.Context) error {
		if c.Request().URL.Path == "/found.js" {
			c.Response().Write([]byte("test"))
		} else if c.Request().URL.Path == "/toobig.js" {
			c.Response().Write([]byte("testtesttesttesttest"))
		} else if c.Request().URL.Path == "/blocked.ts" {
			c.Response().Write([]byte("blocked"))
		}

		c.Response().WriteHeader(http.StatusNotFound)
		return nil
	})

	handler(ctx)

	require.Equal(t, "test", rec.Body.String())
	require.Equal(t, 200, rec.Result().StatusCode)
	require.Equal(t, "MISS", rec.Result().Header.Get("x-cache"))

	rec = httptest.NewRecorder()
	ctx = e.NewContext(req, rec)

	handler(ctx)

	require.Equal(t, "test", rec.Body.String())
	require.Equal(t, 200, rec.Result().StatusCode)
	require.Equal(t, "HIT", rec.Result().Header.Get("x-cache")[:3])

	req = httptest.NewRequest(http.MethodGet, "/notfound.js", nil)
	rec = httptest.NewRecorder()
	ctx = e.NewContext(req, rec)

	handler(ctx)

	require.Equal(t, 404, rec.Result().StatusCode)
	require.Equal(t, "SKIP NOTOK", rec.Result().Header.Get("x-cache"))

	req = httptest.NewRequest(http.MethodGet, "/toobig.js", nil)
	rec = httptest.NewRecorder()
	ctx = e.NewContext(req, rec)

	handler(ctx)

	require.Equal(t, "testtesttesttesttest", rec.Body.String())
	require.Equal(t, 200, rec.Result().StatusCode)
	require.Equal(t, "SKIP TOOBIG", rec.Result().Header.Get("x-cache"))

	req = httptest.NewRequest(http.MethodGet, "/blocked.ts", nil)
	rec = httptest.NewRecorder()
	ctx = e.NewContext(req, rec)

	handler(ctx)

	require.Equal(t, "blocked", rec.Body.String())
	require.Equal(t, 200, rec.Result().StatusCode)
	require.Equal(t, "SKIP EXT", rec.Result().Header.Get("x-cache"))

	req = httptest.NewRequest(http.MethodPost, "/found.js", nil)
	rec = httptest.NewRecorder()
	ctx = e.NewContext(req, rec)

	handler(ctx)

	require.Equal(t, "test", rec.Body.String())
	require.Equal(t, 200, rec.Result().StatusCode)
	require.Equal(t, "SKIP ONLYGET", rec.Result().Header.Get("x-cache"))
}

func TestCacheSkipper(t *testing.T) {
	c, err := cache.NewLRUCache(cache.LRUConfig{
		TTL:             300 * time.Second,
		MaxSize:         0,
		MaxFileSize:     16,
		AllowExtensions: []string{".js"},
		Logger:          nil,
	})
	require.NoError(t, err)

	calls := 0

	handler := NewWithConfig(Config{
		Skipper: func(c echo.Context) bool {
			return len(c.QueryParam("filename")) != 0
		},
		Cache: c,
	})(func(c echo.Context) error {
		calls++
		return c.String(http.StatusOK, "test")
	})

	e := echo.New()

	request := func(target string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		rec := httptest.NewRecorder()
		require.NoError(t, handler(e.NewContext(req, rec)))
		return rec
	}

	for i := 1; i <= 2; i++ {
		rec := request("/found.js?filename=foo.js")
		require.Equal(t, "test", rec.Body.String())
		require.Empty(t, rec.Header().Get("X-Cache"))
		require.Equal(t, i, calls)
	}

	rec := request("/found.js")
	require.Equal(t, "MISS", rec.Header().Get("X-Cache"))
	require.Equal(t, 3, calls)

	rec = request("/found.js")
	require.Equal(t, "HIT", rec.Header().Get("X-Cache")[:3])
	require.Equal(t, 3, calls)

	rec = request("/found.js?filename=foo.js")
	require.Empty(t, rec.Header().Get("X-Cache"))
	require.Equal(t, 4, calls)
}
