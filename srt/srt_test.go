package srt

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSplitPath(t *testing.T) {
	elms := splitPath("foo")
	require.Equal(t, []string{"foo"}, elms)

	elms = splitPath("/foo")
	require.Equal(t, []string{"foo"}, elms)

	elms = splitPath("foo/")
	require.Equal(t, []string{"foo"}, elms)

	elms = splitPath("/foo/")
	require.Equal(t, []string{"foo"}, elms)

	elms = splitPath("foo/bar")
	require.Equal(t, []string{"foo", "bar"}, elms)

	elms = splitPath("/foo/bar")
	require.Equal(t, []string{"foo", "bar"}, elms)

	elms = splitPath("foo///bar/")
	require.Equal(t, []string{"foo", "bar"}, elms)

	elms = splitPath("///foo///bar/")
	require.Equal(t, []string{"foo", "bar"}, elms)
}
