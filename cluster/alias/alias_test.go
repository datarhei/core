package alias

import (
	"testing"

	"github.com/datarhei/core/v16/alias"
	"github.com/datarhei/core/v16/cluster/store"
	"github.com/datarhei/core/v16/encoding/json"

	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/require"
)

func apply(t *testing.T, s store.Store, op store.Operation, data any) {
	b, err := json.Marshal(store.Command{Operation: op, Data: data})
	require.NoError(t, err)

	res := s.Apply(&raft.Log{Index: 1, Term: 1, Type: raft.LogCommand, Data: b})
	require.Nil(t, res)
}

func TestAlias(t *testing.T) {
	s, err := store.NewStore(store.Config{})
	require.NoError(t, err)

	apply(t, s, store.OpAddAlias, store.CommandAddAlias{Rule: alias.Rule{Pattern: "^/memfs/b/(.+)$", Target: "/memfs/a/$1"}})

	m, err := New(s, nil)
	require.NoError(t, err)

	target, ok := m.Alias("/memfs/b/x.txt")
	require.True(t, ok)
	require.Equal(t, "/memfs/a/x.txt", target)

	apply(t, s, store.OpUpdateAlias, store.CommandUpdateAlias{
		Pattern: "^/memfs/b/(.+)$",
		Rule:    alias.Rule{Pattern: "^/memfs/b/(.+)$", Target: "/memfs/c/$1"},
	})

	target, ok = m.Alias("/memfs/b/x.txt")
	require.True(t, ok)
	require.Equal(t, "/memfs/c/x.txt", target)

	apply(t, s, store.OpAddAlias, store.CommandAddAlias{Rule: alias.Rule{Pattern: "^/memfs/d/(.+)$", Target: "/memfs/a/$1"}})

	target, ok = m.Alias("/memfs/d/x.txt")
	require.True(t, ok)
	require.Equal(t, "/memfs/a/x.txt", target)

	require.Equal(t, []alias.Rule{
		{Pattern: "^/memfs/b/(.+)$", Target: "/memfs/c/$1"},
		{Pattern: "^/memfs/d/(.+)$", Target: "/memfs/a/$1"},
	}, m.List())

	r, err := m.Get("^/memfs/d/(.+)$")
	require.NoError(t, err)
	require.Equal(t, alias.Rule{Pattern: "^/memfs/d/(.+)$", Target: "/memfs/a/$1"}, r)

	_, err = m.Get("^/memfs/x/(.+)$")
	require.ErrorIs(t, err, alias.ErrNotFound)

	apply(t, s, store.OpRemoveAlias, store.CommandRemoveAlias{Pattern: "^/memfs/b/(.+)$"})

	_, ok = m.Alias("/memfs/b/x.txt")
	require.False(t, ok)

	apply(t, s, store.OpSetAliases, store.CommandSetAliases{Rules: []alias.Rule{}})

	_, ok = m.Alias("/memfs/d/x.txt")
	require.False(t, ok)
	require.Empty(t, m.List())
}

func TestReadOnly(t *testing.T) {
	s, err := store.NewStore(store.Config{})
	require.NoError(t, err)

	m, err := New(s, nil)
	require.NoError(t, err)

	r := alias.Rule{Pattern: "^/memfs/b/(.+)$", Target: "/memfs/a/$1"}

	require.ErrorIs(t, m.Add(r), alias.ErrReadOnly)
	require.ErrorIs(t, m.Update(r.Pattern, r), alias.ErrReadOnly)
	require.ErrorIs(t, m.Remove(r.Pattern), alias.ErrReadOnly)
	require.ErrorIs(t, m.Set([]alias.Rule{r}), alias.ErrReadOnly)

	require.Empty(t, m.List())
}
