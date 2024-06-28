package rtmp

import (
	"net/url"
	"testing"

	rtmpurl "github.com/datarhei/core/v16/rtmp/url"

	"github.com/stretchr/testify/require"
)

func TestToken(t *testing.T) {
	data := [][]string{
		{"/foo/bar", "/foo", "bar"},
		{"/foo/bar?token=abc", "/foo/bar", "abc"},
		{"/foo/bar/abc", "/foo/bar", "abc"},
		{"/foo/bar/&!'", "/foo/bar", "&!'"},
		{"/foo/bar/%26%21%27", "/foo/bar", "&!'"},
	}

	for _, d := range data {
		u, err := url.Parse(d[0])
		require.NoError(t, err)

		path, token, _ := rtmpurl.GetToken(u)

		require.Equal(t, d[1], path, "url=%s", u.String())
		require.Equal(t, d[2], token, "url=%s", u.String())
	}
}

func TestSplitPath(t *testing.T) {
	data := map[string][]string{
		"/foo/bar":  {"foo", "bar"},
		"foo/bar":   {"foo", "bar"},
		"/foo/bar/": {"foo", "bar"},
	}

	for path, split := range data {
		elms := rtmpurl.SplitPath(path)

		require.ElementsMatch(t, split, elms, "%s", path)
	}
}

func TestRemovePathPrefix(t *testing.T) {
	data := [][]string{
		{"/foo/bar", "/foo", "/bar"},
		{"/foo/bar", "/fo", "/foo/bar"},
		{"/foo/bar/abc", "/foo/bar", "/abc"},
	}

	for _, d := range data {
		x, _ := rtmpurl.RemovePathPrefix(d[0], d[1])

		require.Equal(t, d[2], x, "path=%s prefix=%s", d[0], d[1])
	}
}
