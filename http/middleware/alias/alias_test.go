package alias

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/datarhei/core/v16/alias"
	"github.com/datarhei/core/v16/http/handler/util"
	"github.com/datarhei/core/v16/io/fs"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

type result struct {
	Status    int
	Param     string
	Path      string
	Query     string
	OuterPath string
}

func newAliaser(t *testing.T, rules []alias.Rule) alias.Manager {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	aliaser, err := alias.New(alias.Config{
		Filesystem: memfs,
		Filepath:   "/aliases.json",
	})
	require.NoError(t, err)

	for _, rule := range rules {
		require.NoError(t, aliaser.Add(rule))
	}

	return aliaser
}

func newRouter(t *testing.T, rules []alias.Rule) (*echo.Echo, alias.Manager) {
	aliaser := newAliaser(t, rules)

	e := echo.New()

	// Mimics the log middleware which sees the path after the handler returns.
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			err := next(c)
			c.Response().Header().Set("X-Outer-Path", c.Request().URL.Path)
			return err
		}
	})

	e.Use(NewWithConfig(Config{
		Mounts:  []string{"/", "/memfs"},
		Aliaser: aliaser,
	}))

	handler := func(c echo.Context) error {
		c.Response().Header().Set("X-Param", util.PathWildcardParam(c))
		c.Response().Header().Set("X-Path", c.Request().URL.Path)
		c.Response().Header().Set("X-Query", c.Request().URL.RawQuery)
		return c.NoContent(http.StatusOK)
	}

	for _, mount := range []string{"/memfs/*", "/*"} {
		g := e.Group(mount)
		g.GET("", handler)
		g.HEAD("", handler)
		g.PUT("", handler)
		g.DELETE("", handler)
	}

	e.GET("/api/*", handler)

	return e, aliaser
}

func request(e *echo.Echo, method, target string) result {
	req := httptest.NewRequest(method, target, nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	return result{
		Status:    rec.Code,
		Param:     rec.Header().Get("X-Param"),
		Path:      rec.Header().Get("X-Path"),
		Query:     rec.Header().Get("X-Query"),
		OuterPath: rec.Header().Get("X-Outer-Path"),
	}
}

func TestAlias(t *testing.T) {
	e, _ := newRouter(t, []alias.Rule{
		{Pattern: `^/memfs/b/(.*)$`, Target: "/memfs/a/$1"},
		{Pattern: `^/memfs/b/x\.txt$`, Target: "/memfs/never.txt"},
		{Pattern: `^/memfs/(?P<user>[^/]+)/shared/(.*)$`, Target: "/memfs/${user}-public/$2"},
		{Pattern: `^/foo/(.*)$`, Target: "/bar/$1"},
	})

	r := request(e, http.MethodGet, "/memfs/b/x.txt?session=123&token=abc")
	require.Equal(t, http.StatusOK, r.Status)
	require.Equal(t, "/a/x.txt", r.Param)
	require.Equal(t, "/memfs/a/x.txt", r.Path)
	require.Equal(t, "session=123&token=abc", r.Query)
	require.Equal(t, "/memfs/b/x.txt", r.OuterPath)

	r = request(e, http.MethodHead, "/memfs/b/live/stream.m3u8")
	require.Equal(t, http.StatusOK, r.Status)
	require.Equal(t, "/a/live/stream.m3u8", r.Param)

	r = request(e, http.MethodGet, "/memfs/c/shared/x.txt")
	require.Equal(t, http.StatusOK, r.Status)
	require.Equal(t, "/c-public/x.txt", r.Param)

	r = request(e, http.MethodGet, "/foo/x.txt")
	require.Equal(t, http.StatusOK, r.Status)
	require.Equal(t, "/bar/x.txt", r.Param)
	require.Equal(t, "/bar/x.txt", r.Path)
}

func TestAliasNoMatch(t *testing.T) {
	e, _ := newRouter(t, []alias.Rule{
		{Pattern: `^/memfs/b/(.*)$`, Target: "/memfs/a/$1"},
	})

	r := request(e, http.MethodGet, "/memfs/c/x.txt")
	require.Equal(t, http.StatusOK, r.Status)
	require.Equal(t, "/c/x.txt", r.Param)
	require.Equal(t, "/memfs/c/x.txt", r.Path)
}

func TestAliasMethods(t *testing.T) {
	e, _ := newRouter(t, []alias.Rule{
		{Pattern: `^/memfs/b/(.*)$`, Target: "/memfs/a/$1"},
	})

	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		r := request(e, method, "/memfs/b/x.txt")
		require.Equal(t, http.StatusOK, r.Status, method)
		require.Equal(t, "/b/x.txt", r.Param, method)
		require.Equal(t, "/memfs/b/x.txt", r.Path, method)
	}
}

