package cluster

import (
	"testing"
	"time"

	"github.com/datarhei/core/v16/cluster/proxy"
	"github.com/datarhei/core/v16/cluster/store"
	"github.com/datarhei/core/v16/restream/app"

	"github.com/stretchr/testify/require"
)

func TestSynchronizeAdd(t *testing.T) {
	wish := map[string]string{}

	want := []store.Process{
		{
			UpdatedAt: time.Now(),
			Config: &app.Config{
				ID:          "foobar",
				LimitCPU:    10,
				LimitMemory: 50,
			},
		},
	}

	have := []proxy.Process{}

	nodes := map[string]proxy.NodeAbout{
		"node1": {
			LastContact: time.Now(),
			Resources: proxy.NodeResources{
				NCPU:     1,
				CPU:      7,
				Mem:      35,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			LastContact: time.Now(),
			Resources: proxy.NodeResources{
				NCPU:     1,
				CPU:      85,
				Mem:      11,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
	}

	stack, resources, reality := synchronize(wish, want, have, nodes, 2*time.Minute)

	require.Equal(t, []interface{}{
		processOpAdd{
			nodeid: "node1",
			config: &app.Config{
				ID:          "foobar",
				LimitCPU:    10,
				LimitMemory: 50,
			},
		},
	}, stack)

	require.Equal(t, map[string]string{
		"foobar@": "node1",
	}, reality)

	require.Equal(t, map[string]proxy.NodeResources{
		"node1": {
			NCPU:     1,
			CPU:      17,
			Mem:      85,
			CPULimit: 90,
			MemLimit: 90,
		},
		"node2": {
			NCPU:     1,
			CPU:      85,
			Mem:      11,
			CPULimit: 90,
			MemLimit: 90,
		},
	}, resources)
}

func TestSynchronizeAddReferenceAffinity(t *testing.T) {
	wish := map[string]string{
		"foobar@": "node2",
	}

	now := time.Now()

	want := []store.Process{
		{
			UpdatedAt: now,
			Config: &app.Config{
				ID:          "foobar",
				Reference:   "barfoo",
				LimitCPU:    10,
				LimitMemory: 20,
			},
		},
		{
			UpdatedAt: now,
			Config: &app.Config{
				ID:          "foobar2",
				Reference:   "barfoo",
				LimitCPU:    10,
				LimitMemory: 30,
			},
		},
	}

	have := []proxy.Process{
		{
			NodeID:    "node2",
			Order:     "start",
			State:     "running",
			CPU:       12,
			Mem:       5,
			Runtime:   42,
			UpdatedAt: now,
			Config: &app.Config{
				ID:          "foobar",
				Reference:   "barfoo",
				LimitCPU:    10,
				LimitMemory: 20,
			},
		},
	}

	nodes := map[string]proxy.NodeAbout{
		"node1": {
			LastContact: time.Now(),
			Resources: proxy.NodeResources{
				NCPU:     1,
				CPU:      1,
				Mem:      1,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			LastContact: time.Now(),
			Resources: proxy.NodeResources{
				NCPU:     1,
				CPU:      1,
				Mem:      1,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
	}

	stack, _, reality := synchronize(wish, want, have, nodes, 2*time.Minute)

	require.Equal(t, []interface{}{
		processOpAdd{
			nodeid: "node2",
			config: &app.Config{
				ID:          "foobar2",
				Reference:   "barfoo",
				LimitCPU:    10,
				LimitMemory: 30,
			},
		},
	}, stack)

	require.Equal(t, map[string]string{
		"foobar@":  "node2",
		"foobar2@": "node2",
	}, reality)
}

func TestSynchronizeAddLimit(t *testing.T) {
	wish := map[string]string{}

	want := []store.Process{
		{
			UpdatedAt: time.Now(),
			Config: &app.Config{
				ID:          "foobar",
				LimitCPU:    10,
				LimitMemory: 5,
			},
		},
	}

	have := []proxy.Process{}

	nodes := map[string]proxy.NodeAbout{
		"node1": {
			LastContact: time.Now(),
			Resources: proxy.NodeResources{
				NCPU:     1,
				CPU:      81,
				Mem:      72,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			LastContact: time.Now(),
			Resources: proxy.NodeResources{
				NCPU:     1,
				CPU:      79,
				Mem:      72,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
	}

	stack, resources, reality := synchronize(wish, want, have, nodes, 2*time.Minute)

	require.Equal(t, []interface{}{
		processOpAdd{
			nodeid: "node2",
			config: &app.Config{
				ID:          "foobar",
				LimitCPU:    10,
				LimitMemory: 5,
			},
		},
	}, stack)

	require.Equal(t, map[string]string{
		"foobar@": "node2",
	}, reality)

	require.Equal(t, map[string]proxy.NodeResources{
		"node1": {
			NCPU:     1,
			CPU:      81,
			Mem:      72,
			CPULimit: 90,
			MemLimit: 90,
		},
		"node2": {
			NCPU:     1,
			CPU:      89,
			Mem:      77,
			CPULimit: 90,
			MemLimit: 90,
		},
	}, resources)
}

func TestSynchronizeAddNoResourcesCPU(t *testing.T) {
	wish := map[string]string{}

	want := []store.Process{
		{
			UpdatedAt: time.Now(),
			Config: &app.Config{
				ID:          "foobar",
				LimitCPU:    30,
				LimitMemory: 5,
			},
		},
	}

	have := []proxy.Process{}

	nodes := map[string]proxy.NodeAbout{
		"node1": {
			LastContact: time.Now(),
			Resources: proxy.NodeResources{
				NCPU:     1,
				CPU:      81,
				Mem:      72,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			LastContact: time.Now(),
			Resources: proxy.NodeResources{
				NCPU:     1,
				CPU:      79,
				Mem:      72,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
	}

	stack, _, _ := synchronize(wish, want, have, nodes, 2*time.Minute)

	require.Equal(t, []interface{}{
		processOpReject{
			processid: app.ProcessID{ID: "foobar"},
			err:       errNotEnoughResources,
		},
	}, stack)
}

func TestSynchronizeAddNoResourcesMemory(t *testing.T) {
	wish := map[string]string{}

	want := []store.Process{
		{
			UpdatedAt: time.Now(),
			Config: &app.Config{
				ID:          "foobar",
				LimitCPU:    1,
				LimitMemory: 50,
			},
		},
	}

	have := []proxy.Process{}

	nodes := map[string]proxy.NodeAbout{
		"node1": {
			LastContact: time.Now(),
			Resources: proxy.NodeResources{
				NCPU:     1,
				CPU:      81,
				Mem:      72,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			LastContact: time.Now(),
			Resources: proxy.NodeResources{
				NCPU:     1,
				CPU:      79,
				Mem:      72,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
	}

	stack, _, _ := synchronize(wish, want, have, nodes, 2*time.Minute)

	require.Equal(t, []interface{}{
		processOpReject{
			processid: app.ProcessID{ID: "foobar"},
			err:       errNotEnoughResources,
		},
	}, stack)
}

func TestSynchronizeAddNoLimits(t *testing.T) {
	wish := map[string]string{}

	want := []store.Process{
		{
			UpdatedAt: time.Now(),
			Config: &app.Config{
				ID: "foobar",
			},
		},
	}

	have := []proxy.Process{}

	nodes := map[string]proxy.NodeAbout{
		"node1": {
			LastContact: time.Now(),
			Resources: proxy.NodeResources{
				NCPU:     1,
				CPU:      81,
				Mem:      72,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			LastContact: time.Now(),
			Resources: proxy.NodeResources{
				NCPU:     1,
				CPU:      79,
				Mem:      72,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
	}

	stack, _, _ := synchronize(wish, want, have, nodes, 2*time.Minute)

	require.Equal(t, []interface{}{
		processOpReject{
			processid: app.ProcessID{ID: "foobar"},
			err:       errNoLimitsDefined,
		},
	}, stack)
}

func TestSynchronizeRemove(t *testing.T) {
	wish := map[string]string{
		"foobar@": "node2",
	}

	want := []store.Process{}

	have := []proxy.Process{
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

	nodes := map[string]proxy.NodeAbout{
		"node1": {
			LastContact: time.Now(),
			Resources: proxy.NodeResources{
				NCPU:     1,
				CPU:      7,
				Mem:      65,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			LastContact: time.Now(),
			Resources: proxy.NodeResources{
				NCPU:     1,
				CPU:      85,
				Mem:      11,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
	}

	stack, resources, reality := synchronize(wish, want, have, nodes, 2*time.Minute)

	require.Equal(t, []interface{}{
		processOpDelete{
			nodeid:    "node2",
			processid: app.ProcessID{ID: "foobar"},
		},
	}, stack)

	require.Equal(t, map[string]proxy.NodeResources{
		"node1": {
			NCPU:     1,
			CPU:      7,
			Mem:      65,
			CPULimit: 90,
			MemLimit: 90,
		},
		"node2": {
			NCPU:     1,
			CPU:      73,
			Mem:      6,
			CPULimit: 90,
			MemLimit: 90,
		},
	}, resources)

	require.Equal(t, map[string]string{}, reality)
}

func TestSynchronizeAddRemove(t *testing.T) {
	wish := map[string]string{
		"foobar2@": "node2",
	}

	want := []store.Process{
		{
			UpdatedAt: time.Now(),
			Config: &app.Config{
				ID:          "foobar1",
				LimitCPU:    10,
				LimitMemory: 5,
			},
		},
	}

	have := []proxy.Process{
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

	nodes := map[string]proxy.NodeAbout{
		"node1": {
			LastContact: time.Now(),
			Resources: proxy.NodeResources{
				NCPU:     1,
				CPU:      7,
				Mem:      35,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			LastContact: time.Now(),
			Resources: proxy.NodeResources{
				NCPU:     1,
				CPU:      85,
				Mem:      65,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
	}

	stack, resources, reality := synchronize(wish, want, have, nodes, 2*time.Minute)

	require.Equal(t, []interface{}{
		processOpDelete{
			nodeid:    "node2",
			processid: app.ProcessID{ID: "foobar2"},
		},
		processOpAdd{
			nodeid: "node1",
			config: &app.Config{
				ID:          "foobar1",
				LimitCPU:    10,
				LimitMemory: 5,
			},
		},
	}, stack)

	require.Equal(t, map[string]proxy.NodeResources{
		"node1": {
			NCPU:     1,
			CPU:      17,
			Mem:      40,
			CPULimit: 90,
			MemLimit: 90,
		},
		"node2": {
			NCPU:     1,
			CPU:      73,
			Mem:      60,
			CPULimit: 90,
			MemLimit: 90,
		},
	}, resources)

	require.Equal(t, map[string]string{
		"foobar1@": "node1",
	}, reality)
}

func TestRebalanceNothingToDo(t *testing.T) {
	processes := []proxy.Process{
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

	nodes := map[string]proxy.NodeAbout{
		"node1": {
			LastContact: time.Now(),
			Resources: proxy.NodeResources{
				NCPU:     1,
				CPU:      42,
				Mem:      35,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			LastContact: time.Now(),
			Resources: proxy.NodeResources{
				NCPU:     1,
				CPU:      37,
				Mem:      11,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
	}

	opStack, _ := rebalance(processes, nodes)

	require.Empty(t, opStack)
}

func TestRebalanceOverload(t *testing.T) {
	processes := []proxy.Process{
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

	nodes := map[string]proxy.NodeAbout{
		"node1": {
			LastContact: time.Now(),
			Resources: proxy.NodeResources{
				NCPU:     1,
				CPU:      91,
				Mem:      35,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			LastContact: time.Now(),
			Resources: proxy.NodeResources{
				NCPU:     1,
				CPU:      15,
				Mem:      11,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
	}

	opStack, resources := rebalance(processes, nodes)

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

	require.Equal(t, map[string]proxy.NodeResources{
		"node1": {
			NCPU:     1,
			CPU:      74,
			Mem:      4,
			CPULimit: 90,
			MemLimit: 90,
		},
		"node2": {
			NCPU:     1,
			CPU:      32,
			Mem:      42,
			CPULimit: 90,
			MemLimit: 90,
		},
	}, resources)
}

func TestRebalanceSkip(t *testing.T) {
	processes := []proxy.Process{
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

	nodes := map[string]proxy.NodeAbout{
		"node1": {
			LastContact: time.Now(),
			Resources: proxy.NodeResources{
				NCPU:     1,
				CPU:      91,
				Mem:      35,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			LastContact: time.Now(),
			Resources: proxy.NodeResources{
				NCPU:     1,
				CPU:      15,
				Mem:      92,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
	}

	opStack, resources := rebalance(processes, nodes)

	require.NotEmpty(t, opStack)

	require.ElementsMatch(t, []interface{}{
		processOpSkip{
			nodeid:    "node1",
			processid: app.ProcessID{ID: "foobar3"},
			err:       errNotEnoughResourcesForRebalancing,
		},
		processOpSkip{
			nodeid:    "node1",
			processid: app.ProcessID{ID: "foobar1"},
			err:       errNotEnoughResourcesForRebalancing,
		},
		processOpSkip{
			nodeid:    "node2",
			processid: app.ProcessID{ID: "foobar2"},
			err:       errNotEnoughResourcesForRebalancing,
		},
	}, opStack)

	require.Equal(t, map[string]proxy.NodeResources{
		"node1": {
			NCPU:     1,
			CPU:      91,
			Mem:      35,
			CPULimit: 90,
			MemLimit: 90,
		},
		"node2": {
			NCPU:     1,
			CPU:      15,
			Mem:      92,
			CPULimit: 90,
			MemLimit: 90,
		},
	}, resources)
}

func TestRebalanceReferenceAffinity(t *testing.T) {
	processes := []proxy.Process{
		{
			NodeID:  "node1",
			Order:   "start",
			State:   "running",
			CPU:     1,
			Mem:     1,
			Runtime: 42,
			Config: &app.Config{
				ID: "foobar1",
			},
		},
		{
			NodeID:  "node1",
			Order:   "start",
			State:   "running",
			CPU:     1,
			Mem:     1,
			Runtime: 1,
			Config: &app.Config{
				ID:        "foobar2",
				Reference: "barfoo",
			},
		},
		{
			NodeID:  "node2",
			Order:   "start",
			State:   "running",
			CPU:     1,
			Mem:     1,
			Runtime: 42,
			Config: &app.Config{
				ID:        "foobar3",
				Reference: "barfoo",
			},
		},
		{
			NodeID:  "node3",
			Order:   "start",
			State:   "running",
			CPU:     1,
			Mem:     1,
			Runtime: 42,
			Config: &app.Config{
				ID:        "foobar4",
				Reference: "barfoo",
			},
		},
		{
			NodeID:  "node3",
			Order:   "start",
			State:   "running",
			CPU:     1,
			Mem:     1,
			Runtime: 42,
			Config: &app.Config{
				ID:        "foobar5",
				Reference: "barfoo",
			},
		},
	}

	nodes := map[string]proxy.NodeAbout{
		"node1": {
			LastContact: time.Now(),
			Resources: proxy.NodeResources{
				NCPU:     1,
				CPU:      90,
				Mem:      90,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			LastContact: time.Now(),
			Resources: proxy.NodeResources{
				NCPU:     1,
				CPU:      1,
				Mem:      1,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node3": {
			LastContact: time.Now(),
			Resources: proxy.NodeResources{
				NCPU:     1,
				CPU:      1,
				Mem:      1,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
	}

	opStack, resources := rebalance(processes, nodes)

	require.NotEmpty(t, opStack)

	require.Equal(t, []interface{}{
		processOpMove{
			fromNodeid: "node1",
			toNodeid:   "node3",
			config: &app.Config{
				ID:        "foobar2",
				Reference: "barfoo",
			},
		},
	}, opStack)

	require.Equal(t, map[string]proxy.NodeResources{
		"node1": {
			NCPU:     1,
			CPU:      89,
			Mem:      89,
			CPULimit: 90,
			MemLimit: 90,
		},
		"node2": {
			NCPU:     1,
			CPU:      1,
			Mem:      1,
			CPULimit: 90,
			MemLimit: 90,
		},
		"node3": {
			NCPU:     1,
			CPU:      2,
			Mem:      2,
			CPULimit: 90,
			MemLimit: 90,
		},
	}, resources)
}

func TestCreateNodeProcessMap(t *testing.T) {
	processes := []proxy.Process{
		{
			NodeID:  "node1",
			Order:   "start",
			State:   "running",
			CPU:     1,
			Mem:     1,
			Runtime: 42,
			Config: &app.Config{
				ID: "foobar1",
			},
		},
		{
			NodeID:  "node1",
			Order:   "start",
			State:   "running",
			CPU:     1,
			Mem:     1,
			Runtime: 1,
			Config: &app.Config{
				ID:        "foobar2",
				Reference: "ref1",
			},
		},
		{
			NodeID:  "node2",
			Order:   "start",
			State:   "running",
			CPU:     1,
			Mem:     1,
			Runtime: 67,
			Config: &app.Config{
				ID:        "foobar3",
				Reference: "ref3",
			},
		},
		{
			NodeID:  "node2",
			Order:   "start",
			State:   "running",
			CPU:     1,
			Mem:     1,
			Runtime: 42,
			Config: &app.Config{
				ID:        "foobar3",
				Reference: "ref2",
			},
		},
		{
			NodeID:  "node3",
			Order:   "start",
			State:   "running",
			CPU:     1,
			Mem:     1,
			Runtime: 41,
			Config: &app.Config{
				ID:        "foobar4",
				Reference: "ref1",
			},
		},
		{
			NodeID:  "node3",
			Order:   "start",
			State:   "running",
			CPU:     1,
			Mem:     1,
			Runtime: 42,
			Config: &app.Config{
				ID:        "foobar5",
				Reference: "ref1",
			},
		},
	}

	nodeProcessMap := createNodeProcessMap(processes)

	require.Equal(t, map[string][]proxy.Process{
		"node1": {
			{
				NodeID:  "node1",
				Order:   "start",
				State:   "running",
				CPU:     1,
				Mem:     1,
				Runtime: 1,
				Config: &app.Config{
					ID:        "foobar2",
					Reference: "ref1",
				},
			},
			{
				NodeID:  "node1",
				Order:   "start",
				State:   "running",
				CPU:     1,
				Mem:     1,
				Runtime: 42,
				Config: &app.Config{
					ID: "foobar1",
				},
			},
		},
		"node2": {
			{
				NodeID:  "node2",
				Order:   "start",
				State:   "running",
				CPU:     1,
				Mem:     1,
				Runtime: 42,
				Config: &app.Config{
					ID:        "foobar3",
					Reference: "ref2",
				},
			},
			{
				NodeID:  "node2",
				Order:   "start",
				State:   "running",
				CPU:     1,
				Mem:     1,
				Runtime: 67,
				Config: &app.Config{
					ID:        "foobar3",
					Reference: "ref3",
				},
			},
		},
		"node3": {
			{
				NodeID:  "node3",
				Order:   "start",
				State:   "running",
				CPU:     1,
				Mem:     1,
				Runtime: 41,
				Config: &app.Config{
					ID:        "foobar4",
					Reference: "ref1",
				},
			},
			{
				NodeID:  "node3",
				Order:   "start",
				State:   "running",
				CPU:     1,
				Mem:     1,
				Runtime: 42,
				Config: &app.Config{
					ID:        "foobar5",
					Reference: "ref1",
				},
			},
		},
	}, nodeProcessMap)
}

func TestCreateReferenceAffinityNodeMap(t *testing.T) {
	processes := []proxy.Process{
		{
			NodeID:  "node1",
			Order:   "start",
			State:   "running",
			CPU:     1,
			Mem:     1,
			Runtime: 42,
			Config: &app.Config{
				ID: "foobar1",
			},
		},
		{
			NodeID:  "node1",
			Order:   "start",
			State:   "running",
			CPU:     1,
			Mem:     1,
			Runtime: 1,
			Config: &app.Config{
				ID:        "foobar2",
				Reference: "ref1",
			},
		},
		{
			NodeID:  "node2",
			Order:   "start",
			State:   "running",
			CPU:     1,
			Mem:     1,
			Runtime: 42,
			Config: &app.Config{
				ID:        "foobar3",
				Reference: "ref3",
			},
		},
		{
			NodeID:  "node2",
			Order:   "start",
			State:   "running",
			CPU:     1,
			Mem:     1,
			Runtime: 42,
			Config: &app.Config{
				ID:        "foobar3",
				Reference: "ref2",
			},
		},
		{
			NodeID:  "node3",
			Order:   "start",
			State:   "running",
			CPU:     1,
			Mem:     1,
			Runtime: 42,
			Config: &app.Config{
				ID:        "foobar4",
				Reference: "ref1",
			},
		},
		{
			NodeID:  "node3",
			Order:   "start",
			State:   "running",
			CPU:     1,
			Mem:     1,
			Runtime: 42,
			Config: &app.Config{
				ID:        "foobar5",
				Reference: "ref1",
			},
		},
	}

	affinityMap := createReferenceAffinityMap(processes)

	require.Equal(t, map[string][]referenceAffinityNodeCount{
		"ref1@": {
			{
				nodeid: "node3",
				count:  2,
			},
			{
				nodeid: "node1",
				count:  1,
			},
		},
		"ref2@": {
			{
				nodeid: "node2",
				count:  1,
			},
		},
		"ref3@": {
			{
				nodeid: "node2",
				count:  1,
			},
		},
	}, affinityMap)
}
