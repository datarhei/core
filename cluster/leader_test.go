package cluster

import (
	"testing"
	"time"

	"github.com/datarhei/core/v16/cluster/node"
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
				LimitMemory: 20,
			},
			Order: "start",
		},
		{
			UpdatedAt: time.Now(),
			Config: &app.Config{
				ID:          "foobaz",
				LimitCPU:    10,
				LimitMemory: 20,
			},
			Order: "stop",
		},
	}

	have := []node.Process{}

	nodes := map[string]node.About{
		"node1": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
				NCPU:     1,
				CPU:      7,
				Mem:      35,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
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
				LimitMemory: 20,
			},
			order: "start",
		},
		processOpAdd{
			nodeid: "node1",
			config: &app.Config{
				ID:          "foobaz",
				LimitCPU:    10,
				LimitMemory: 20,
			},
			order: "stop",
		},
	}, stack)

	require.Equal(t, map[string]string{
		"foobar@": "node1",
		"foobaz@": "node1",
	}, reality)

	require.Equal(t, map[string]node.Resources{
		"node1": {
			NCPU:     1,
			CPU:      27,
			Mem:      75,
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

func TestSynchronizeAddDeleted(t *testing.T) {
	wish := map[string]string{
		"foobar@": "node1",
	}

	now := time.Now()

	want := []store.Process{
		{
			UpdatedAt: now,
			Config: &app.Config{
				ID:          "foobar",
				LimitCPU:    10,
				LimitMemory: 20,
			},
			Order: "stop",
		},
	}

	have := []node.Process{}

	nodes := map[string]node.About{
		"node1": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
				NCPU:     1,
				CPU:      1,
				Mem:      1,
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
				LimitMemory: 20,
			},
			order: "stop",
		},
	}, stack)

	require.Equal(t, map[string]string{
		"foobar@": "node1",
	}, reality)

	require.Equal(t, map[string]node.Resources{
		"node1": {
			NCPU:     1,
			CPU:      11,
			Mem:      21,
			CPULimit: 90,
			MemLimit: 90,
		},
	}, resources)
}

func TestSynchronizeOrderStop(t *testing.T) {
	wish := map[string]string{
		"foobar@": "node1",
	}

	now := time.Now()

	want := []store.Process{
		{
			UpdatedAt: now,
			Config: &app.Config{
				ID:          "foobar",
				LimitCPU:    10,
				LimitMemory: 20,
			},
			Order: "stop",
		},
	}

	have := []node.Process{
		{
			NodeID: "node1",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 12,
				Mem: 5,
			},
			Runtime:   42,
			UpdatedAt: now,
			Config: &app.Config{
				ID:          "foobar",
				LimitCPU:    10,
				LimitMemory: 20,
			},
		},
	}

	nodes := map[string]node.About{
		"node1": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
				NCPU:     1,
				CPU:      20,
				Mem:      35,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
				NCPU:     1,
				CPU:      1,
				Mem:      1,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
	}

	stack, resources, reality := synchronize(wish, want, have, nodes, 2*time.Minute)

	require.Equal(t, []interface{}{
		processOpNull{
			processid: app.ProcessID{ID: "foobar"},
		},
		processOpStop{
			nodeid:    "node1",
			processid: app.ProcessID{ID: "foobar"},
		},
	}, stack)

	require.Equal(t, map[string]string{
		"foobar@": "node1",
	}, reality)

	require.Equal(t, map[string]node.Resources{
		"node1": {
			NCPU:     1,
			CPU:      8,
			Mem:      30,
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
	}, resources)
}

