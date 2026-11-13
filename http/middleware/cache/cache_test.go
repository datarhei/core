package cache

import (
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestCachePassthrough(t *testing.T) {
	c, err := cache.NewLRUCache(cache.LRUConfig{
		TTL:         300 * time.Second,
		MaxSize:     0,
		MaxFileSize: 16,
		Logger:      nil,
	})
	require.NoError(t, err)

	chunk := []byte(strings.Repeat("x", 10))

	var rec *httptest.ResponseRecorder

	handler := NewWithConfig(Config{
		Cache: c,
	})(func(c echo.Context) error {
		res := c.Response()

		switch c.Request().URL.Path {
		case "/withlength.js":
			res.Header().Set(echo.HeaderContentLength, "1000")
			res.WriteHeader(http.StatusOK)

			for i := 0; i < 100; i++ {
				res.Write(chunk)
				// Every chunk has to be sent immediately
				require.Equal(t, (i+1)*len(chunk), rec.Body.Len())
			}
		case "/withoutlength.js":
			res.WriteHeader(http.StatusOK)

			res.Write(chunk)
			// The first chunk can still be cached and is buffered
			require.Equal(t, 0, rec.Body.Len())

			for i := 1; i < 100; i++ {
				res.Write(chunk)
				// The limit is exceeded, every chunk is sent immediately
				require.Equal(t, (i+1)*len(chunk), rec.Body.Len())
			}
		case "/notfound.js":
			res.Header().Set("X-Foobar", "foobar")
			res.WriteHeader(http.StatusNotFound)
			res.Write(chunk)
			require.Equal(t, len(chunk), rec.Body.Len())
		}

		return nil
	})

	e := echo.New()

	request := func(target string) {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		rec = httptest.NewRecorder()
		require.NoError(t, handler(e.NewContext(req, rec)))
	}

	for i := 0; i < 2; i++ {
		request("/withlength.js")
		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, 1000, rec.Body.Len())
		require.Equal(t, "SKIP TOOBIG", rec.Header().Get("X-Cache"))
		require.Equal(t, "1000", rec.Header().Get(echo.HeaderContentLength))

		request("/withoutlength.js")
		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, 1000, rec.Body.Len())
		require.Equal(t, "SKIP TOOBIG", rec.Header().Get("X-Cache"))

		request("/notfound.js")
		require.Equal(t, http.StatusNotFound, rec.Code)
		require.Equal(t, string(chunk), rec.Body.String())
		require.Equal(t, "SKIP NOTOK", rec.Header().Get("X-Cache"))
		require.Equal(t, "foobar", rec.Header().Get("X-Foobar"))
	}

	obj, _, _ := c.Get("/withlength.js")
	require.Nil(t, obj)

	obj, _, _ = c.Get("/withoutlength.js")
	require.Nil(t, obj)
}
