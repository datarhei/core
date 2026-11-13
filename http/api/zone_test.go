package api

import (
	"testing"

	"github.com/datarhei/core/v16/zone"

	"github.com/stretchr/testify/require"
)

func TestZone(t *testing.T) {
	original := zone.Zone{
		ID: "a",
		Limits: zone.Resources{
			CPU:        100.5,
			Memory:     512 * 1024 * 1024,
			GPUUsage:   10,
			GPUEncoder: 20,
			GPUDecoder: 30,
			GPUMemory:  1024 * 1024 * 1024,
		},
	}

	z := Zone{}
	z.Marshal(original)

	require.Equal(t, Zone{
		ID: "a",
		Limits: ZoneResources{
			CPU:        100.5,
			Memory:     512,
			GPUUsage:   10,
			GPUEncoder: 20,
			GPUDecoder: 30,
			GPUMemory:  1024,
		},
	}, z)

	require.Equal(t, original, z.Unmarshal())
}

func TestZoneInfo(t *testing.T) {
	info := ZoneInfo{}
	info.Marshal(zone.Info{
		Zone:      zone.Zone{ID: zone.DefaultID, Limits: zone.Resources{CPU: 100}},
		Usage:     zone.Resources{CPU: 60, Memory: 3 * 1024 * 1024},
		Processes: 2,
	})

	require.Equal(t, ZoneInfo{
		Zone:      Zone{ID: "$default", Limits: ZoneResources{CPU: 100}},
		Usage:     ZoneResources{CPU: 60, Memory: 3},
		Processes: 2,
	}, info)
}
