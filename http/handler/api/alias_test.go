package api

import (
	"bytes"
	"net/http"
	"net/url"
	"testing"

	"github.com/datarhei/core/v16/alias"
	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/mock"
	"github.com/datarhei/core/v16/io/fs"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func getDummyAliasRouter(t *testing.T, superuser bool) (*echo.Echo, alias.Manager) {
	router := mock.DummyEcho()

	router.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("superuser", superuser)
			return next(c)
		}
	})

	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	aliases, err := alias.New(alias.Config{
		Filesystem: memfs,
		Filepath:   "/aliases.json",
	})
	require.NoError(t, err)

	handler := NewAlias(aliases)

	router.Add("GET", "/", handler.List)
	router.Add("PUT", "/", handler.Set)
	router.Add("POST", "/", handler.Add)
	router.Add("GET", "/rule", handler.Get)
	router.Add("PUT", "/rule", handler.Update)
	router.Add("DELETE", "/rule", handler.Delete)

	return router, aliases
}

func aliasBody(t *testing.T, v any) *bytes.Reader {
	data, err := json.Marshal(v)
	require.NoError(t, err)

	return bytes.NewReader(data)
}

func aliasList(t *testing.T, router *echo.Echo) []api.Alias {
	response := mock.Request(t, http.StatusOK, router, "GET", "/", nil)

	aliases := []api.Alias{}
	require.NoError(t, json.Unmarshal(response.Raw, &aliases))

	return aliases
}

func rulePath(pattern string) string {
	return "/rule?pattern=" + url.QueryEscape(pattern)
}

func TestAliasAdd(t *testing.T) {
	router, _ := getDummyAliasRouter(t, true)

	require.Equal(t, []api.Alias{}, aliasList(t, router))

	mock.Request(t, http.StatusOK, router, "POST", "/", aliasBody(t, api.Alias{Pattern: "^/memfs/b/(.+)$", Target: "/memfs/a/$1"}))
	mock.Request(t, http.StatusOK, router, "POST", "/", aliasBody(t, api.Alias{Pattern: "^/memfs/c/(.+)$", Target: "/memfs/a/$1"}))

	// Duplicate
	mock.Request(t, http.StatusConflict, router, "POST", "/", aliasBody(t, api.Alias{Pattern: "^/memfs/b/(.+)$", Target: "/memfs/x/$1"}))

	// Invalid
	mock.Request(t, http.StatusBadRequest, router, "POST", "/", aliasBody(t, api.Alias{Pattern: "^/memfs/d/(.+$", Target: "/memfs/a/$1"}))
	mock.Request(t, http.StatusBadRequest, router, "POST", "/", aliasBody(t, api.Alias{Pattern: "^/memfs/d/(.+)$", Target: "memfs/a/$1"}))
	mock.Request(t, http.StatusBadRequest, router, "POST", "/", aliasBody(t, api.Alias{Pattern: "^/memfs/d/(.+)$"}))
	mock.Request(t, http.StatusBadRequest, router, "POST", "/", bytes.NewReader([]byte("{")))

	require.Equal(t, []api.Alias{
		{Pattern: "^/memfs/b/(.+)$", Target: "/memfs/a/$1"},
		{Pattern: "^/memfs/c/(.+)$", Target: "/memfs/a/$1"},
	}, aliasList(t, router))
}

func TestAliasRule(t *testing.T) {
	router, _ := getDummyAliasRouter(t, true)

	pattern := `^/memfs/b%20c/(.+)\.m3u8$`

	mock.Request(t, http.StatusOK, router, "POST", "/", aliasBody(t, api.Alias{Pattern: pattern, Target: "/memfs/a/$1.m3u8"}))
	mock.Request(t, http.StatusOK, router, "POST", "/", aliasBody(t, api.Alias{Pattern: "^/memfs/c/(.+)$", Target: "/memfs/a/$1"}))

	// Get
	response := mock.Request(t, http.StatusOK, router, "GET", rulePath(pattern), nil)
	a := api.Alias{}
	require.NoError(t, json.Unmarshal(response.Raw, &a))
	require.Equal(t, api.Alias{Pattern: pattern, Target: "/memfs/a/$1.m3u8"}, a)

	mock.Request(t, http.StatusBadRequest, router, "GET", "/rule", nil)
	mock.Request(t, http.StatusNotFound, router, "GET", rulePath("^/memfs/x/(.+)$"), nil)

	// Update
	mock.Request(t, http.StatusOK, router, "PUT", rulePath(pattern), aliasBody(t, api.Alias{Pattern: "^/memfs/d/(.+)$", Target: "/memfs/x/$1"}))
	mock.Request(t, http.StatusNotFound, router, "PUT", rulePath(pattern), aliasBody(t, api.Alias{Pattern: "^/memfs/e/(.+)$", Target: "/memfs/x/$1"}))
	mock.Request(t, http.StatusConflict, router, "PUT", rulePath("^/memfs/d/(.+)$"), aliasBody(t, api.Alias{Pattern: "^/memfs/c/(.+)$", Target: "/memfs/x/$1"}))
	mock.Request(t, http.StatusBadRequest, router, "PUT", rulePath("^/memfs/d/(.+)$"), aliasBody(t, api.Alias{Pattern: "^/memfs/d/(.+)$", Target: "x/$1"}))
	mock.Request(t, http.StatusBadRequest, router, "PUT", "/rule", aliasBody(t, api.Alias{Pattern: "^/memfs/d/(.+)$", Target: "/memfs/x/$1"}))

	require.Equal(t, []api.Alias{
		{Pattern: "^/memfs/d/(.+)$", Target: "/memfs/x/$1"},
		{Pattern: "^/memfs/c/(.+)$", Target: "/memfs/a/$1"},
	}, aliasList(t, router))

	// Delete
	mock.Request(t, http.StatusOK, router, "DELETE", rulePath("^/memfs/d/(.+)$"), nil)
	mock.Request(t, http.StatusNotFound, router, "DELETE", rulePath("^/memfs/d/(.+)$"), nil)
	mock.Request(t, http.StatusBadRequest, router, "DELETE", "/rule", nil)

	require.Equal(t, []api.Alias{
		{Pattern: "^/memfs/c/(.+)$", Target: "/memfs/a/$1"},
	}, aliasList(t, router))
}

