package iam

import (
	"encoding/json"
	"testing"

	"github.com/datarhei/core/v16/io/fs"
	"github.com/stretchr/testify/require"
)

func TestAddPolicy(t *testing.T) {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	a, err := newAdapter(memfs, "/policy.json", nil)
	require.NoError(t, err)

	err = a.AddPolicy("p", "p", []string{"foobar", "group", "resource", "action"})
	require.NoError(t, err)

	require.Equal(t, 1, len(a.groups))

	data, err := memfs.ReadFile("/policy.json")
	require.NoError(t, err)

	g := []Group{}
	err = json.Unmarshal(data, &g)
	require.NoError(t, err)

	require.Equal(t, "group", g[0].Name)
	require.Equal(t, 1, len(g[0].Policies))
	require.Equal(t, GroupPolicy{
		Username: "foobar",
		Role: Role{
			Resource: "resource",
			Actions:  "action",
		},
	}, g[0].Policies[0])
}

func TestFormatActions(t *testing.T) {
	data := [][]string{
		{"a|b|c", "a|b|c"},
		{"b|c|a", "a|b|c"},
	}

	for _, d := range data {
		require.Equal(t, d[1], formatActions(d[0]), d[0])
	}
}

func TestRemovePolicy(t *testing.T) {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	a, err := newAdapter(memfs, "/policy.json", nil)
	require.NoError(t, err)

	err = a.AddPolicies("p", "p", [][]string{
		{"foobar1", "group", "resource1", "action1"},
		{"foobar2", "group", "resource2", "action2"},
	})
	require.NoError(t, err)

	require.Equal(t, 1, len(a.groups))
	require.Equal(t, 2, len(a.groups[0].Policies))

	err = a.RemovePolicy("p", "p", []string{"foobar1", "group", "resource1", "action1"})
	require.NoError(t, err)

	require.Equal(t, 1, len(a.groups))
	require.Equal(t, 1, len(a.groups[0].Policies))

	err = a.RemovePolicy("p", "p", []string{"foobar2", "group", "resource2", "action2"})
	require.NoError(t, err)

	require.Equal(t, 0, len(a.groups))

	data, err := memfs.ReadFile("/policy.json")
	require.NoError(t, err)

	g := []Group{}
	err = json.Unmarshal(data, &g)
	require.NoError(t, err)

	require.Equal(t, 0, len(g))
}