func TestSynchronizeOrderStart(t *testing.T) {
	wish := map[string]string{
		"foobar@": "node1",
	}

	now := time.Now()

	want := []store.Process{
		{
			UpdatedAt: now,
			Config: &app.Config{
				ID:          "foobar",
				LimitCPU:    10,
				LimitMemory: 20,
			},
			Order: "start",
		},
	}

	have := []node.Process{
		{
			NodeID: "node1",
			Order:  "stop",
			State:  "finished",
			Resources: node.ProcessResources{
				CPU: 0,
				Mem: 0,
			},
			Runtime:   42,
			UpdatedAt: now,
			Config: &app.Config{
				ID:          "foobar",
				LimitCPU:    10,
				LimitMemory: 20,
			},
		},
	}

	nodes := map[string]node.About{
		"node1": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
				NCPU:     1,
				CPU:      20,
				Mem:      35,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
				NCPU:     1,
				CPU:      1,
				Mem:      1,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
	}

	stack, resources, reality := synchronize(wish, want, have, nodes, 2*time.Minute)

	require.Equal(t, []interface{}{
		processOpNull{
			processid: app.ProcessID{ID: "foobar"},
		},
		processOpStart{
			nodeid:    "node1",
			processid: app.ProcessID{ID: "foobar"},
		},
	}, stack)

	require.Equal(t, map[string]string{
		"foobar@": "node1",
	}, reality)

	require.Equal(t, map[string]node.Resources{
		"node1": {
			NCPU:     1,
			CPU:      30,
			Mem:      55,
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
			Order: "start",
		},
		{
			UpdatedAt: now,
			Config: &app.Config{
				ID:          "foobar2",
				Reference:   "barfoo",
				LimitCPU:    10,
				LimitMemory: 30,
			},
			Order: "start",
		},
	}

	have := []node.Process{
		{
			NodeID: "node2",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 12,
				Mem: 5,
			},
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

	nodes := map[string]node.About{
		"node1": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
				NCPU:     1,
				CPU:      1,
				Mem:      1,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
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
		processOpNull{
			processid: app.ProcessID{ID: "foobar"},
		},
		processOpAdd{
			nodeid: "node2",
			config: &app.Config{
				ID:          "foobar2",
				Reference:   "barfoo",
				LimitCPU:    10,
				LimitMemory: 30,
			},
			order: "start",
		},
	}, stack)

	require.Equal(t, map[string]string{
		"foobar@":  "node2",
		"foobar2@": "node2",
	}, reality)
}

func TestSynchronizeAddReferenceAffinityMultiple(t *testing.T) {
	wish := map[string]string{}

	now := time.Now()

	want := []store.Process{
		{
			UpdatedAt: now,
			Config: &app.Config{
				ID:          "foobar1",
				Reference:   "barfoo",
				LimitCPU:    10,
				LimitMemory: 10,
			},
			Order: "start",
		},
		{
			UpdatedAt: now,
			Config: &app.Config{
				ID:          "foobar2",
				Reference:   "barfoo",
				LimitCPU:    10,
				LimitMemory: 10,
			},
			Order: "start",
		},
		{
			UpdatedAt: now,
			Config: &app.Config{
				ID:          "foobar3",
				Reference:   "barfoo",
				LimitCPU:    10,
				LimitMemory: 10,
			},
			Order: "start",
		},
	}

	have := []node.Process{
		{
			NodeID: "node2",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 1,
				Mem: 2,
			},
			Runtime:   42,
			UpdatedAt: now,
			Config: &app.Config{
				ID:          "foobar1",
				Reference:   "barfoo",
				LimitCPU:    10,
				LimitMemory: 10,
			},
		},
	}

	nodes := map[string]node.About{
		"node1": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
				NCPU:     1,
				CPU:      1,
				Mem:      1,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
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
		processOpNull{
			processid: app.ProcessID{ID: "foobar1"},
		},
		processOpAdd{
			nodeid: "node2",
			config: &app.Config{
				ID:          "foobar2",
				Reference:   "barfoo",
				LimitCPU:    10,
				LimitMemory: 10,
			},
			order: "start",
		},
		processOpAdd{
			nodeid: "node2",
			config: &app.Config{
				ID:          "foobar3",
				Reference:   "barfoo",
				LimitCPU:    10,
				LimitMemory: 10,
			},
			order: "start",
		},
	}, stack)

	require.Equal(t, map[string]string{
		"foobar1@": "node2",
		"foobar2@": "node2",
		"foobar3@": "node2",
	}, reality)
}

