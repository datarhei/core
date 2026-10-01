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

	ok, err = Match("/*/5faffe88-b191-4950-a342-2e7cc93006b4/{ef09dd12-bdf5-428a-a3af-c96a690dcbec,e0d64f10-52ac-4baf-a1e6-a71085724b14}/**", "/memfs/5faffe88-b191-4950-a342-2e7cc93006b4/ef09dd12-bdf5-428a-a3af-c96a690dcbec/live/main.m3u8", '/')
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = Match("/*/5faffe88-b191-4950-a342-2e7cc93006b4/{ef09dd12-bdf5-428a-a3af-c96a690dcbec,e0d64f10-52ac-4baf-a1e6-a71085724b14}/**", "/memfs/5faffe88-b191-4950-a342-2e7cc93006b4/500680aa-a920-4ef6-982d-2e964beae825/live/main.m3u8", '/')
	require.NoError(t, err)
	require.False(t, ok)
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
