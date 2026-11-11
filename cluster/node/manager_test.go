package node

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetRandomNodeNoNodes(t *testing.T) {
	m, err := NewManager(ManagerConfig{ID: "foo"})
	require.NoError(t, err)

	require.NotPanics(t, func() {
		nodeid, err := m.GetRandomNode()
		require.ErrorIs(t, err, ErrNoNodes)
		require.Equal(t, "", nodeid)
	})
}

func TestGetRandomNode(t *testing.T) {
	m, err := NewManager(ManagerConfig{ID: "foo"})
	require.NoError(t, err)

	m.nodes["node1"] = &Node{}
	m.nodes["node2"] = &Node{}

	for i := 0; i < 20; i++ {
		nodeid, err := m.GetRandomNode()
		require.NoError(t, err)
		require.Contains(t, []string{"node1", "node2"}, nodeid)
	}
}
