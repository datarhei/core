package value

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAliasValue(t *testing.T) {
	aliases := []Alias{}

	v := NewAliasList(&aliases, nil, " ")
	require.Equal(t, "(empty)", v.String())
	require.NoError(t, v.Validate())

	err := v.Set(`^/memfs/b/(.*)$->/memfs/a/$1  ^/data/x\.txt$->/data/y.txt`)
	require.NoError(t, err)
	require.Equal(t, []Alias{
		{Pattern: "^/memfs/b/(.*)$", Target: "/memfs/a/$1"},
		{Pattern: `^/data/x\.txt$`, Target: "/data/y.txt"},
	}, aliases)
	require.Equal(t, `^/memfs/b/(.*)$->/memfs/a/$1 ^/data/x\.txt$->/data/y.txt`, v.String())
	require.NoError(t, v.Validate())

	err = v.Set("^/memfs/b/(.*)$")
	require.Error(t, err)

	err = v.Set("^/memfs/b/(.*$->/memfs/a/$1")
	require.NoError(t, err)
	require.Error(t, v.Validate())

	err = v.Set("^/memfs/b/(.*)$->memfs/a/$1")
	require.NoError(t, err)
	require.Error(t, v.Validate())

	err = v.Set("->/memfs/a/")
	require.NoError(t, err)
	require.Error(t, v.Validate())
}
