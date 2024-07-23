package policy

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResourceMatcher(t *testing.T) {
	ok := resourceMatch("fs", "/", []string{"api", "fs"}, "/")
	require.True(t, ok)

	ok = resourceMatch("bla", "/", []string{"api", "fs"}, "/")
	require.False(t, ok)

	ok = resourceMatch("fs", "/foo", []string{"api", "fs"}, "/")
	require.False(t, ok)

	ok = resourceMatch("fs", "/foo", []string{"api", "fs"}, "/*")
	require.True(t, ok)

	ok = resourceMatch("fs", "/foo/boz", []string{"api", "fs"}, "/*")
	require.False(t, ok)

	ok = resourceMatch("fs", "/foo/boz", []string{"api", "fs"}, "/**")
	require.True(t, ok)
}

func TestActionMatcher(t *testing.T) {
	ok := actionMatch("get", []string{"any"}, "any")
	require.True(t, ok)

	ok = actionMatch("get", []string{"get", "head"}, "any")
	require.True(t, ok)
}
