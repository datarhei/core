package util

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func TestDefaultQuery(t *testing.T) {
	e := echo.New()

	tests := map[string]string{
		"":                                 "default",
		"?pattern=":                        "default",
		"?pattern=abc":                     "abc",
		"?pattern=.%2B":                    ".+",
		"?pattern=a+b":                     "a b",
		"?pattern=a%2520":                  "a%20",
		"?pattern=50%25":                   "50%",
		"?pattern=%5E%2Fmemfs%2F(.%2B)%24": "^/memfs/(.+)$",
	}

	for query, expected := range tests {
		req := httptest.NewRequest(http.MethodGet, "/"+query, nil)
		c := e.NewContext(req, httptest.NewRecorder())

		require.Equal(t, expected, DefaultQuery(c, "pattern", "default"), query)
	}
}

func TestPathParam(t *testing.T) {
	e := echo.New()

	var name, wildcard string

	e.GET("/x/:name/*", func(c echo.Context) error {
		name = PathParam(c, "name")
		wildcard = PathWildcardParam(c)
		return nil
	})

	tests := []struct {
		path     string
		name     string
		wildcard string
	}{
		// The router matches on the decoded path
		{"/x/a%20b/c%20d", "a b", "/c d"},
		{"/x/a%2520b/c%2520d", "a%20b", "/c%20d"},
		{"/x/a%25b/c%25d", "a%b", "/c%d"},
		{"/x/a+b/c+d", "a+b", "/c+d"},
		// The router matches on the raw path
		{"/x/a%2Fb/c%2Fd", "a/b", "/c/d"},
		{"/x/a%2Fb%2520c/d%2520e", "a/b%20c", "/d%20e"},
		{"/x/a%2Fb%25c/d", "a/b%c", "/d"},
	}

	for _, test := range tests {
		name, wildcard = "", ""

		req := httptest.NewRequest(http.MethodGet, test.path, nil)
		e.ServeHTTP(httptest.NewRecorder(), req)

		require.Equal(t, test.name, name, test.path)
		require.Equal(t, test.wildcard, wildcard, test.path)
	}
}
