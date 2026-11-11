package store

import (
	"bytes"
	"io"
	"testing"

	"github.com/datarhei/core/v16/alias"
	"github.com/datarhei/core/v16/encoding/json"

	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/require"
)

func TestAddAliasCommand(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	err = s.applyCommand(Command{
		Operation: OpAddAlias,
		Data: CommandAddAlias{
			Rule: alias.Rule{Pattern: "^/memfs/b/(.+)$", Target: "/memfs/a/$1"},
		},
	})
	require.NoError(t, err)

	require.Equal(t, []alias.Rule{
		{Pattern: "^/memfs/b/(.+)$", Target: "/memfs/a/$1"},
	}, s.AliasList().Rules)
}

func TestAddAlias(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	updatedAt := s.AliasList().UpdatedAt

	err = s.addAlias(CommandAddAlias{Rule: alias.Rule{Pattern: "^/memfs/b/(.+)$", Target: "/memfs/a/$1"}})
	require.NoError(t, err)

	err = s.addAlias(CommandAddAlias{Rule: alias.Rule{Pattern: "^/memfs/c/(.+)$", Target: "/memfs/a/$1"}})
	require.NoError(t, err)

	aliases := s.AliasList()
	require.Greater(t, aliases.UpdatedAt, updatedAt)
	require.Equal(t, []alias.Rule{
		{Pattern: "^/memfs/b/(.+)$", Target: "/memfs/a/$1"},
		{Pattern: "^/memfs/c/(.+)$", Target: "/memfs/a/$1"},
	}, aliases.Rules)

	err = s.addAlias(CommandAddAlias{Rule: alias.Rule{Pattern: "^/memfs/b/(.+)$", Target: "/memfs/x/$1"}})
	require.ErrorIs(t, err, alias.ErrExists)
	require.ErrorIs(t, err, ErrConflict)

	err = s.addAlias(CommandAddAlias{Rule: alias.Rule{Pattern: "^/memfs/d/(.+$", Target: "/memfs/x/$1"}})
	require.ErrorIs(t, err, alias.ErrInvalid)
	require.ErrorIs(t, err, ErrBadRequest)

	err = s.addAlias(CommandAddAlias{Rule: alias.Rule{Pattern: "^/memfs/d/(.+)$", Target: "memfs/x/$1"}})
	require.ErrorIs(t, err, ErrBadRequest)

	require.Equal(t, aliases.Rules, s.AliasList().Rules)
}

func TestUpdateAlias(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	require.NoError(t, s.addAlias(CommandAddAlias{Rule: alias.Rule{Pattern: "^/memfs/b/(.+)$", Target: "/memfs/a/$1"}}))
	require.NoError(t, s.addAlias(CommandAddAlias{Rule: alias.Rule{Pattern: "^/memfs/c/(.+)$", Target: "/memfs/a/$1"}}))

	err = s.updateAlias(CommandUpdateAlias{
		Pattern: "^/memfs/b/(.+)$",
		Rule:    alias.Rule{Pattern: "^/memfs/d/(.+)$", Target: "/memfs/x/$1"},
	})
	require.NoError(t, err)

	// Same pattern
	err = s.updateAlias(CommandUpdateAlias{
		Pattern: "^/memfs/c/(.+)$",
		Rule:    alias.Rule{Pattern: "^/memfs/c/(.+)$", Target: "/memfs/y/$1"},
	})
	require.NoError(t, err)

	err = s.updateAlias(CommandUpdateAlias{
		Pattern: "^/memfs/b/(.+)$",
		Rule:    alias.Rule{Pattern: "^/memfs/e/(.+)$", Target: "/memfs/x/$1"},
	})
	require.ErrorIs(t, err, alias.ErrNotFound)
	require.ErrorIs(t, err, ErrNotFound)

	err = s.updateAlias(CommandUpdateAlias{
		Pattern: "^/memfs/d/(.+)$",
		Rule:    alias.Rule{Pattern: "^/memfs/c/(.+)$", Target: "/memfs/x/$1"},
	})
	require.ErrorIs(t, err, ErrConflict)

	err = s.updateAlias(CommandUpdateAlias{
		Pattern: "^/memfs/d/(.+)$",
		Rule:    alias.Rule{Pattern: "^/memfs/d/(.+)$", Target: "x/$1"},
	})
	require.ErrorIs(t, err, ErrBadRequest)

	require.Equal(t, []alias.Rule{
		{Pattern: "^/memfs/d/(.+)$", Target: "/memfs/x/$1"},
		{Pattern: "^/memfs/c/(.+)$", Target: "/memfs/y/$1"},
	}, s.AliasList().Rules)
}

