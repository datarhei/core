package app

import (
	"bytes"
	"testing"

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
		LimitWaitFor:   20,
	}

	hash1 := config.Hash()

	require.Equal(t, []byte{0x7e, 0xae, 0x5b, 0xc3, 0xad, 0xe3, 0x9a, 0xfc, 0xd3, 0x49, 0x15, 0x28, 0x93, 0x17, 0xc5, 0xbf}, hash1)

	config.Reconnect = false

	hash2 := config.Hash()

	require.False(t, bytes.Equal(hash1, hash2))
}