func TestSynchronizeAddReferenceAffinityMultipleEmptyNodes(t *testing.T) {
	wish := map[string]string{}

	now := time.Now()

	want := []store.Process{
		{
			UpdatedAt: now,
			Config: &app.Config{
				ID:          "foobar1",
				Reference:   "barfoo",
				LimitCPU:    10,
				LimitMemory: 10,
			},
			Order: "start",
		},
		{
			UpdatedAt: now,
			Config: &app.Config{
				ID:          "foobar2",
				Reference:   "barfoo",
				LimitCPU:    10,
				LimitMemory: 10,
			},
			Order: "start",
		},
		{
			UpdatedAt: now,
			Config: &app.Config{
				ID:          "foobar3",
				Reference:   "barfoo",
				LimitCPU:    10,
				LimitMemory: 10,
			},
			Order: "start",
		},
	}

	have := []node.Process{}

	nodes := map[string]node.About{
		"node1": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
				NCPU:     1,
				CPU:      1,
				Mem:      1,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
				NCPU:     1,
				CPU:      1,
				Mem:      1,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
	}

	stack, _, reality := synchronize(wish, want, have, nodes, 2*time.Minute)

	require.Equal(t, 3, len(stack))

	nodeid := reality["foobar1@"]

	require.Equal(t, map[string]string{
		"foobar1@": nodeid,
		"foobar2@": nodeid,
		"foobar3@": nodeid,
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
			Order: "start",
		},
	}

	have := []node.Process{}

	nodes := map[string]node.About{
		"node1": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
				NCPU:     1,
				CPU:      81,
				Mem:      72,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
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
			order: "start",
		},
	}, stack)

	require.Equal(t, map[string]string{
		"foobar@": "node2",
	}, reality)

	require.Equal(t, map[string]node.Resources{
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
			Order: "start",
		},
	}

	have := []node.Process{}

	nodes := map[string]node.About{
		"node1": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
				NCPU:     1,
				CPU:      81,
				Mem:      72,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
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
			err:       errNotEnoughResourcesForDeployment,
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
			Order: "start",
		},
	}

	have := []node.Process{}

	nodes := map[string]node.About{
		"node1": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
				NCPU:     1,
				CPU:      81,
				Mem:      72,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
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
			err:       errNotEnoughResourcesForDeployment,
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
			Order: "start",
		},
	}

	have := []node.Process{}

	nodes := map[string]node.About{
		"node1": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
				NCPU:     1,
				CPU:      81,
				Mem:      72,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
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

	have := []node.Process{
		{
			NodeID: "node2",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 12,
				Mem: 5,
			},
			Runtime: 42,
			Config: &app.Config{
				ID: "foobar",
			},
		},
	}

	nodes := map[string]node.About{
		"node1": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
				NCPU:     1,
				CPU:      7,
				Mem:      65,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
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

	require.Equal(t, map[string]node.Resources{
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
			Order: "start",
		},
	}

	have := []node.Process{
		{
			NodeID: "node2",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 12,
				Mem: 5,
			},
			Runtime: 42,
			Config: &app.Config{
				ID: "foobar2",
			},
		},
	}

	nodes := map[string]node.About{
		"node1": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
				NCPU:     1,
				CPU:      7,
				Mem:      35,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
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
			order: "start",
		},
	}, stack)

	require.Equal(t, map[string]node.Resources{
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

func TestSynchronizeNoUpdate(t *testing.T) {
	wish := map[string]string{
		"foobar@": "node1",
	}

	want := []store.Process{
		{
			UpdatedAt: time.Now(),
			Config: &app.Config{
				ID:          "foobar",
				LimitCPU:    10,
				LimitMemory: 5,
				Reference:   "baz",
			},
			Order: "start",
		},
	}

	have := []node.Process{
		{
			NodeID: "node1",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 12,
				Mem: 5,
			},
			Runtime: 42,
			Config: &app.Config{
				ID:          "foobar",
				LimitCPU:    10,
				LimitMemory: 5,
				Reference:   "baz",
			},
		},
	}

	nodes := map[string]node.About{
		"node1": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
				NCPU:     1,
				CPU:      7,
				Mem:      35,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
				NCPU:     1,
				CPU:      85,
				Mem:      65,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
	}

	stack, _, reality := synchronize(wish, want, have, nodes, 2*time.Minute)

	require.Equal(t, []interface{}{
		processOpNull{
			processid: app.ProcessID{ID: "foobar"},
		},
	}, stack)

	require.Equal(t, map[string]string{
		"foobar@": "node1",
	}, reality)
}

func TestSynchronizeUpdate(t *testing.T) {
	wish := map[string]string{
		"foobar@": "node1",
	}

	want := []store.Process{
		{
			UpdatedAt: time.Now(),
			Config: &app.Config{
				ID:          "foobar",
				LimitCPU:    10,
				LimitMemory: 5,
				Reference:   "baz",
			},
			Order: "start",
		},
	}

	have := []node.Process{
		{
			NodeID: "node1",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 12,
				Mem: 5,
			},
			Runtime: 42,
			Config: &app.Config{
				ID:          "foobar",
				LimitCPU:    10,
				LimitMemory: 5,
				Reference:   "boz",
			},
		},
	}

	nodes := map[string]node.About{
		"node1": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
				NCPU:     1,
				CPU:      7,
				Mem:      35,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
				NCPU:     1,
				CPU:      85,
				Mem:      65,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
	}

	stack, _, reality := synchronize(wish, want, have, nodes, 2*time.Minute)

	require.Equal(t, []interface{}{
		processOpUpdate{
			nodeid:    "node1",
			processid: app.ProcessID{ID: "foobar"},
			config: &app.Config{
				ID:          "foobar",
				LimitCPU:    10,
				LimitMemory: 5,
				Reference:   "baz",
			},
			metadata: map[string]interface{}{},
		},
	}, stack)

	require.Equal(t, map[string]string{
		"foobar@": "node1",
	}, reality)
}

func TestSynchronizeUpdateMetadata(t *testing.T) {
	wish := map[string]string{
		"foobar@": "node1",
	}

	want := []store.Process{
		{
			UpdatedAt: time.Now(),
			Config: &app.Config{
				ID:          "foobar",
				LimitCPU:    10,
				LimitMemory: 5,
				Reference:   "boz",
			},
			Order: "start",
			Metadata: map[string]interface{}{
				"foo": "bar",
			},
		},
	}

	have := []node.Process{
		{
			NodeID: "node1",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 12,
				Mem: 5,
			},
			Runtime: 42,
			Config: &app.Config{
				ID:          "foobar",
				LimitCPU:    10,
				LimitMemory: 5,
				Reference:   "boz",
			},
		},
	}

	nodes := map[string]node.About{
		"node1": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
				NCPU:     1,
				CPU:      7,
				Mem:      35,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
				NCPU:     1,
				CPU:      85,
				Mem:      65,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
	}

	stack, _, reality := synchronize(wish, want, have, nodes, 2*time.Minute)

	require.Equal(t, []interface{}{
		processOpUpdate{
			nodeid:    "node1",
			processid: app.ProcessID{ID: "foobar"},
			config: &app.Config{
				ID:          "foobar",
				LimitCPU:    10,
				LimitMemory: 5,
				Reference:   "boz",
			},
			metadata: map[string]interface{}{
				"foo": "bar",
			},
		},
	}, stack)

	require.Equal(t, map[string]string{
		"foobar@": "node1",
	}, reality)
}

func TestSynchronizeWaitDisconnectedNode(t *testing.T) {
	wish := map[string]string{
		"foobar1@": "node1",
		"foobar2@": "node2",
	}

	now := time.Now()

	want := []store.Process{
		{
			UpdatedAt: now,
			Config: &app.Config{
				ID:          "foobar1",
				Reference:   "barfoo",
				LimitCPU:    10,
				LimitMemory: 20,
			},
			Order: "start",
		},
		{
			UpdatedAt: now,
			Config: &app.Config{
				ID:          "foobar2",
				Reference:   "barfoo",
				LimitCPU:    10,
				LimitMemory: 30,
			},
			Order: "start",
		},
	}

	have := []node.Process{
		{
			NodeID: "node1",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 12,
				Mem: 5,
			},
			Runtime:   42,
			UpdatedAt: now,
			Config: &app.Config{
				ID:          "foobar1",
				Reference:   "barfoo",
				LimitCPU:    10,
				LimitMemory: 20,
			},
		},
	}

	nodes := map[string]node.About{
		"node1": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
				NCPU:     1,
				CPU:      1,
				Mem:      1,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			State:       "offline",
			LastContact: time.Now().Add(-time.Minute),
			Resources: node.Resources{
				IsThrottling: true,
				NCPU:         1,
				CPU:          1,
				Mem:          1,
				CPULimit:     90,
				MemLimit:     90,
			},
		},
	}

	stack, _, reality := synchronize(wish, want, have, nodes, 2*time.Minute)

	require.Equal(t, []interface{}{
		processOpNull{
			processid: app.ProcessID{ID: "foobar1"},
		},
	}, stack)

	require.Equal(t, map[string]string{
		"foobar1@": "node1",
		"foobar2@": "node2",
	}, reality)
}