func TestRemoveAlias(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	require.NoError(t, s.addAlias(CommandAddAlias{Rule: alias.Rule{Pattern: "^/memfs/b/(.+)$", Target: "/memfs/a/$1"}}))
	require.NoError(t, s.addAlias(CommandAddAlias{Rule: alias.Rule{Pattern: "^/memfs/c/(.+)$", Target: "/memfs/a/$1"}}))

	require.NoError(t, s.removeAlias(CommandRemoveAlias{Pattern: "^/memfs/b/(.+)$"}))

	err = s.removeAlias(CommandRemoveAlias{Pattern: "^/memfs/b/(.+)$"})
	require.ErrorIs(t, err, ErrNotFound)

	require.Equal(t, []alias.Rule{
		{Pattern: "^/memfs/c/(.+)$", Target: "/memfs/a/$1"},
	}, s.AliasList().Rules)
}

func TestSetAliases(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	require.NoError(t, s.addAlias(CommandAddAlias{Rule: alias.Rule{Pattern: "^/memfs/b/(.+)$", Target: "/memfs/a/$1"}}))

	rules := []alias.Rule{
		{Pattern: "^/memfs/d/(.+)$", Target: "/memfs/x/$1"},
		{Pattern: "^/memfs/c/(.+)$", Target: "/memfs/y/$1"},
	}

	require.NoError(t, s.setAliases(CommandSetAliases{Rules: rules}))
	require.Equal(t, rules, s.AliasList().Rules)

	err = s.setAliases(CommandSetAliases{Rules: []alias.Rule{rules[0], rules[0]}})
	require.ErrorIs(t, err, alias.ErrExists)
	require.ErrorIs(t, err, ErrConflict)

	err = s.setAliases(CommandSetAliases{Rules: []alias.Rule{rules[0], {Pattern: "(", Target: "/x"}}})
	require.ErrorIs(t, err, ErrBadRequest)

	require.Equal(t, rules, s.AliasList().Rules)

	require.NoError(t, s.setAliases(CommandSetAliases{Rules: nil}))
	require.Equal(t, []alias.Rule{}, s.AliasList().Rules)
}

func TestGetAlias(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	require.NoError(t, s.addAlias(CommandAddAlias{Rule: alias.Rule{Pattern: "^/memfs/b/(.+)$", Target: "/memfs/a/$1"}}))

	r, err := s.AliasGet("^/memfs/b/(.+)$")
	require.NoError(t, err)
	require.Equal(t, alias.Rule{Pattern: "^/memfs/b/(.+)$", Target: "/memfs/a/$1"}, r)

	_, err = s.AliasGet("^/memfs/c/(.+)$")
	require.ErrorIs(t, err, ErrNotFound)

	// The returned list is a copy
	aliases := s.AliasList()
	aliases.Rules[0].Target = "/memfs/x/$1"

	r, err = s.AliasGet("^/memfs/b/(.+)$")
	require.NoError(t, err)
	require.Equal(t, "/memfs/a/$1", r.Target)
}

func TestAliasSnapshotRestore(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	rules := []alias.Rule{
		{Pattern: "^/memfs/b/(.+)$", Target: "/memfs/a/$1"},
		{Pattern: "^/memfs/c/(.+)$", Target: "/memfs/a/$1"},
	}

	require.NoError(t, s.setAliases(CommandSetAliases{Rules: rules}))

	snapshot, err := s.Snapshot()
	require.NoError(t, err)

	data := snapshot.(*fsmSnapshot).data

	ops := []Operation{}

	r, err := createStore()
	require.NoError(t, err)

	r.OnApply(func(op Operation) { ops = append(ops, op) })

	require.NoError(t, r.Restore(io.NopCloser(bytes.NewReader(data))))
	require.Equal(t, rules, r.AliasList().Rules)
	require.Contains(t, ops, OpSetAliases)

	// Snapshot from before aliases existed
	old := map[string]any{}
	require.NoError(t, json.Unmarshal(data, &old))
	delete(old, "Aliases")

	data, err = json.Marshal(old)
	require.NoError(t, err)

	require.NoError(t, r.Restore(io.NopCloser(bytes.NewReader(data))))
	require.Equal(t, []alias.Rule{}, r.AliasList().Rules)

	// Snapshot with a null list
	old["Aliases"] = map[string]any{"Rules": nil}

	data, err = json.Marshal(old)
	require.NoError(t, err)

	require.NoError(t, r.Restore(io.NopCloser(bytes.NewReader(data))))
	require.Equal(t, []alias.Rule{}, r.AliasList().Rules)
}

func TestApplyMultipleCallbacks(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	var op1, op2 Operation

	s.OnApply(func(o Operation) { op1 = o })
	s.OnApply(func(o Operation) { op2 = o })

	cmd := Command{
		Operation: OpAddAlias,
		Data: CommandAddAlias{
			Rule: alias.Rule{Pattern: "^/memfs/b/(.+)$", Target: "/memfs/a/$1"},
		},
	}

	data, err := json.Marshal(&cmd)
	require.NoError(t, err)

	res := s.Apply(&raft.Log{
		Index: 1,
		Term:  1,
		Type:  raft.LogCommand,
		Data:  data,
	})
	require.Nil(t, res)

	require.Equal(t, OpAddAlias, op1)
	require.Equal(t, OpAddAlias, op2)
}
