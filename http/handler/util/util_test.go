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
