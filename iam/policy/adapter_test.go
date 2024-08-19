package policy

import (
	"testing"

	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/io/fs"

	"github.com/stretchr/testify/require"
)

func TestAddPolicy(t *testing.T) {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	ai, err := NewJSONAdapter(memfs, "/policy.json", nil)
	require.NoError(t, err)

	a, ok := ai.(*policyadapter)
	require.True(t, ok)

	err = a.AddPolicy(Policy{
		Name:     "foobar",
		Domain:   "group",
		Types:    []string{},
		Resource: "resource",
		Actions:  []string{"action"},
	})
	require.NoError(t, err)

	require.Equal(t, 1, len(a.domains))

	data, err := memfs.ReadFile("/policy.json")
	require.NoError(t, err)

	g := []Domain{}
	err = json.Unmarshal(data, &g)
	require.NoError(t, err)

	require.Equal(t, "group", g[0].Name)
	require.Equal(t, 1, len(g[0].Policies))
	require.Equal(t, DomainPolicy{
		Username: "foobar",
		Resource: "resource",
		Actions:  "action",
	}, g[0].Policies[0])
}

func TestRemovePolicy(t *testing.T) {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	ai, err := NewJSONAdapter(memfs, "/policy.json", nil)
	require.NoError(t, err)

	a, ok := ai.(*policyadapter)
	require.True(t, ok)

	err = a.AddPolicies([]Policy{
		{
			Name:     "foobar1",
			Domain:   "group",
			Types:    []string{},
			Resource: "resource1",
			Actions:  []string{"action1"},
		},
		{
			Name:     "foobar2",
			Domain:   "group",
			Types:    []string{},
			Resource: "resource2",
			Actions:  []string{"action2"},
		},
	})
	require.NoError(t, err)

	require.Equal(t, 1, len(a.domains))
	require.Equal(t, 2, len(a.domains[0].Policies))

	err = a.RemovePolicy(Policy{
		Name:     "foobar1",
		Domain:   "group",
		Types:    []string{},
		Resource: "resource1",
		Actions:  []string{"action1"},
	})
	require.NoError(t, err)

	require.Equal(t, 1, len(a.domains))
	require.Equal(t, 1, len(a.domains[0].Policies))

	err = a.RemovePolicy(Policy{
		Name:     "foobar2",
		Domain:   "group",
		Types:    []string{},
		Resource: "resource2",
		Actions:  []string{"action2"},
	})
	require.NoError(t, err)

	require.Equal(t, 0, len(a.domains))

	data, err := memfs.ReadFile("/policy.json")
	require.NoError(t, err)

	g := []Domain{}
	err = json.Unmarshal(data, &g)
	require.NoError(t, err)

	require.Equal(t, 0, len(g))
}
