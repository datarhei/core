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
			Mem:      65,
			MemTotal: 4 * 1024 * 1024 * 1024,
			CPULimit: 90,
			MemLimit: 90,
		},
		"node2": {
			NCPU:     1,
			CPU:      85,
			Mem:      11,
			MemTotal: 4 * 1024 * 1024 * 1024,
			CPULimit: 90,
			MemLimit: 90,
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

	require.Equal(t, map[string]NodeResources{
		"node1": {
			NCPU:     1,
			CPU:      17,
			Mem:      65 + (50. / (4. * 1024) * 100),
			MemTotal: 4 * 1024 * 1024 * 1024,
			CPULimit: 90,
			MemLimit: 90,
		},
		"node2": {
			NCPU:     1,
			CPU:      85,
			Mem:      11,
			MemTotal: 4 * 1024 * 1024 * 1024,
			CPULimit: 90,
			MemLimit: 90,
		},
	}, resources)
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
			CPULimit: 90,
			MemLimit: 90,
		},
		"node2": {
			NCPU:     1,
			CPU:      79,
			Mem:      72,
			MemTotal: 4 * 1024 * 1024 * 1024,
			CPULimit: 90,
			MemLimit: 90,
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

	require.Equal(t, map[string]NodeResources{
		"node1": {
			NCPU:     1,
			CPU:      81,
			Mem:      72,
			MemTotal: 4 * 1024 * 1024 * 1024,
			CPULimit: 90,
			MemLimit: 90,
		},
		"node2": {
			NCPU:     1,
			CPU:      89,
			Mem:      72 + (50. / (4. * 1024) * 100),
			MemTotal: 4 * 1024 * 1024 * 1024,
			CPULimit: 90,
			MemLimit: 90,
		},
	}, resources)
}

func TestSynchronizeAddNoResourcesCPU(t *testing.T) {
	want := []app.Config{
		{
			ID:          "foobar",
			LimitCPU:    30,
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
			CPULimit: 90,
			MemLimit: 90,
		},
		"node2": {
			NCPU:     1,
			CPU:      79,
			Mem:      72,
			MemTotal: 4 * 1024 * 1024 * 1024,
			CPULimit: 90,
			MemLimit: 90,
		},
	}

	stack := synchronize(want, have, resources)

	require.Equal(t, []interface{}{
		processOpReject{
			processid: "foobar",
			err:       errNotEnoughResources,
		},
	}, stack)
}

func TestSynchronizeAddNoResourcesMemory(t *testing.T) {
	want := []app.Config{
		{
			ID:          "foobar",
			LimitCPU:    1,
			LimitMemory: 2 * 1024 * 1024 * 1024,
		},
	}

	have := []ProcessConfig{}

	resources := map[string]NodeResources{
		"node1": {
			NCPU:     1,
			CPU:      81,
			Mem:      72,
			MemTotal: 4 * 1024 * 1024 * 1024,
			CPULimit: 90,
			MemLimit: 90,
		},
		"node2": {
			NCPU:     1,
			CPU:      79,
			Mem:      72,
			MemTotal: 4 * 1024 * 1024 * 1024,
			CPULimit: 90,
			MemLimit: 90,
		},
	}

	stack := synchronize(want, have, resources)

	require.Equal(t, []interface{}{
		processOpReject{
			processid: "foobar",
			err:       errNotEnoughResources,
		},
	}, stack)
}

func TestSynchronizeAddNoLimits(t *testing.T) {
	want := []app.Config{
		{
			ID: "foobar",
		},
	}

	have := []ProcessConfig{}

	resources := map[string]NodeResources{
		"node1": {
			NCPU:     1,
			CPU:      81,
			Mem:      72,
			MemTotal: 4 * 1024 * 1024 * 1024,
			CPULimit: 90,
			MemLimit: 90,
		},
		"node2": {
			NCPU:     1,
			CPU:      79,
			Mem:      72,
			MemTotal: 4 * 1024 * 1024 * 1024,
			CPULimit: 90,
			MemLimit: 90,
		},
	}

	stack := synchronize(want, have, resources)

	require.Equal(t, []interface{}{
		processOpReject{
			processid: "foobar",
			err:       errNoLimitsDefined,
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
			Mem:      65,
			MemTotal: 4 * 1024 * 1024 * 1024,
			CPULimit: 90,
			MemLimit: 90,
		},
		"node2": {
			NCPU:     1,
			CPU:      85,
			Mem:      11,
			MemTotal: 4 * 1024 * 1024 * 1024,
			CPULimit: 90,
			MemLimit: 90,
		},
	}

	stack := synchronize(want, have, resources)

	require.Equal(t, []interface{}{
		processOpDelete{
			nodeid:    "node2",
			processid: "foobar",
		},
	}, stack)

	require.Equal(t, map[string]NodeResources{
		"node1": {
			NCPU:     1,
			CPU:      7,
			Mem:      65,
			MemTotal: 4 * 1024 * 1024 * 1024,
			CPULimit: 90,
			MemLimit: 90,
		},
		"node2": {
			NCPU:     1,
			CPU:      73,
			Mem:      6,
			MemTotal: 4 * 1024 * 1024 * 1024,
			CPULimit: 90,
			MemLimit: 90,
		},
	}, resources)
}

func TestSynchronizeAddRemove(t *testing.T) {
	want := []app.Config{
		{
			ID:          "foobar1",
			LimitCPU:    10,
			LimitMemory: 50 * 1024 * 1024,
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
			Mem:      65,
			MemTotal: 4 * 1024 * 1024 * 1024,
			CPULimit: 90,
			MemLimit: 90,
		},
		"node2": {
			NCPU:     1,
			CPU:      85,
			Mem:      11,
			MemTotal: 4 * 1024 * 1024 * 1024,
			CPULimit: 90,
			MemLimit: 90,
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
				ID:          "foobar1",
				LimitCPU:    10,
				LimitMemory: 50 * 1024 * 1024,
			},
		},
	}, stack)

	require.Equal(t, map[string]NodeResources{
		"node1": {
			NCPU:     1,
			CPU:      17,
			Mem:      65 + (50. / (4. * 1024) * 100),
			MemTotal: 4 * 1024 * 1024 * 1024,
			CPULimit: 90,
			MemLimit: 90,
		},
		"node2": {
			NCPU:     1,
			CPU:      73,
			Mem:      6,
			MemTotal: 4 * 1024 * 1024 * 1024,
			CPULimit: 90,
			MemLimit: 90,
		},
	}, resources)
}

