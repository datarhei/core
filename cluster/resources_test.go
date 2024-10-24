package cluster

import (
	"testing"

	"github.com/datarhei/core/v16/cluster/node"
	"github.com/stretchr/testify/require"
)

func TestResources(t *testing.T) {
	r := Resources{
		CPU: 1,
		Mem: 1,
	}

	require.False(t, r.HasGPU())

	r.GPU = ResourcesGPU{
		Index:   0,
		Usage:   1,
		Encoder: 0,
		Decoder: 0,
		Mem:     1,
	}

	require.True(t, r.HasGPU())
}

func TestResourcePlanner(t *testing.T) {
	nodes := map[string]node.About{
		"node1": {
			State: "online",
			Resources: node.Resources{
				NCPU:     1,
				CPU:      7,
				Mem:      35,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			State: "online",
			Resources: node.Resources{
				NCPU:     1,
				CPU:      85,
				Mem:      11,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
	}

	planner := NewResourcePlanner(nodes)

	require.Equal(t, map[string]node.Resources{
		"node1": {
			NCPU:     1,
			CPU:      7,
			Mem:      35,
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
	}, planner.Map())
}

func TestResourcePlannerBlocked(t *testing.T) {
	nodes := map[string]node.About{
		"node1": {
			State: "degraded",
			Resources: node.Resources{
				NCPU:     1,
				CPU:      7,
				Mem:      35,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			State: "online",
			Resources: node.Resources{
				NCPU:     1,
				CPU:      85,
				Mem:      11,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
	}

	planner := NewResourcePlanner(nodes)

	require.Equal(t, []string{"node1"}, planner.Blocked())
}

func TestResourcePlannerThrottling(t *testing.T) {
	nodes := map[string]node.About{
		"node1": {
			State: "online",
			Resources: node.Resources{
				NCPU:     1,
				CPU:      7,
				Mem:      35,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
		"node2": {
			State: "online",
			Resources: node.Resources{
				NCPU:     1,
				CPU:      85,
				Mem:      11,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
	}

	planner := NewResourcePlanner(nodes)

	require.True(t, planner.HasNodeEnough("node1", Resources{
		CPU: 30,
		Mem: 5,
	}))

	planner.Throttling("node1", true)

	require.False(t, planner.HasNodeEnough("node1", Resources{
		CPU: 30,
		Mem: 5,
	}))

	planner.Throttling("node1", false)

	require.True(t, planner.HasNodeEnough("node1", Resources{
		CPU: 30,
		Mem: 5,
	}))
}

func TestRecourcePlannerHasNodeEnough(t *testing.T) {
	nodes := map[string]node.About{
		"node1": {
			State: "online",
			Resources: node.Resources{
				NCPU:     1,
				CPU:      7,
				Mem:      35,
				CPULimit: 90,
				MemLimit: 90,
				GPU: []node.ResourcesGPU{
					{
						Mem:        5,
						MemLimit:   90,
						Usage:      53,
						UsageLimit: 90,
						Encoder:    32,
						Decoder:    26,
					},
					{
						Mem:        85,
						MemLimit:   90,
						Usage:      64,
						UsageLimit: 90,
						Encoder:    43,
						Decoder:    12,
					},
				},
			},
		},
		"node2": {
			State: "online",
			Resources: node.Resources{
				NCPU:     1,
				CPU:      85,
				Mem:      11,
				CPULimit: 90,
				MemLimit: 90,
				GPU: []node.ResourcesGPU{
					{
						Mem:        5,
						MemLimit:   90,
						Usage:      53,
						UsageLimit: 90,
						Encoder:    32,
						Decoder:    26,
					},
				},
			},
		},
	}

	planner := NewResourcePlanner(nodes)

	require.True(t, planner.HasNodeEnough("node1", Resources{
		CPU: 30,
		Mem: 5,
	}))

	require.False(t, planner.HasNodeEnough("node2", Resources{
		CPU: 30,
		Mem: 5,
	}))

	require.True(t, planner.HasNodeEnough("node1", Resources{
		CPU: 30,
		Mem: 5,
		GPU: ResourcesGPU{
			Usage:   0,
			Encoder: 0,
			Decoder: 0,
			Mem:     50,
		},
	}))

	require.False(t, planner.HasNodeEnough("node1", Resources{
		CPU: 30,
		Mem: 5,
		GPU: ResourcesGPU{
			Usage:   0,
			Encoder: 0,
			Decoder: 0,
			Mem:     86,
		},
	}))

	require.True(t, planner.HasNodeEnough("node1", Resources{
		CPU: 30,
		Mem: 5,
		GPU: ResourcesGPU{
			Usage:   0,
			Encoder: 50,
			Decoder: 0,
			Mem:     50,
		},
	}))
}

func TestResourcePlannerAdd(t *testing.T) {
	nodes := map[string]node.About{
		"node1": {
			State: "online",
			Resources: node.Resources{
				NCPU:     1,
				CPU:      7,
				Mem:      35,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
	}

	planner := NewResourcePlanner(nodes)

	planner.Add("node1", Resources{
		CPU: 42,
		Mem: 33,
	})

	require.Equal(t, map[string]node.Resources{
		"node1": {
			NCPU:     1,
			CPU:      49,
			Mem:      68,
			CPULimit: 90,
			MemLimit: 90,
		},
	}, planner.Map())
}

func TestResourcePlannerNoGPUAddGPU(t *testing.T) {
	nodes := map[string]node.About{
		"node1": {
			State: "online",
			Resources: node.Resources{
				NCPU:     1,
				CPU:      7,
				Mem:      35,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
	}

	planner := NewResourcePlanner(nodes)

	planner.Add("node1", Resources{
		CPU: 42,
		Mem: 33,
		GPU: ResourcesGPU{
			Index:   0,
			Usage:   1,
			Encoder: 2,
			Decoder: 3,
			Mem:     4,
		},
	})

	require.Equal(t, map[string]node.Resources{
		"node1": {
			NCPU:     1,
			CPU:      49,
			Mem:      68,
			CPULimit: 90,
			MemLimit: 90,
		},
	}, planner.Map())
}

func TestResourcePlannerAddGPU(t *testing.T) {
	nodes := map[string]node.About{
		"node1": {
			State: "online",
			Resources: node.Resources{
				NCPU:     1,
				CPU:      7,
				Mem:      35,
				CPULimit: 90,
				MemLimit: 90,
				GPU: []node.ResourcesGPU{
					{
						Mem:        0,
						MemLimit:   0,
						Usage:      0,
						UsageLimit: 0,
						Encoder:    0,
						Decoder:    0,
					},
					{
						Mem:        0,
						MemLimit:   100,
						Usage:      0,
						UsageLimit: 100,
						Encoder:    0,
						Decoder:    0,
					},
				},
			},
		},
	}

	planner := NewResourcePlanner(nodes)

	planner.Add("node1", Resources{
		CPU: 42,
		Mem: 33,
		GPU: ResourcesGPU{
			Usage:   1,
			Encoder: 2,
			Decoder: 3,
			Mem:     4,
		},
	})

	require.Equal(t, map[string]node.Resources{
		"node1": {
			NCPU:     1,
			CPU:      49,
			Mem:      68,
			CPULimit: 90,
			MemLimit: 90,
			GPU: []node.ResourcesGPU{
				{
					Mem:        0,
					MemLimit:   0,
					Usage:      0,
					UsageLimit: 0,
					Encoder:    0,
					Decoder:    0,
				},
				{
					Mem:        4,
					MemLimit:   100,
					Usage:      1,
					UsageLimit: 100,
					Encoder:    2,
					Decoder:    3,
				},
			},
		},
	}, planner.Map())
}

func TestResourcePlannerRemove(t *testing.T) {
	nodes := map[string]node.About{
		"node1": {
			State: "online",
			Resources: node.Resources{
				NCPU:     1,
				CPU:      53,
				Mem:      35,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
	}

	planner := NewResourcePlanner(nodes)

	planner.Remove("node1", Resources{
		CPU: 13,
		Mem: 20,
	})

	require.Equal(t, map[string]node.Resources{
		"node1": {
			NCPU:     1,
			CPU:      40,
			Mem:      15,
			CPULimit: 90,
			MemLimit: 90,
		},
	}, planner.Map())
}

func TestResourcePlannerRemoveTooMuch(t *testing.T) {
	nodes := map[string]node.About{
		"node1": {
			State: "online",
			Resources: node.Resources{
				NCPU:     1,
				CPU:      53,
				Mem:      35,
				CPULimit: 90,
				MemLimit: 90,
			},
		},
	}

	planner := NewResourcePlanner(nodes)

	planner.Remove("node1", Resources{
		CPU: 100,
		Mem: 100,
	})

	require.Equal(t, map[string]node.Resources{
		"node1": {
			NCPU:     1,
			CPU:      0,
			Mem:      0,
			CPULimit: 90,
			MemLimit: 90,
		},
	}, planner.Map())
}

func TestResourcePlannerRemoveGPU(t *testing.T) {
	nodes := map[string]node.About{
		"node1": {
			State: "online",
			Resources: node.Resources{
				NCPU:     1,
				CPU:      53,
				Mem:      35,
				CPULimit: 90,
				MemLimit: 90,
				GPU: []node.ResourcesGPU{
					{
						Mem:        4,
						MemLimit:   100,
						Usage:      1,
						UsageLimit: 100,
						Encoder:    2,
						Decoder:    3,
					},
					{
						Mem:        23,
						MemLimit:   100,
						Usage:      43,
						UsageLimit: 100,
						Encoder:    95,
						Decoder:    12,
					},
				},
			},
		},
	}

	planner := NewResourcePlanner(nodes)

	planner.Remove("node1", Resources{
		CPU: 13,
		Mem: 20,
		GPU: ResourcesGPU{
			Index:   1,
			Usage:   3,
			Encoder: 40,
			Decoder: 0,
			Mem:     5,
		},
	})

	require.Equal(t, map[string]node.Resources{
		"node1": {
			NCPU:     1,
			CPU:      40,
			Mem:      15,
			CPULimit: 90,
			MemLimit: 90,
			GPU: []node.ResourcesGPU{
				{
					Mem:        4,
					MemLimit:   100,
					Usage:      1,
					UsageLimit: 100,
					Encoder:    2,
					Decoder:    3,
				},
				{
					Mem:        18,
					MemLimit:   100,
					Usage:      40,
					UsageLimit: 100,
					Encoder:    55,
					Decoder:    12,
				},
			},
		},
	}, planner.Map())
}

func TestResourcePlannerRemoveGPUTooMuch(t *testing.T) {
	nodes := map[string]node.About{
		"node1": {
			State: "online",
			Resources: node.Resources{
				NCPU:     1,
				CPU:      53,
				Mem:      35,
				CPULimit: 90,
				MemLimit: 90,
				GPU: []node.ResourcesGPU{
					{
						Mem:        4,
						MemLimit:   100,
						Usage:      1,
						UsageLimit: 100,
						Encoder:    2,
						Decoder:    3,
					},
					{
						Mem:        23,
						MemLimit:   100,
						Usage:      43,
						UsageLimit: 100,
						Encoder:    95,
						Decoder:    12,
					},
				},
			},
		},
	}

	planner := NewResourcePlanner(nodes)

	planner.Remove("node1", Resources{
		CPU: 13,
		Mem: 20,
		GPU: ResourcesGPU{
			Index:   1,
			Usage:   100,
			Encoder: 100,
			Decoder: 100,
			Mem:     100,
		},
	})

	require.Equal(t, map[string]node.Resources{
		"node1": {
			NCPU:     1,
			CPU:      40,
			Mem:      15,
			CPULimit: 90,
			MemLimit: 90,
			GPU: []node.ResourcesGPU{
				{
					Mem:        4,
					MemLimit:   100,
					Usage:      1,
					UsageLimit: 100,
					Encoder:    2,
					Decoder:    3,
				},
				{
					Mem:        0,
					MemLimit:   100,
					Usage:      0,
					UsageLimit: 100,
					Encoder:    0,
					Decoder:    0,
				},
			},
		},
	}, planner.Map())
}