func TestSynchronizeWaitDisconnectedNodeNoWish(t *testing.T) {
	wish := map[string]string{
		"foobar1@": "node1",
	}

	now := time.Now()

	want := []store.Process{
		{
			UpdatedAt: now,
			Config: &app.Config{
				ID:          "foobar1",
				Reference:   "barfoo",
				LimitCPU:    10,
				LimitMemory: 20,
			},
			Order: "start",
		},
		{
			UpdatedAt: now,
			Config: &app.Config{
				ID:          "foobar2",
				Reference:   "barfoo",
				LimitCPU:    10,
				LimitMemory: 30,
			},
			Order: "start",
		},
	}

	have := []node.Process{
		{
			NodeID: "node1",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 12,
				Mem: 5,
			},
			Runtime:   42,
			UpdatedAt: now,
			Config: &app.Config{
				ID:          "foobar1",
				Reference:   "barfoo",
				LimitCPU:    10,
				LimitMemory: 20,
			},
		},
	}

	nodes := map[string]node.About{
		"node1": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
				NCPU:     1,
				CPU:      1,
				Mem:      1,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			State:       "offline",
			LastContact: time.Now().Add(-time.Minute),
			Resources: node.Resources{
				IsThrottling: true,
				NCPU:         1,
				CPU:          1,
				Mem:          1,
				CPULimit:     90,
				MemLimit:     90,
			},
		},
	}

	stack, _, reality := synchronize(wish, want, have, nodes, 2*time.Minute)

	require.Equal(t, []interface{}{
		processOpNull{
			processid: app.ProcessID{ID: "foobar1"},
		},
		processOpAdd{
			nodeid: "node1",
			config: &app.Config{
				ID:          "foobar2",
				Reference:   "barfoo",
				LimitCPU:    10,
				LimitMemory: 30,
			},
			order: "start",
		},
	}, stack)

	require.Equal(t, map[string]string{
		"foobar1@": "node1",
		"foobar2@": "node1",
	}, reality)
}

func TestSynchronizeWaitDisconnectedNodeUnrealisticWish(t *testing.T) {
	wish := map[string]string{
		"foobar1@": "node1",
		"foobar2@": "node3",
	}

	now := time.Now()

	want := []store.Process{
		{
			UpdatedAt: now,
			Config: &app.Config{
				ID:          "foobar1",
				Reference:   "barfoo",
				LimitCPU:    10,
				LimitMemory: 20,
			},
			Order: "start",
		},
		{
			UpdatedAt: now,
			Config: &app.Config{
				ID:          "foobar2",
				Reference:   "barfoo",
				LimitCPU:    10,
				LimitMemory: 30,
			},
			Order: "start",
		},
	}

	have := []node.Process{
		{
			NodeID: "node1",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 12,
				Mem: 5,
			},
			Runtime:   42,
			UpdatedAt: now,
			Config: &app.Config{
				ID:          "foobar1",
				Reference:   "barfoo",
				LimitCPU:    10,
				LimitMemory: 20,
			},
		},
	}

	nodes := map[string]node.About{
		"node1": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
				NCPU:     1,
				CPU:      1,
				Mem:      1,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			State:       "offline",
			LastContact: time.Now().Add(-time.Minute),
			Resources: node.Resources{
				IsThrottling: true,
				NCPU:         1,
				CPU:          1,
				Mem:          1,
				CPULimit:     90,
				MemLimit:     90,
			},
		},
	}

	stack, _, reality := synchronize(wish, want, have, nodes, 2*time.Minute)

	require.Equal(t, []interface{}{
		processOpNull{
			processid: app.ProcessID{ID: "foobar1"},
		},
		processOpAdd{
			nodeid: "node1",
			config: &app.Config{
				ID:          "foobar2",
				Reference:   "barfoo",
				LimitCPU:    10,
				LimitMemory: 30,
			},
			order: "start",
		},
	}, stack)

	require.Equal(t, map[string]string{
		"foobar1@": "node1",
		"foobar2@": "node1",
	}, reality)
}

