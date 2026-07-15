package url

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSplitPath(t *testing.T) {
	elms := SplitPath("foo")
	require.Equal(t, []string{"foo"}, elms)

	elms = SplitPath("/foo")
	require.Equal(t, []string{"foo"}, elms)

	elms = SplitPath("foo/")
	require.Equal(t, []string{"foo"}, elms)

	elms = SplitPath("/foo/")
	require.Equal(t, []string{"foo"}, elms)

	elms = SplitPath("foo/bar")
	require.Equal(t, []string{"foo", "bar"}, elms)

	elms = SplitPath("/foo/bar")
	require.Equal(t, []string{"foo", "bar"}, elms)

	elms = SplitPath("foo///bar/")
	require.Equal(t, []string{"foo", "bar"}, elms)

	elms = SplitPath("///foo///bar/")
	require.Equal(t, []string{"foo", "bar"}, elms)
}
