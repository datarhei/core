package cluster

import (
	"testing"

	"github.com/datarhei/core/v16/restream/app"
	"github.com/stretchr/testify/require"
)

func TestNormalize(t *testing.T) {
	have := []ProcessConfig{
		{
			NodeID:  "node2",
			Order:   "start",
			State:   "running",
			CPU:     12,
			Mem:     5,
			Runtime: 42,
			Config: &app.Config{
				ID: "foobar",
			},
		},
	}

	resources := map[string]NodeResources{
		"node1": {
			NCPU:     2,
			CPU:      7,
			Mem:      35,
			MemTotal: 2 * 1024 * 1024 * 1024, // 2GB
		},
		"node2": {
			NCPU:     1,
			CPU:      75,
			Mem:      11,
			MemTotal: 4 * 1024 * 1024 * 1024, // 4GB
		},
	}

	normalizeProcessesAndResources(have, resources)

	require.Equal(t, []ProcessConfig{
		{
			NodeID:  "node2",
			Order:   "start",
			State:   "running",
			CPU:     56,
			Mem:     5,
			Runtime: 42,
			Config: &app.Config{
				ID: "foobar",
			},
		},
	}, have)

	require.Equal(t, map[string]NodeResources{
		"node1": {
			NCPU:     2,
			CPU:      7,
			Mem:      67.5,
			MemTotal: 4 * 1024 * 1024 * 1024,
		},
		"node2": {
			NCPU:     2,
			CPU:      87.5,
			Mem:      11,
			MemTotal: 4 * 1024 * 1024 * 1024,
		},
	}, resources)

	// test idempotency
	normalizeProcessesAndResources(have, resources)

	require.Equal(t, []ProcessConfig{
		{
			NodeID:  "node2",
			Order:   "start",
			State:   "running",
			CPU:     56,
			Mem:     5,
			Runtime: 42,
			Config: &app.Config{
				ID: "foobar",
			},
		},
	}, have)

	require.Equal(t, map[string]NodeResources{
		"node1": {
			NCPU:     2,
			CPU:      7,
			Mem:      67.5,
			MemTotal: 4 * 1024 * 1024 * 1024,
		},
		"node2": {
			NCPU:     2,
			CPU:      87.5,
			Mem:      11,
			MemTotal: 4 * 1024 * 1024 * 1024,
		},
	}, resources)
}

func TestSynchronizeAdd(t *testing.T) {
	want := []app.Config{
		{
			ID:          "foobar",
			LimitCPU:    10,
			LimitMemory: 50 * 1024 * 1024,
		},
	}

	have := []ProcessConfig{}

	resources := map[string]NodeResources{
		"node1": {
			NCPU:     1,
			CPU:      7,
			Mem:      67.5,
			MemTotal: 4 * 1024 * 1024 * 1024,
		},
		"node2": {
			NCPU:     1,
			CPU:      87.5,
			Mem:      11,
			MemTotal: 4 * 1024 * 1024 * 1024,
		},
	}

	stack := synchronize(want, have, resources)

	require.Equal(t, []interface{}{
		processOpAdd{
			nodeid: "node1",
			config: &app.Config{
				ID:          "foobar",
				LimitCPU:    10,
				LimitMemory: 50 * 1024 * 1024,
			},
		},
	}, stack)
}

func TestSynchronizeAddLimit(t *testing.T) {
	want := []app.Config{
		{
			ID:          "foobar",
			LimitCPU:    10,
			LimitMemory: 50 * 1024 * 1024,
		},
	}

	have := []ProcessConfig{}

	resources := map[string]NodeResources{
		"node1": {
			NCPU:     1,
			CPU:      81,
			Mem:      72,
			MemTotal: 4 * 1024 * 1024 * 1024,
		},
		"node2": {
			NCPU:     1,
			CPU:      79,
			Mem:      72,
			MemTotal: 4 * 1024 * 1024 * 1024,
		},
	}

	stack := synchronize(want, have, resources)

	require.Equal(t, []interface{}{
		processOpAdd{
			nodeid: "node2",
			config: &app.Config{
				ID:          "foobar",
				LimitCPU:    10,
				LimitMemory: 50 * 1024 * 1024,
			},
		},
	}, stack)
}

func TestSynchronizeRemove(t *testing.T) {
	want := []app.Config{}

	have := []ProcessConfig{
		{
			NodeID:  "node2",
			Order:   "start",
			State:   "running",
			CPU:     12,
			Mem:     5,
			Runtime: 42,
			Config: &app.Config{
				ID: "foobar",
			},
		},
	}

	resources := map[string]NodeResources{
		"node1": {
			NCPU:     1,
			CPU:      7,
			Mem:      67.5,
			MemTotal: 4 * 1024 * 1024 * 1024,
		},
		"node2": {
			NCPU:     1,
			CPU:      87.5,
			Mem:      11,
			MemTotal: 4 * 1024 * 1024 * 1024,
		},
	}

	stack := synchronize(want, have, resources)

	require.Equal(t, map[string]NodeResources{
		"node1": {
			NCPU:     1,
			CPU:      7,
			Mem:      67.5,
			MemTotal: 4 * 1024 * 1024 * 1024,
		},
		"node2": {
			NCPU:     1,
			CPU:      75.5,
			Mem:      6,
			MemTotal: 4 * 1024 * 1024 * 1024,
		},
	}, resources)

	require.Equal(t, []interface{}{
		processOpDelete{
			nodeid:    "node2",
			processid: "foobar",
		},
	}, stack)
}

func TestSynchronizeAddRemove(t *testing.T) {
	want := []app.Config{
		{
			ID: "foobar1",
		},
	}

	have := []ProcessConfig{
		{
			NodeID:  "node2",
			Order:   "start",
			State:   "running",
			CPU:     12,
			Mem:     5,
			Runtime: 42,
			Config: &app.Config{
				ID: "foobar2",
			},
		},
	}

	resources := map[string]NodeResources{
		"node1": {
			NCPU:     1,
			CPU:      7,
			Mem:      67.5,
			MemTotal: 4 * 1024 * 1024 * 1024,
		},
		"node2": {
			NCPU:     1,
			CPU:      87.5,
			Mem:      11,
			MemTotal: 4 * 1024 * 1024 * 1024,
		},
	}

	stack := synchronize(want, have, resources)

	require.Equal(t, []interface{}{
		processOpDelete{
			nodeid:    "node2",
			processid: "foobar2",
		},
		processOpAdd{
			nodeid: "node1",
			config: &app.Config{
				ID: "foobar1",
			},
		},
	}, stack)
}