func TestSynchronizeTimeoutDisconnectedNode(t *testing.T) {
	wish := map[string]string{
		"foobar1@": "node1",
		"foobar2@": "node2",
	}

	now := time.Now()

	want := []store.Process{
		{
			UpdatedAt: now,
			Config: &app.Config{
				ID:          "foobar1",
				Reference:   "barfoo",
				LimitCPU:    10,
				LimitMemory: 20,
			},
			Order: "start",
		},
		{
			UpdatedAt: now,
			Config: &app.Config{
				ID:          "foobar2",
				Reference:   "barfoo",
				LimitCPU:    10,
				LimitMemory: 30,
			},
			Order: "start",
		},
	}

	have := []node.Process{
		{
			NodeID: "node1",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 12,
				Mem: 5,
			},
			Runtime:   42,
			UpdatedAt: now,
			Config: &app.Config{
				ID:          "foobar1",
				Reference:   "barfoo",
				LimitCPU:    10,
				LimitMemory: 20,
			},
		},
	}

	nodes := map[string]node.About{
		"node1": {
			LastContact: time.Now(),
			State:       "online",
			Resources: node.Resources{
				NCPU:     1,
				CPU:      1,
				Mem:      1,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			LastContact: time.Now().Add(-3 * time.Minute),
			State:       "online",
			Resources: node.Resources{
				IsThrottling: true,
				NCPU:         1,
				CPU:          1,
				Mem:          1,
				CPULimit:     90,
				MemLimit:     90,
			},
		},
	}

	stack, _, reality := synchronize(wish, want, have, nodes, 2*time.Minute)

	require.Equal(t, []interface{}{
		processOpNull{
			processid: app.ProcessID{ID: "foobar1"},
		},
		processOpAdd{
			nodeid: "node1",
			config: &app.Config{
				ID:          "foobar2",
				Reference:   "barfoo",
				LimitCPU:    10,
				LimitMemory: 30,
			},
			order: "start",
		},
	}, stack)

	require.Equal(t, map[string]string{
		"foobar1@": "node1",
		"foobar2@": "node1",
	}, reality)
}

