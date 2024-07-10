package api

import (
	"testing"

	"github.com/datarhei/core/v16/restream/app"

	"github.com/stretchr/testify/require"
)

func TestProcessUsageCPU(t *testing.T) {
	original := app.ProcessUsageCPU{
		NCPU:         1.5,
		Current:      0.7,
		Average:      0.9,
		Max:          1.3,
		Limit:        100,
		IsThrottling: true,
	}

	p := ProcessUsageCPU{}
	p.Unmarshal(&original)
	restored := p.Marshal()

	require.Equal(t, original, restored)
}

func TestProcessUsageMemory(t *testing.T) {
	original := app.ProcessUsageMemory{
		Current: 100,
		Average: 72,
		Max:     150,
		Limit:   200,
	}

	p := ProcessUsageMemory{}
	p.Unmarshal(&original)
	restored := p.Marshal()

	require.Equal(t, original, restored)
}

func TestProcessUsage(t *testing.T) {
	original := app.ProcessUsage{
		CPU: app.ProcessUsageCPU{
			NCPU:         1.5,
			Current:      0.7,
			Average:      0.9,
			Max:          1.3,
			Limit:        100,
			IsThrottling: true,
		},
		Memory: app.ProcessUsageMemory{
			Current: 100,
			Average: 72,
			Max:     150,
			Limit:   200,
		},
	}

	p := ProcessUsage{}
	p.Unmarshal(&original)
	restored := p.Marshal()

	require.Equal(t, original, restored)
}

func TestProcessConfig(t *testing.T) {
	original := app.Config{
		ID:        "foobar",
		Reference: "none",
		Owner:     "me",
		Domain:    "all",
		Input: []app.ConfigIO{
			{
				ID:      "in",
				Address: "example_in",
				Options: []string{"io1", "io2"},
			},
		},
		Output: []app.ConfigIO{
			{
				ID:      "out",
				Address: "example_out",
				Options: []string{"oo1", "oo2", "oo3"},
				Cleanup: []app.ConfigIOCleanup{
					{
						Pattern:       "xxxx",
						MaxFiles:      5,
						MaxFileAge:    100,
						PurgeOnDelete: true,
					},
				},
			},
		},
		Options:        []string{"o1", "o2", "o3"},
		Reconnect:      true,
		ReconnectDelay: 20,
		Autostart:      true,
		StaleTimeout:   50,
		Timeout:        60,
		Scheduler:      "xxx",
		LogPatterns:    []string{"bla", "blubb"},
		LimitCPU:       10,
		LimitMemory:    100 * 1024 * 1024,
		LimitWaitFor:   20,
	}

	p := ProcessConfig{}
	p.Unmarshal(&original)
	restored, _ := p.Marshal()

	require.Equal(t, &original, restored)
}
