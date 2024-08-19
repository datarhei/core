package app

import (
	"testing"

	"github.com/datarhei/core/v16/ffmpeg/parse"
	"github.com/stretchr/testify/require"
)

func TestAVstreamIO(t *testing.T) {
	original := parse.AVstreamIO{
		State:  "running",
		Packet: 484,
		Time:   4373,
		Size:   4783,
	}

	p := AVstreamIO{}
	p.UnmarshalParser(&original)
	restored := p.MarshalParser()

	require.Equal(t, original, restored)
}

func TestAVstreamSwap(t *testing.T) {
	original := parse.AVStreamSwap{
		URL:       "ffdsjhhj",
		Status:    "none",
		LastURL:   "fjfd",
		LastError: "none",
	}

	p := AVStreamSwap{}
	p.UnmarshalParser(&original)
	restored := p.MarshalParser()

	require.Equal(t, original, restored)
}

func TestAVstream(t *testing.T) {
	original := parse.AVstream{
		Input: parse.AVstreamIO{
			State:  "running",
			Packet: 484,
			Time:   4373,
			Size:   4783,
		},
		Output: parse.AVstreamIO{
			State:  "idle",
			Packet: 4843,
			Time:   483,
			Size:   34,
		},
		Aqueue:         8574,
		Queue:          5877,
		Dup:            473,
		Drop:           463,
		Enc:            474,
		Looping:        true,
		LoopingRuntime: 347,
		Duplicating:    true,
		GOP:            "xxx",
		Mode:           "yyy",
		Debug:          nil,
		Swap: parse.AVStreamSwap{
			URL:       "ffdsjhhj",
			Status:    "none",
			LastURL:   "fjfd",
			LastError: "none",
		},
	}

	p := AVstream{}
	p.UnmarshalParser(&original)
	restored := p.MarshalParser()

	require.Equal(t, &original, restored)
}