func TestRebalanceNothingToDo(t *testing.T) {
	processes := []node.Process{
		{
			NodeID: "node1",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 35,
				Mem: 20,
			},
			Runtime: 42,
			Config: &app.Config{
				ID: "foobar1",
			},
		},
		{
			NodeID: "node2",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 12,
				Mem: 5,
			},
			Runtime: 42,
			Config: &app.Config{
				ID: "foobar2",
			},
		},
	}

	nodes := map[string]node.About{
		"node1": {
			LastContact: time.Now(),
			State:       "online",
			Resources: node.Resources{
				NCPU:     1,
				CPU:      42,
				Mem:      35,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			LastContact: time.Now(),
			State:       "online",
			Resources: node.Resources{
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
	processes := []node.Process{
		{
			NodeID: "node1",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 35,
				Mem: 20,
			},
			Runtime: 42,
			Config: &app.Config{
				ID: "foobar1",
			},
		},
		{
			NodeID: "node1",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 17,
				Mem: 31,
			},
			Runtime: 27,
			Config: &app.Config{
				ID: "foobar3",
			},
		},
		{
			NodeID: "node2",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 12,
				Mem: 5,
			},
			Runtime: 42,
			Config: &app.Config{
				ID: "foobar2",
			},
		},
	}

	nodes := map[string]node.About{
		"node1": {
			LastContact: time.Now(),
			State:       "online",
			Resources: node.Resources{
				NCPU:     1,
				CPU:      91,
				Mem:      35,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			LastContact: time.Now(),
			State:       "online",
			Resources: node.Resources{
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
			order: "start",
		},
	}, opStack)

	require.Equal(t, map[string]node.Resources{
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
	processes := []node.Process{
		{
			NodeID: "node1",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 35,
				Mem: 20,
			},
			Runtime: 42,
			Config: &app.Config{
				ID: "foobar1",
			},
		},
		{
			NodeID: "node1",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 17,
				Mem: 31,
			},
			Runtime: 27,
			Config: &app.Config{
				ID: "foobar3",
			},
		},
		{
			NodeID: "node2",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 12,
				Mem: 5,
			},
			Runtime: 42,
			Config: &app.Config{
				ID: "foobar2",
			},
		},
	}

	nodes := map[string]node.About{
		"node1": {
			LastContact: time.Now(),
			State:       "online",
			Resources: node.Resources{
				NCPU:     1,
				CPU:      91,
				Mem:      35,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			LastContact: time.Now(),
			State:       "online",
			Resources: node.Resources{
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

	require.Equal(t, map[string]node.Resources{
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
	processes := []node.Process{
		{
			NodeID: "node1",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 1,
				Mem: 1,
			},
			Runtime: 42,
			Config: &app.Config{
				ID: "foobar1",
			},
		},
		{
			NodeID: "node1",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 1,
				Mem: 1,
			},
			Runtime: 1,
			Config: &app.Config{
				ID:        "foobar2",
				Reference: "barfoo",
			},
		},
		{
			NodeID: "node2",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 1,
				Mem: 1,
			},
			Runtime: 42,
			Config: &app.Config{
				ID:        "foobar3",
				Reference: "barfoo",
			},
		},
		{
			NodeID: "node3",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 1,
				Mem: 1,
			},
			Runtime: 42,
			Config: &app.Config{
				ID:        "foobar4",
				Reference: "barfoo",
			},
		},
		{
			NodeID: "node3",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 1,
				Mem: 1,
			},
			Runtime: 42,
			Config: &app.Config{
				ID:        "foobar5",
				Reference: "barfoo",
			},
		},
	}

	nodes := map[string]node.About{
		"node1": {
			LastContact: time.Now(),
			State:       "online",
			Resources: node.Resources{
				NCPU:     1,
				CPU:      90,
				Mem:      90,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			LastContact: time.Now(),
			State:       "online",
			Resources: node.Resources{
				NCPU:     1,
				CPU:      1,
				Mem:      1,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node3": {
			LastContact: time.Now(),
			State:       "online",
			Resources: node.Resources{
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
			order: "start",
		},
	}, opStack)

	require.Equal(t, map[string]node.Resources{
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

func TestRebalanceRelocateTarget(t *testing.T) {
	processes := []node.Process{
		{
			NodeID: "node1",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 35,
				Mem: 20,
			},
			Runtime: 42,
			Config: &app.Config{
				ID: "foobar1",
			},
		},
		{
			NodeID: "node1",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 17,
				Mem: 31,
			},
			Runtime: 27,
			Config: &app.Config{
				ID: "foobar3",
			},
		},
		{
			NodeID: "node2",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 12,
				Mem: 5,
			},
			Runtime: 42,
			Config: &app.Config{
				ID: "foobar2",
			},
		},
	}

	nodes := map[string]node.About{
		"node1": {
			LastContact: time.Now(),
			State:       "online",
			Resources: node.Resources{
				NCPU:     1,
				CPU:      27,
				Mem:      35,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			LastContact: time.Now(),
			State:       "online",
			Resources: node.Resources{
				NCPU:     1,
				CPU:      15,
				Mem:      11,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node3": {
			LastContact: time.Now(),
			State:       "online",
			Resources: node.Resources{
				NCPU:     1,
				CPU:      0,
				Mem:      0,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
	}

	relocateMap := map[string]string{
		"foobar1@": "node3",
	}

	opStack, resources, _ := relocate(processes, nodes, relocateMap)

	require.NotEmpty(t, opStack)

	require.Equal(t, []interface{}{
		processOpMove{
			fromNodeid: "node1",
			toNodeid:   "node3",
			config: &app.Config{
				ID: "foobar1",
			},
			order: "start",
		},
	}, opStack)

	require.Equal(t, map[string]node.Resources{
		"node1": {
			NCPU:     1,
			CPU:      0,
			Mem:      15,
			CPULimit: 90,
			MemLimit: 90,
		},
		"node2": {
			NCPU:     1,
			CPU:      15,
			Mem:      11,
			CPULimit: 90,
			MemLimit: 90,
		},
		"node3": {
			NCPU:     1,
			CPU:      35,
			Mem:      20,
			CPULimit: 90,
			MemLimit: 90,
		},
	}, resources)
}

func TestRebalanceRelocateAny(t *testing.T) {
	processes := []node.Process{
		{
			NodeID: "node1",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 35,
				Mem: 20,
			},
			Runtime: 42,
			Config: &app.Config{
				ID: "foobar1",
			},
		},
		{
			NodeID: "node1",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 17,
				Mem: 31,
			},
			Runtime: 27,
			Config: &app.Config{
				ID: "foobar3",
			},
		},
		{
			NodeID: "node2",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 12,
				Mem: 5,
			},
			Runtime: 42,
			Config: &app.Config{
				ID: "foobar2",
			},
		},
	}

	nodes := map[string]node.About{
		"node1": {
			LastContact: time.Now(),
			State:       "online",
			Resources: node.Resources{
				NCPU:     1,
				CPU:      27,
				Mem:      35,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			LastContact: time.Now(),
			State:       "online",
			Resources: node.Resources{
				NCPU:     1,
				CPU:      15,
				Mem:      11,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node3": {
			LastContact: time.Now(),
			State:       "online",
			Resources: node.Resources{
				NCPU:     1,
				CPU:      0,
				Mem:      0,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
	}

	relocateMap := map[string]string{
		"foobar1@": "",
	}

	opStack, resources, _ := relocate(processes, nodes, relocateMap)

	require.NotEmpty(t, opStack)

	require.Equal(t, []interface{}{
		processOpMove{
			fromNodeid: "node1",
			toNodeid:   "node3",
			config: &app.Config{
				ID: "foobar1",
			},

			order: "start",
		},
	}, opStack)

	require.Equal(t, map[string]node.Resources{
		"node1": {
			NCPU:     1,
			CPU:      0,
			Mem:      15,
			CPULimit: 90,
			MemLimit: 90,
		},
		"node2": {
			NCPU:     1,
			CPU:      15,
			Mem:      11,
			CPULimit: 90,
			MemLimit: 90,
		},
		"node3": {
			NCPU:     1,
			CPU:      35,
			Mem:      20,
			CPULimit: 90,
			MemLimit: 90,
		},
	}, resources)
}

func TestFindBestNodesForProcess(t *testing.T) {
	nodes := map[string]node.About{
		"node1": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
				NCPU:     1,
				CPU:      27,
				Mem:      35,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
				NCPU:     1,
				CPU:      15,
				Mem:      11,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node3": {
			State:       "online",
			LastContact: time.Now(),
			Resources: node.Resources{
				NCPU:     1,
				CPU:      0,
				Mem:      0,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
	}

	resources := NewResourcePlanner(nodes)

	list := resources.FindBestNodes(Resources{
		CPU: 35,
		Mem: 20,
	})

	require.Equal(t, []string{"node3", "node2", "node1"}, list)
}

func TestFindBestNodesForProcess2(t *testing.T) {
	resources := NewResourcePlanner(nil)
	resources.nodes = map[string]node.Resources{
		"node1": {
			CPULimit:     104.50000000000001,
			CPU:          29.725299999999997,
			IsThrottling: false,
			MemLimit:     1051931443,
			Mem:          212262912,
			NCPU:         1.1,
		},
		"node2": {
			CPULimit:     104.50000000000001,
			CPU:          53.576600000000006,
			IsThrottling: false,
			MemLimit:     1051931443,
			Mem:          805830656,
			NCPU:         1.1,
		},
		"node3": {
			CPULimit:     104.50000000000001,
			CPU:          33.99000000000001,
			IsThrottling: false,
			MemLimit:     1051931443,
			Mem:          190910464,
			NCPU:         1.1,
		},
		"node4": {
			CPULimit:     104.50000000000001,
			CPU:          31.291700000000006,
			IsThrottling: false,
			MemLimit:     1051931443,
			Mem:          129310720,
			NCPU:         1.1,
		},
		"node5": {
			CPULimit:     104.50000000000001,
			CPU:          30.634999999999994,
			IsThrottling: false,
			MemLimit:     1051931443,
			Mem:          159158272,
			NCPU:         1.1,
		},
		"node6": {
			CPULimit:     104.50000000000001,
			CPU:          40.368900000000004,
			IsThrottling: false,
			MemLimit:     1051931443,
			Mem:          212189184,
			NCPU:         1.1,
		},
		"node7": {
			CPULimit:     104.50000000000001,
			CPU:          25.469399999999997,
			IsThrottling: false,
			MemLimit:     1051931443,
			Mem:          206098432,
			NCPU:         1.1,
		},
		"node8": {
			CPULimit:     104.50000000000001,
			CPU:          22.180400000000002,
			IsThrottling: false,
			MemLimit:     1051931443,
			Mem:          144138240,
			NCPU:         1.1,
		},
		"node9": {
			CPULimit:     104.50000000000001,
			CPU:          62.6714,
			IsThrottling: true,
			MemLimit:     1051931443,
			Mem:          978501632,
			NCPU:         1.1,
		},
		"node10": {
			CPULimit:     104.50000000000001,
			CPU:          18.7748,
			IsThrottling: false,
			MemLimit:     1051931443,
			Mem:          142430208,
			NCPU:         1.1,
		},
		"node11": {
			CPULimit:     104.50000000000001,
			CPU:          43.807500000000005,
			IsThrottling: false,
			MemLimit:     1051931443,
			Mem:          368091136,
			NCPU:         1.1,
		},
		"node12": {
			CPULimit:     104.50000000000001,
			CPU:          31.067299999999996,
			IsThrottling: false,
			MemLimit:     1051931443,
			Mem:          149897216,
			NCPU:         1.1,
		},
		"node13": {
			CPULimit:     104.50000000000001,
			CPU:          35.93480000000001,
			IsThrottling: false,
			MemLimit:     1051931443,
			Mem:          194408448,
			NCPU:         1.1,
		},
	}

	list := resources.FindBestNodes(Resources{
		CPU: 4.0,
		Mem: 45 * 1024 * 1024,
	})

	require.Equal(t, []string{"node10", "node8", "node7", "node1", "node5", "node12", "node4", "node3", "node13", "node6", "node11", "node2"}, list)
}

func TestCreateNodeProcessMap(t *testing.T) {
	processes := []node.Process{
		{
			NodeID: "node1",
			Order:  "start",
			State:  "finished",
			Resources: node.ProcessResources{
				CPU: 1,
				Mem: 1,
			},
			Runtime: 1,
			Config: &app.Config{
				ID:        "foobar7",
				Reference: "ref1",
			},
		},
		{
			NodeID: "node1",
			Order:  "start",
			State:  "failed",
			Resources: node.ProcessResources{
				CPU: 1,
				Mem: 1,
			},
			Runtime: 1,
			Config: &app.Config{
				ID:        "foobar8",
				Reference: "ref1",
			},
		},
		{
			NodeID: "node1",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 1,
				Mem: 1,
			},
			Runtime: 42,
			Config: &app.Config{
				ID: "foobar1",
			},
		},
		{
			NodeID: "node1",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 1,
				Mem: 1,
			},
			Runtime: 1,
			Config: &app.Config{
				ID:        "foobar2",
				Reference: "ref1",
			},
		},
		{
			NodeID: "node2",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 1,
				Mem: 1,
			},
			Runtime: 67,
			Config: &app.Config{
				ID:        "foobar3",
				Reference: "ref3",
			},
		},
		{
			NodeID: "node2",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 1,
				Mem: 1,
			},
			Runtime: 42,
			Config: &app.Config{
				ID:        "foobar6",
				Reference: "ref2",
			},
		},
		{
			NodeID: "node3",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 1,
				Mem: 1,
			},
			Runtime: 41,
			Config: &app.Config{
				ID:        "foobar4",
				Reference: "ref1",
			},
		},
		{
			NodeID: "node3",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 1,
				Mem: 1,
			},
			Runtime: 42,
			Config: &app.Config{
				ID:        "foobar5",
				Reference: "ref1",
			},
		},
	}

	nodeProcessMap := createNodeProcessMap(processes)

	require.Equal(t, map[string][]node.Process{
		"node1": {
			{
				NodeID: "node1",
				Order:  "start",
				State:  "running",
				Resources: node.ProcessResources{
					CPU: 1,
					Mem: 1,
				},
				Runtime: 1,
				Config: &app.Config{
					ID:        "foobar2",
					Reference: "ref1",
				},
			},
			{
				NodeID: "node1",
				Order:  "start",
				State:  "running",
				Resources: node.ProcessResources{
					CPU: 1,
					Mem: 1,
				},
				Runtime: 42,
				Config: &app.Config{
					ID: "foobar1",
				},
			},
		},
		"node2": {
			{
				NodeID: "node2",
				Order:  "start",
				State:  "running",
				Resources: node.ProcessResources{
					CPU: 1,
					Mem: 1,
				},
				Runtime: 42,
				Config: &app.Config{
					ID:        "foobar6",
					Reference: "ref2",
				},
			},
			{
				NodeID: "node2",
				Order:  "start",
				State:  "running",
				Resources: node.ProcessResources{
					CPU: 1,
					Mem: 1,
				},
				Runtime: 67,
				Config: &app.Config{
					ID:        "foobar3",
					Reference: "ref3",
				},
			},
		},
		"node3": {
			{
				NodeID: "node3",
				Order:  "start",
				State:  "running",
				Resources: node.ProcessResources{
					CPU: 1,
					Mem: 1,
				},
				Runtime: 41,
				Config: &app.Config{
					ID:        "foobar4",
					Reference: "ref1",
				},
			},
			{
				NodeID: "node3",
				Order:  "start",
				State:  "running",
				Resources: node.ProcessResources{
					CPU: 1,
					Mem: 1,
				},
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
	processes := []node.Process{
		{
			NodeID: "node1",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 1,
				Mem: 1,
			},
			Runtime: 42,
			Config: &app.Config{
				ID: "foobar1",
			},
		},
		{
			NodeID: "node1",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 1,
				Mem: 1,
			},
			Runtime: 1,
			Config: &app.Config{
				ID:        "foobar2",
				Reference: "ref1",
			},
		},
		{
			NodeID: "node2",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 1,
				Mem: 1,
			},
			Runtime: 42,
			Config: &app.Config{
				ID:        "foobar3",
				Reference: "ref3",
			},
		},
		{
			NodeID: "node2",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 1,
				Mem: 1,
			},
			Runtime: 42,
			Config: &app.Config{
				ID:        "foobar3",
				Reference: "ref2",
			},
		},
		{
			NodeID: "node3",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 1,
				Mem: 1,
			},
			Runtime: 42,
			Config: &app.Config{
				ID:        "foobar4",
				Reference: "ref1",
			},
		},
		{
			NodeID: "node3",
			Order:  "start",
			State:  "running",
			Resources: node.ProcessResources{
				CPU: 1,
				Mem: 1,
			},
			Runtime: 42,
			Config: &app.Config{
				ID:        "foobar5",
				Reference: "ref1",
			},
		},
	}

	affinityMap := NewReferenceAffinity(processes)

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
	}, affinityMap.m)
}

