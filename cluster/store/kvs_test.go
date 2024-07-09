package store

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetKVCommand(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	err = s.applyCommand(Command{
		Operation: OpSetKV,
		Data: CommandSetKV{
			Key:   "foo",
			Value: "bar",
		},
	})
	require.NoError(t, err)

	_, ok := s.data.KVS["foo"]
	require.True(t, ok)
}

func TestSetKV(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	err = s.setKV(CommandSetKV{
		Key:   "foo",
		Value: "bar",
	})
	require.NoError(t, err)

	value, err := s.KVSGetValue("foo")
	require.NoError(t, err)
	require.Equal(t, "bar", value.Value)

	updatedAt := value.UpdatedAt

	err = s.setKV(CommandSetKV{
		Key:   "foo",
		Value: "baz",
	})
	require.NoError(t, err)

	value, err = s.KVSGetValue("foo")
	require.NoError(t, err)
	require.Equal(t, "baz", value.Value)
	require.Greater(t, value.UpdatedAt, updatedAt)
}

func TestUnsetKVCommand(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	err = s.applyCommand(Command{
		Operation: OpSetKV,
		Data: CommandSetKV{
			Key:   "foo",
			Value: "bar",
		},
	})
	require.NoError(t, err)

	_, ok := s.data.KVS["foo"]
	require.True(t, ok)

	err = s.applyCommand(Command{
		Operation: OpUnsetKV,
		Data: CommandUnsetKV{
			Key: "foo",
		},
	})
	require.NoError(t, err)

	_, ok = s.data.KVS["foo"]
	require.False(t, ok)

	err = s.applyCommand(Command{
		Operation: OpUnsetKV,
		Data: CommandUnsetKV{
			Key: "foo",
		},
	})
	require.Error(t, err)
	require.ErrorIs(t, err, fs.ErrNotExist)
}

func TestUnsetKV(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	err = s.setKV(CommandSetKV{
		Key:   "foo",
		Value: "bar",
	})
	require.NoError(t, err)

	_, err = s.KVSGetValue("foo")
	require.NoError(t, err)

	err = s.unsetKV(CommandUnsetKV{
		Key: "foo",
	})
	require.NoError(t, err)

	_, err = s.KVSGetValue("foo")
	require.Error(t, err)
	require.ErrorIs(t, err, fs.ErrNotExist)
}