func TestRebalanceNothingToDo(t *testing.T) {
	processes := []ProcessConfig{
		{
			NodeID:  "node1",
			Order:   "start",
			State:   "running",
			CPU:     35,
			Mem:     20,
			Runtime: 42,
			Config: &app.Config{
				ID: "foobar1",
			},
		},
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
			CPU:      42,
			Mem:      35,
			MemTotal: 4 * 1024 * 1024 * 1024,
			CPULimit: 90,
			MemLimit: 90,
		},
		"node2": {
			NCPU:     1,
			CPU:      37,
			Mem:      11,
			MemTotal: 4 * 1024 * 1024 * 1024,
			CPULimit: 90,
			MemLimit: 90,
		},
	}

	opStack := rebalance(processes, resources)

	require.Empty(t, opStack)
}

func TestRebalanceOverload(t *testing.T) {
	processes := []ProcessConfig{
		{
			NodeID:  "node1",
			Order:   "start",
			State:   "running",
			CPU:     35,
			Mem:     20,
			Runtime: 42,
			Config: &app.Config{
				ID: "foobar1",
			},
		},
		{
			NodeID:  "node1",
			Order:   "start",
			State:   "running",
			CPU:     17,
			Mem:     31,
			Runtime: 27,
			Config: &app.Config{
				ID: "foobar3",
			},
		},
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
			CPU:      91,
			Mem:      35,
			MemTotal: 4 * 1024 * 1024 * 1024,
			CPULimit: 90,
			MemLimit: 90,
		},
		"node2": {
			NCPU:     1,
			CPU:      15,
			Mem:      11,
			MemTotal: 4 * 1024 * 1024 * 1024,
			CPULimit: 90,
			MemLimit: 90,
		},
	}

	opStack := rebalance(processes, resources)

	require.NotEmpty(t, opStack)

	require.Equal(t, []interface{}{
		processOpMove{
			fromNodeid: "node1",
			toNodeid:   "node2",
			config: &app.Config{
				ID: "foobar3",
			},
		},
	}, opStack)

	require.Equal(t, map[string]NodeResources{
		"node1": {
			NCPU:     1,
			CPU:      74,
			Mem:      4,
			MemTotal: 4 * 1024 * 1024 * 1024,
			CPULimit: 90,
			MemLimit: 90,
		},
		"node2": {
			NCPU:     1,
			CPU:      32,
			Mem:      42,
			MemTotal: 4 * 1024 * 1024 * 1024,
			CPULimit: 90,
			MemLimit: 90,
		},
	}, resources)
}

func TestRebalanceSkip(t *testing.T) {
	processes := []ProcessConfig{
		{
			NodeID:  "node1",
			Order:   "start",
			State:   "running",
			CPU:     35,
			Mem:     20,
			Runtime: 42,
			Config: &app.Config{
				ID: "foobar1",
			},
		},
		{
			NodeID:  "node1",
			Order:   "start",
			State:   "running",
			CPU:     17,
			Mem:     31,
			Runtime: 27,
			Config: &app.Config{
				ID: "foobar3",
			},
		},
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
			CPU:      91,
			Mem:      35,
			MemTotal: 4 * 1024 * 1024 * 1024,
			CPULimit: 90,
			MemLimit: 90,
		},
		"node2": {
			NCPU:     1,
			CPU:      15,
			Mem:      92,
			MemTotal: 4 * 1024 * 1024 * 1024,
			CPULimit: 90,
			MemLimit: 90,
		},
	}

	opStack := rebalance(processes, resources)

	require.NotEmpty(t, opStack)

	require.Equal(t, []interface{}{
		processOpSkip{
			nodeid:    "node1",
			processid: "foobar3",
			err:       errNotEnoughResourcesForRebalancing,
		},
		processOpSkip{
			nodeid:    "node1",
			processid: "foobar1",
			err:       errNotEnoughResourcesForRebalancing,
		},
		processOpSkip{
			nodeid:    "node2",
			processid: "foobar2",
			err:       errNotEnoughResourcesForRebalancing,
		},
	}, opStack)

	require.Equal(t, map[string]NodeResources{
		"node1": {
			NCPU:     1,
			CPU:      91,
			Mem:      35,
			MemTotal: 4 * 1024 * 1024 * 1024,
			CPULimit: 90,
			MemLimit: 90,
		},
		"node2": {
			NCPU:     1,
			CPU:      15,
			Mem:      92,
			MemTotal: 4 * 1024 * 1024 * 1024,
			CPULimit: 90,
			MemLimit: 90,
		},
	}, resources)
}
