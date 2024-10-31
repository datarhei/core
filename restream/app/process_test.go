package app

import (
	"bytes"
	"testing"

	"github.com/datarhei/core/v16/ffmpeg/parse"
	"github.com/stretchr/testify/require"
)

func TestCreateCommand(t *testing.T) {
	config := &Config{
		Options: []string{"-global", "global"},
		Input: []ConfigIO{
			{Address: "inputAddress", Options: []string{"-input", "inputoption"}},
		},
		Output: []ConfigIO{
			{Address: "outputAddress", Options: []string{"-output", "outputoption"}},
		},
	}

	command := config.CreateCommand()
	require.Equal(t, []string{
		"-global", "global",
		"-input", "inputoption", "-i", "inputAddress",
		"-output", "outputoption", "outputAddress",
	}, command)
}

func TestConfigHash(t *testing.T) {
	config := &Config{
		ID:             "id",
		Reference:      "ref",
		Owner:          "owner",
		Domain:         "domain",
		FFVersion:      "1.2.3",
		Input:          []ConfigIO{{Address: "inputAddress", Options: []string{"-input", "inputoption"}}},
		Output:         []ConfigIO{{Address: "outputAddress", Options: []string{"-output", "outputoption"}}},
		Options:        []string{"-global", "global"},
		Reconnect:      true,
		ReconnectDelay: 15,
		Autostart:      false,
		StaleTimeout:   42,
		Timeout:        9,
		Scheduler:      "* * * * *",
		LogPatterns:    []string{"^libx264"},
		LimitCPU:       50,
		LimitMemory:    3 * 1024 * 1024,
		LimitGPU: ConfigLimitGPU{
			Usage:   10,
			Encoder: 42,
			Decoder: 14,
			Memory:  500 * 1024 * 1024,
		},
		LimitWaitFor: 20,
	}

	hash1 := config.Hash()

	require.Equal(t, []byte{0x5e, 0x85, 0xc3, 0xc5, 0x44, 0xfd, 0x3e, 0x10, 0x13, 0x76, 0x36, 0x8b, 0xbe, 0x7e, 0xa6, 0xbb}, hash1)

	config.Reconnect = false

	hash2 := config.Hash()

	require.False(t, bytes.Equal(hash1, hash2))
}

func TestProcessUsageCPU(t *testing.T) {
	original := parse.UsageCPU{
		NCPU:    1.5,
		Average: 0.9,
		Max:     1.3,
		Limit:   100,
	}

	p := ProcessUsageCPU{}
	p.UnmarshalParser(&original)
	restored := p.MarshalParser()

	require.Equal(t, original, restored)
}

func TestProcessUsageMemory(t *testing.T) {
	original := parse.UsageMemory{
		Average: 72,
		Max:     150,
		Limit:   200,
	}

	p := ProcessUsageMemory{}
	p.UnmarshalParser(&original)
	restored := p.MarshalParser()

	require.Equal(t, original, restored)
}

func TestProcessUsage(t *testing.T) {
	original := parse.Usage{
		CPU: parse.UsageCPU{
			NCPU:    1.5,
			Average: 0.9,
			Max:     1.3,
			Limit:   100,
		},
		Memory: parse.UsageMemory{
			Average: 72,
			Max:     150,
			Limit:   200,
		},
	}

	p := ProcessUsage{}
	p.UnmarshalParser(&original)
	restored := p.MarshalParser()

	require.Equal(t, original, restored)
}
