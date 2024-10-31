package app

import (
	"testing"

	"github.com/datarhei/core/v16/ffmpeg/parse"
	"github.com/stretchr/testify/require"
)

func TestProgressIO(t *testing.T) {
	original := parse.ProgressIO{
		Address:  "fhdj",
		Index:    2,
		Stream:   4,
		Format:   "yuv420p",
		Type:     "video",
		Codec:    "h264",
		Coder:    "libx264",
		Profile:  848,
		Level:    48,
		Frame:    39,
		Keyframe: 433,
		Framerate: struct {
			Min     float64
			Max     float64
			Average float64
		}{
			Min:     47.0,
			Max:     97.8,
			Average: 463.9,
		},
		FPS:       34.8,
		Packet:    4737,
		PPS:       473.8,
		Size:      48474,
		Bitrate:   38473,
		Extradata: 4874,
		Pixfmt:    "none",
		Quantizer: 2.3,
		Width:     4848,
		Height:    9373,
		Samplefmt: "fltp",
		Sampling:  4733,
		Layout:    "atmos",
		Channels:  83,
	}

	p := ProgressIO{
		AVstream: nil,
	}
	p.UnmarshalParser(&original)
	restored := p.MarshalParser()

	require.Equal(t, original, restored)
}

func TestProgressIOWithAVstream(t *testing.T) {
	original := parse.ProgressIO{
		Address:  "fhdj",
		Index:    2,
		Stream:   4,
		Format:   "yuv420p",
		Type:     "video",
		Codec:    "h264",
		Coder:    "libx264",
		Profile:  848,
		Level:    48,
		Frame:    39,
		Keyframe: 433,
		Framerate: struct {
			Min     float64
			Max     float64
			Average float64
		}{
			Min:     47.0,
			Max:     97.8,
			Average: 463.9,
		},
		FPS:       34.8,
		Packet:    4737,
		PPS:       473.8,
		Size:      48474,
		Bitrate:   38473,
		Extradata: 4874,
		Pixfmt:    "none",
		Quantizer: 2.3,
		Width:     4848,
		Height:    9373,
		Samplefmt: "fltp",
		Sampling:  4733,
		Layout:    "atmos",
		Channels:  83,
		AVstream: &parse.AVstream{
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
		},
	}

	p := ProgressIO{
		AVstream: nil,
	}
	p.UnmarshalParser(&original)
	restored := p.MarshalParser()

	require.Equal(t, original, restored)
}

func TestGraphMapping(t *testing.T) {
	original := parse.GraphMapping{
		Input:  1,
		Output: 3,
		Index:  39,
		Name:   "foobar",
		Copy:   true,
	}

	p := GraphMapping{}
	p.UnmarshalParser(&original)
	restored := p.MarshalParser()

	require.Equal(t, original, restored)
}

func TestGraphElement(t *testing.T) {
	original := parse.GraphElement{
		Index:     5,
		Name:      "foobar",
		Filter:    "infilter",
		DstName:   "outfilter_",
		DstFilter: "outfilter",
		Inpad:     "inpad",
		Outpad:    "outpad",
		Timebase:  "100",
		Type:      "video",
		Format:    "yuv420p",
		Sampling:  39944,
		Layout:    "atmos",
		Width:     1029,
		Height:    463,
	}

	p := GraphElement{}
	p.UnmarshalParser(&original)
	restored := p.MarshalParser()

	require.Equal(t, original, restored)
}

func TestStreamMapping(t *testing.T) {
	original := parse.StreamMapping{
		Graphs: []parse.GraphElement{
			{
				Index:     5,
				Name:      "foobar",
				Filter:    "infilter",
				DstName:   "outfilter_",
				DstFilter: "outfilter",
				Inpad:     "inpad",
				Outpad:    "outpad",
				Timebase:  "100",
				Type:      "video",
				Format:    "yuv420p",
				Sampling:  39944,
				Layout:    "atmos",
				Width:     1029,
				Height:    463,
			},
		},
		Mapping: []parse.GraphMapping{
			{
				Input:  1,
				Output: 3,
				Index:  39,
				Name:   "foobar",
				Copy:   true,
			},
		},
	}

	p := StreamMapping{}
	p.UnmarshalParser(&original)
	restored := p.MarshalParser()

	require.Equal(t, original, restored)
}

func TestProgress(t *testing.T) {
	original := parse.Progress{
		Started: false,
		Input: []parse.ProgressIO{
			{
				Address:  "fhd873487j",
				Index:    2,
				Stream:   4,
				Format:   "yuv420p",
				Type:     "video",
				Codec:    "h264",
				Coder:    "libx264",
				Profile:  848,
				Level:    48,
				Frame:    39,
				Keyframe: 433,
				Framerate: struct {
					Min     float64
					Max     float64
					Average float64
				}{
					Min:     47.0,
					Max:     97.8,
					Average: 463.9,
				},
				FPS:       34.8,
				Packet:    4737,
				PPS:       473.8,
				Size:      48474,
				Bitrate:   38473,
				Extradata: 4874,
				Pixfmt:    "none",
				Quantizer: 2.3,
				Width:     4848,
				Height:    9373,
				Samplefmt: "fltp",
				Sampling:  4733,
				Layout:    "atmos",
				Channels:  83,
				AVstream: &parse.AVstream{
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
				},
			},
		},
		Output: []parse.ProgressIO{
			{
				Address:  "fhdj",
				Index:    2,
				Stream:   4,
				Format:   "yuv420p",
				Type:     "video",
				Codec:    "h264",
				Coder:    "libx264",
				Profile:  848,
				Level:    48,
				Frame:    39,
				Keyframe: 433,
				Framerate: struct {
					Min     float64
					Max     float64
					Average float64
				}{
					Min:     47.0,
					Max:     97.8,
					Average: 463.9,
				},
				FPS:       34.8,
				Packet:    4737,
				PPS:       473.8,
				Size:      48474,
				Bitrate:   38473,
				Extradata: 4874,
				Pixfmt:    "none",
				Quantizer: 2.3,
				Width:     4848,
				Height:    9373,
				Samplefmt: "fltp",
				Sampling:  4733,
				Layout:    "atmos",
				Channels:  83,
				AVstream: &parse.AVstream{
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
				},
			},
		},
		Mapping: parse.StreamMapping{
			Graphs: []parse.GraphElement{
				{
					Index:     5,
					Name:      "foobar",
					Filter:    "infilter",
					DstName:   "outfilter_",
					DstFilter: "outfilter",
					Inpad:     "inpad",
					Outpad:    "outpad",
					Timebase:  "100",
					Type:      "video",
					Format:    "yuv420p",
					Sampling:  39944,
					Layout:    "atmos",
					Width:     1029,
					Height:    463,
				},
			},
			Mapping: []parse.GraphMapping{
				{
					Input:  1,
					Output: 3,
					Index:  39,
					Name:   "foobar",
					Copy:   true,
				},
			},
		},
		Frame:     0,
		Packet:    0,
		FPS:       0,
		PPS:       0,
		Quantizer: 0,
		Size:      0,
		Time:      0,
		Bitrate:   0,
		Speed:     0,
		Drop:      0,
		Dup:       0,
	}

	p := Progress{}
	p.UnmarshalParser(&original)
	restored := p.MarshalParser()

	require.Equal(t, original, restored)
}