func TestAliasNonFilesystemRoute(t *testing.T) {
	e, _ := newRouter(t, []alias.Rule{
		{Pattern: `^/api/(.*)$`, Target: "/api/v3/$1"},
	})

	r := request(e, http.MethodGet, "/api/foo")
	require.Equal(t, http.StatusOK, r.Status)
	require.Equal(t, "/api/foo", r.Path)
}

func TestAliasOtherMount(t *testing.T) {
	e, _ := newRouter(t, []alias.Rule{
		{Pattern: `^/memfs/b/(.*)$`, Target: "/disk/$1"},
		{Pattern: `^/foo/(.*)$`, Target: "/memfs/$1"},
		{Pattern: `^/memfs/c/(.*)$`, Target: "/memfs/../$1"},
		{Pattern: `^/memfs/d/(.*)$`, Target: "/memfs/a/$1"},
	})

	// From /memfs to /
	r := request(e, http.MethodGet, "/memfs/b/x.txt")
	require.Equal(t, http.StatusNotFound, r.Status)

	// From / to /memfs
	r = request(e, http.MethodGet, "/foo/x.txt")
	require.Equal(t, http.StatusNotFound, r.Status)

	// Escaping the mount
	r = request(e, http.MethodGet, "/memfs/c/x.txt")
	require.Equal(t, http.StatusNotFound, r.Status)

	// Escaping the mount with the captured part
	r = request(e, http.MethodGet, "/memfs/d/../../x.txt")
	require.Equal(t, http.StatusNotFound, r.Status)
}

func TestAliasEscaping(t *testing.T) {
	e, _ := newRouter(t, []alias.Rule{
		{Pattern: `^/memfs/b/(.*)$`, Target: "/memfs/a/$1"},
	})

	r := request(e, http.MethodGet, "/memfs/b/x%20y%2541.txt")
	require.Equal(t, http.StatusOK, r.Status)
	require.Equal(t, "/a/x y%41.txt", r.Param)
	require.Equal(t, "/memfs/a/x y%41.txt", r.Path)
	require.Equal(t, "/memfs/b/x y%41.txt", r.OuterPath)
}

func TestAliasTrailingSlash(t *testing.T) {
	e, _ := newRouter(t, []alias.Rule{
		{Pattern: `^/memfs/b/(.*)$`, Target: "/memfs/a/$1"},
	})

	r := request(e, http.MethodGet, "/memfs/b/dir/")
	require.Equal(t, http.StatusOK, r.Status)
	require.Equal(t, "/a/dir/", r.Param)
}

func TestAliasNoRules(t *testing.T) {
	e, _ := newRouter(t, nil)

	r := request(e, http.MethodGet, "/memfs/b/x.txt")
	require.Equal(t, http.StatusOK, r.Status)
	require.Equal(t, "/b/x.txt", r.Param)
}

func TestAliasDynamic(t *testing.T) {
	e, aliaser := newRouter(t, nil)

	r := request(e, http.MethodGet, "/memfs/b/x.txt")
	require.Equal(t, http.StatusOK, r.Status)
	require.Equal(t, "/b/x.txt", r.Param)

	// A new rule applies to the next request
	require.NoError(t, aliaser.Add(alias.Rule{Pattern: `^/memfs/b/(.*)$`, Target: "/memfs/a/$1"}))

	r = request(e, http.MethodGet, "/memfs/b/x.txt")
	require.Equal(t, http.StatusOK, r.Status)
	require.Equal(t, "/a/x.txt", r.Param)

	// An updated rule applies to the next request
	require.NoError(t, aliaser.Update(`^/memfs/b/(.*)$`, alias.Rule{Pattern: `^/memfs/b/(.*)$`, Target: "/memfs/c/$1"}))

	r = request(e, http.MethodGet, "/memfs/b/x.txt")
	require.Equal(t, http.StatusOK, r.Status)
	require.Equal(t, "/c/x.txt", r.Param)

	// A removed rule doesn't apply anymore
	require.NoError(t, aliaser.Remove(`^/memfs/b/(.*)$`))

	r = request(e, http.MethodGet, "/memfs/b/x.txt")
	require.Equal(t, http.StatusOK, r.Status)
	require.Equal(t, "/b/x.txt", r.Param)
}

func TestAliasNoAliaser(t *testing.T) {
	e := echo.New()
	e.Use(NewWithConfig(Config{
		Mounts: []string{"/memfs"},
	}))
	e.GET("/memfs/*", func(c echo.Context) error {
		return c.String(http.StatusOK, util.PathWildcardParam(c))
	})

	req := httptest.NewRequest(http.MethodGet, "/memfs/b/x.txt", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "/b/x.txt", rec.Body.String())
}