func TestUpdateReferenceAffinityNodeMap(t *testing.T) {
	affinityMap := &referenceAffinity{
		m: map[string][]referenceAffinityNodeCount{
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
		},
	}

	affinityMap.Add("ref3", "", "node1")

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
			{
				nodeid: "node1",
				count:  1,
			},
		},
	}, affinityMap.m)

	affinityMap.Add("ref2", "", "node2")

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
				count:  2,
			},
		},
		"ref3@": {
			{
				nodeid: "node2",
				count:  1,
			},
			{
				nodeid: "node1",
				count:  1,
			},
		},
	}, affinityMap.m)

	affinityMap.Add("ref4", "", "node2")

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
				count:  2,
			},
		},
		"ref3@": {
			{
				nodeid: "node2",
				count:  1,
			},
			{
				nodeid: "node1",
				count:  1,
			},
		},
		"ref4@": {
			{
				nodeid: "node2",
				count:  1,
			},
		},
	}, affinityMap.m)

	affinityMap.Move("ref2", "", "node2", "node3")

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
			{
				nodeid: "node3",
				count:  1,
			},
		},
		"ref3@": {
			{
				nodeid: "node2",
				count:  1,
			},
			{
				nodeid: "node1",
				count:  1,
			},
		},
		"ref4@": {
			{
				nodeid: "node2",
				count:  1,
			},
		},
	}, affinityMap.m)

	affinityMap.Move("ref2", "", "node2", "node3")

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
				nodeid: "node3",
				count:  2,
			},
		},
		"ref3@": {
			{
				nodeid: "node2",
				count:  1,
			},
			{
				nodeid: "node1",
				count:  1,
			},
		},
		"ref4@": {
			{
				nodeid: "node2",
				count:  1,
			},
		},
	}, affinityMap.m)
}

