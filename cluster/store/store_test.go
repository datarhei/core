package store

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/raft"

	"github.com/stretchr/testify/require"
)

func createStore() (*store, error) {
	si, err := NewStore(Config{
		Logger: nil,
	})
	if err != nil {
		return nil, err
	}

	s := si.(*store)

	return s, nil
}

func TestCreateStore(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	require.NotNil(t, s.data.Process)
	require.NotNil(t, s.data.ProcessNodeMap)
	require.NotNil(t, s.data.Users.Users)
	require.NotNil(t, s.data.Policies.Policies)
	require.NotNil(t, s.data.Locks)
	require.NotNil(t, s.data.KVS)
}

func TestApplyCommand(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	err = s.applyCommand(Command{
		Operation: "unknown",
		Data:      nil,
	})
	require.Error(t, err)
}

func TestApply(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	entry := &raft.Log{
		Index: 1,
		Term:  1,
		Type:  raft.LogCommand,
		Data:  []byte("123"),
	}

	res := s.Apply(entry)
	require.NotNil(t, res)

	cmd := Command{
		Operation: "unknown",
		Data:      nil,
	}

	data, err := json.Marshal(&cmd)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	entry.Data = data

	res = s.Apply(entry)
	require.NotNil(t, res)

	cmd = Command{
		Operation: OpSetProcessNodeMap,
		Data: CommandSetProcessNodeMap{
			Map: nil,
		},
	}

	data, err = json.Marshal(&cmd)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	entry.Data = data

	res = s.Apply(entry)
	require.Nil(t, res)
}

func TestApplyWithCallback(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	var op Operation

	s.OnApply(func(o Operation) {
		op = o
	})

	cmd := Command{
		Operation: OpSetProcessNodeMap,
		Data: CommandSetProcessNodeMap{
			Map: nil,
		},
	}

	data, err := json.Marshal(&cmd)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	entry := &raft.Log{
		Index: 1,
		Term:  1,
		Type:  raft.LogCommand,
		Data:  data,
	}

	res := s.Apply(entry)
	require.Nil(t, res)

	require.Equal(t, OpSetProcessNodeMap, op)
}

func TestSnapshot(t *testing.T) {
	s, err := createStore()
	require.NoError(t, err)

	snapshot, err := s.Snapshot()
	require.NoError(t, err)

	sshot := snapshot.(*fsmSnapshot)

	data, err := json.Marshal(s.data)
	require.NoError(t, err)

	require.Equal(t, data, sshot.data)

	snapshot.Release()

	require.Equal(t, []byte(nil), sshot.data)
}
