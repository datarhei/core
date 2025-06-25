package psutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChildren(t *testing.T) {
	p := &procfs{
		children: map[int32][]int32{
			0: {1},
			1: {2, 3},
			2: {4, 5, 6},
			3: {7, 8, 9},
		},
	}

	require.Equal(t, []int32{1}, p.Children(0))
	require.Equal(t, []int32{2, 3}, p.Children(1))
	require.Equal(t, []int32{4, 5, 6}, p.Children(2))
	require.Equal(t, []int32{7, 8, 9}, p.Children(3))
	require.Equal(t, []int32{}, p.Children(4))
}

func TestAllChildren(t *testing.T) {
	p := &procfs{
		children: map[int32][]int32{
			0: {1},
			1: {2, 3},
			2: {4, 5, 6},
			3: {7, 8, 9},
		},
	}

	require.Equal(t, []int32{1, 2, 3, 4, 5, 6, 7, 8, 9}, p.AllChildren(0))
	require.Equal(t, []int32{2, 3, 4, 5, 6, 7, 8, 9}, p.AllChildren(1))
	require.Equal(t, []int32{4, 5, 6}, p.AllChildren(2))
	require.Equal(t, []int32{7, 8, 9}, p.AllChildren(3))
	require.Equal(t, []int32{}, p.AllChildren(4))
}
