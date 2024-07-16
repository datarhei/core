package nvidia

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseNV(t *testing.T) {
	data, err := os.ReadFile("./fixtures/data1.xml")
	require.NoError(t, err)

	nv, err := parse(data)
	require.NoError(t, err)

	require.Equal(t, Stats{
		GPU: []GPUStats{
			{
				Name:         "NVIDIA GeForce GTX 1080",
				Architecture: "Pascal",
				MemoryTotal:  8119 * 1024 * 1024,
				MemoryUsed:   918 * 1024 * 1024,
				Usage:        15,
				MemoryUsage:  7,
				EncoderUsage: 3,
				DecoderUsage: 0,
				Process: []Process{
					{
						PID:    18179,
						Memory: 916 * 1024 * 1024,
					},
				},
			},
		},
	}, nv)

	data, err = os.ReadFile("./fixtures/data2.xml")
	require.NoError(t, err)

	nv, err = parse(data)
	require.NoError(t, err)

	require.Equal(t, Stats{
		GPU: []GPUStats{
			{
				Name:         "NVIDIA L4",
				Architecture: "Ada Lovelace",
				MemoryTotal:  23034 * 1024 * 1024,
				MemoryUsed:   1 * 1024 * 1024,
				Usage:        2,
				MemoryUsage:  0,
				EncoderUsage: 0,
				DecoderUsage: 0,
			},
			{
				Name:         "NVIDIA L4",
				Architecture: "Ada Lovelace",
				MemoryTotal:  23034 * 1024 * 1024,
				MemoryUsed:   1 * 1024 * 1024,
				Usage:        3,
				MemoryUsage:  0,
				EncoderUsage: 0,
				DecoderUsage: 0,
			},
		},
	}, nv)

	data, err = os.ReadFile("./fixtures/data3.xml")
	require.NoError(t, err)

	nv, err = parse(data)
	require.NoError(t, err)

	require.Equal(t, Stats{
		GPU: []GPUStats{
			{
				Name:         "GeForce GTX 1080",
				MemoryTotal:  8119 * 1024 * 1024,
				MemoryUsed:   2006 * 1024 * 1024,
				Usage:        32,
				MemoryUsage:  11,
				EncoderUsage: 17,
				DecoderUsage: 25,
				Process: []Process{
					{
						PID:    10131,
						Memory: 389 * 1024 * 1024,
					},
					{
						PID:    13597,
						Memory: 1054 * 1024 * 1024,
					},
					{
						PID:    16870,
						Memory: 549 * 1024 * 1024,
					},
				},
			},
		},
	}, nv)
}