func TestIsMetadataUpdateRequired(t *testing.T) {
	want1 := map[string]interface{}{
		"foo": "boz",
		"sum": "sum",
		"sim": []string{"id", "sam"},
	}

	have := map[string]interface{}{
		"sim": []string{"id", "sam"},
		"foo": "boz",
		"sum": "sum",
	}

	changes, _ := isMetadataUpdateRequired(want1, have)
	require.False(t, changes)

	want2 := map[string]interface{}{
		"sim": []string{"id", "sam"},
		"foo": "boz",
	}

	changes, metadata := isMetadataUpdateRequired(want2, have)
	require.True(t, changes)
	require.Equal(t, map[string]interface{}{
		"sim": []string{"id", "sam"},
		"foo": "boz",
		"sum": nil,
	}, metadata)

	want3 := map[string]interface{}{
		"sim": []string{"id", "sim"},
		"foo": "boz",
		"sum": "sum",
	}

	changes, metadata = isMetadataUpdateRequired(want3, have)
	require.True(t, changes)
	require.Equal(t, map[string]interface{}{
		"sim": []string{"id", "sim"},
		"foo": "boz",
		"sum": "sum",
	}, metadata)

	want4 := map[string]interface{}{}

	changes, metadata = isMetadataUpdateRequired(want4, have)
	require.True(t, changes)
	require.Equal(t, map[string]interface{}{
		"sim": nil,
		"foo": nil,
		"sum": nil,
	}, metadata)
}