func TestAliasSet(t *testing.T) {
	router, aliases := getDummyAliasRouter(t, true)

	mock.Request(t, http.StatusOK, router, "POST", "/", aliasBody(t, api.Alias{Pattern: "^/memfs/b/(.+)$", Target: "/memfs/a/$1"}))

	expected := []api.Alias{
		{Pattern: "^/memfs/d/(.+)$", Target: "/memfs/x/$1"},
		{Pattern: "^/memfs/c/(.+)$", Target: "/memfs/y/$1"},
	}

	mock.Request(t, http.StatusOK, router, "PUT", "/", aliasBody(t, expected))
	require.Equal(t, expected, aliasList(t, router))

	// Invalid lists don't change anything
	mock.Request(t, http.StatusBadRequest, router, "PUT", "/", aliasBody(t, []api.Alias{
		{Pattern: "^/memfs/e/(.+)$", Target: "/memfs/x/$1"},
		{Pattern: "^/memfs/f/(.+)$"},
	}))
	mock.Request(t, http.StatusBadRequest, router, "PUT", "/", aliasBody(t, []api.Alias{
		{Pattern: "^/memfs/e/(.+$", Target: "/memfs/x/$1"},
	}))
	mock.Request(t, http.StatusConflict, router, "PUT", "/", aliasBody(t, []api.Alias{
		{Pattern: "^/memfs/e/(.+)$", Target: "/memfs/x/$1"},
		{Pattern: "^/memfs/e/(.+)$", Target: "/memfs/y/$1"},
	}))
	mock.Request(t, http.StatusBadRequest, router, "PUT", "/", bytes.NewReader([]byte(`{"pattern":"^/x$","target":"/y"}`)))

	require.Equal(t, expected, aliasList(t, router))

	// The aliaser applies the new rules
	target, ok := aliases.Alias("/memfs/c/x.txt")
	require.True(t, ok)
	require.Equal(t, "/memfs/y/x.txt", target)

	// Remove all
	mock.Request(t, http.StatusOK, router, "PUT", "/", aliasBody(t, []api.Alias{}))
	require.Equal(t, []api.Alias{}, aliasList(t, router))
}

func TestAliasNoSuperuser(t *testing.T) {
	router, aliases := getDummyAliasRouter(t, false)

	require.NoError(t, aliases.Add(alias.Rule{Pattern: "^/memfs/b/(.+)$", Target: "/memfs/a/$1"}))

	a := api.Alias{Pattern: "^/memfs/c/(.+)$", Target: "/memfs/a/$1"}

	mock.Request(t, http.StatusForbidden, router, "POST", "/", aliasBody(t, a))
	mock.Request(t, http.StatusForbidden, router, "PUT", "/", aliasBody(t, []api.Alias{a}))
	mock.Request(t, http.StatusForbidden, router, "PUT", rulePath("^/memfs/b/(.+)$"), aliasBody(t, a))
	mock.Request(t, http.StatusForbidden, router, "DELETE", rulePath("^/memfs/b/(.+)$"), nil)

	// Reading is allowed
	mock.Request(t, http.StatusOK, router, "GET", rulePath("^/memfs/b/(.+)$"), nil)

	require.Equal(t, []api.Alias{
		{Pattern: "^/memfs/b/(.+)$", Target: "/memfs/a/$1"},
	}, aliasList(t, router))
}
