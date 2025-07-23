package glob

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPatterns(t *testing.T) {
	ok, err := Match("**/a/b/**", "/s3/a/b/test.m3u8", '/')

	require.NoError(t, err)
	require.True(t, ok)

	ok, err = Match("**/a/b/**", "/a/b/test.m3u8", '/')

	require.NoError(t, err)
	require.True(t, ok)

	ok, err = Match("{/memfs,}/a/b/**", "/a/b/test.m3u8", '/')

	require.NoError(t, err)
	require.True(t, ok)
}

func TestPrefix(t *testing.T) {
	prefix := Prefix("/a/b/c/d")
	require.Equal(t, "/a/b/c/d", prefix)

	prefix = Prefix("/a/b/*/d")
	require.Equal(t, "/a/b/", prefix)
}

func TestIsPattern(t *testing.T) {
	require.False(t, IsPattern("/a/b/c/d"))
	require.True(t, IsPattern("/a/b/*/d"))
}
