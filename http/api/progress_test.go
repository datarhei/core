package api

import (
	"testing"

	"github.com/datarhei/core/v16/restream/app"

	"github.com/stretchr/testify/require"
)

func TestGraphMapping(t *testing.T) {
	original := app.GraphMapping{
		Input:  1,
		Output: 3,
		Index:  39,
		Name:   "foobar",
		Copy:   true,
	}

	p := GraphMapping{}
	p.Unmarshal(&original)
	restored := p.Marshal()

	require.Equal(t, original, restored)
}

func TestGraphElement(t *testing.T) {
	original := app.GraphElement{
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
	p.Unmarshal(&original)
	restored := p.Marshal()

	require.Equal(t, original, restored)
}

func TestStreamMapping(t *testing.T) {
	original := app.StreamMapping{
		Graphs: []app.GraphElement{
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
		Mapping: []app.GraphMapping{
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
	p.Unmarshal(&original)
	restored := p.Marshal()

	require.Equal(t, original, restored)
}

func TestProgressIO(t *testing.T) {
	original := app.ProgressIO{
		ID:       "id",
		Address:  "jfdk",
		Index:    4,
		Stream:   7,
		Format:   "rtmp",
		Type:     "video",
		Codec:    "x",
		Coder:    "y",
		Frame:    133,
		Keyframe: 39,
		Framerate: app.ProgressIOFramerate{
			Min:     12.5,
			Max:     30.0,
			Average: 25.9,
		},
		FPS:       25.3,
		Packet:    442,
		PPS:       45.5,
		Size:      45944 * 1024,
		Bitrate:   5848.22 * 1024,
		Extradata: 34,
		Pixfmt:    "yuv420p",
		Quantizer: 494.2,
		Width:     10393,
		Height:    4933,
		Samplefmt: "fltp",
		Sampling:  58483,
		Layout:    "atmos",
		Channels:  4944,
		AVstream:  nil,
	}

	p := ProgressIO{}
	p.Unmarshal(&original)
	restored := p.Marshal()

	require.Equal(t, original, restored)
}

func TestProgressIOAVstream(t *testing.T) {
	original := app.ProgressIO{
		ID:       "id",
		Address:  "jfdk",
		Index:    4,
		Stream:   7,
		Format:   "rtmp",
		Type:     "video",
		Codec:    "x",
		Coder:    "y",
		Frame:    133,
		Keyframe: 39,
		Framerate: app.ProgressIOFramerate{
			Min:     12.5,
			Max:     30.0,
			Average: 25.9,
		},
		FPS:       25.3,
		Packet:    442,
		PPS:       45.5,
		Size:      45944 * 1024,
		Bitrate:   5848.22 * 1024,
		Extradata: 34,
		Pixfmt:    "yuv420p",
		Quantizer: 494.2,
		Width:     10393,
		Height:    4933,
		Samplefmt: "fltp",
		Sampling:  58483,
		Layout:    "atmos",
		Channels:  4944,
		AVstream: &app.AVstream{
			Input: app.AVstreamIO{
				State:  "xxx",
				Packet: 100,
				Time:   42,
				Size:   95744,
			},
			Output: app.AVstreamIO{
				State:  "yyy",
				Packet: 7473,
				Time:   57634,
				Size:   363,
			},
			Aqueue:         3829,
			Queue:          4398,
			Dup:            47,
			Drop:           85,
			Enc:            4578,
			Looping:        true,
			LoopingRuntime: 483,
			Duplicating:    true,
			GOP:            "gop",
			Mode:           "mode",
		},
	}

	p := ProgressIO{}
	p.Unmarshal(&original)
	restored := p.Marshal()

	require.Equal(t, original, restored)
}

func TestProgress(t *testing.T) {
	original := app.Progress{
		Started: true,
		Input: []app.ProgressIO{
			{
				ID:       "id",
				Address:  "jfdk",
				Index:    4,
				Stream:   7,
				Format:   "rtmp",
				Type:     "video",
				Codec:    "x",
				Coder:    "y",
				Frame:    133,
				Keyframe: 39,
				Framerate: app.ProgressIOFramerate{
					Min:     12.5,
					Max:     30.0,
					Average: 25.9,
				},
				FPS:       25.3,
				Packet:    442,
				PPS:       45.5,
				Size:      45944 * 1024,
				Bitrate:   5848.22 * 1024,
				Extradata: 34,
				Pixfmt:    "yuv420p",
				Quantizer: 494.2,
				Width:     10393,
				Height:    4933,
				Samplefmt: "fltp",
				Sampling:  58483,
				Layout:    "atmos",
				Channels:  4944,
				AVstream: &app.AVstream{
					Input: app.AVstreamIO{
						State:  "xxx",
						Packet: 100,
						Time:   42,
						Size:   95744,
					},
					Output: app.AVstreamIO{
						State:  "yyy",
						Packet: 7473,
						Time:   57634,
						Size:   363,
					},
					Aqueue:         3829,
					Queue:          4398,
					Dup:            47,
					Drop:           85,
					Enc:            4578,
					Looping:        true,
					LoopingRuntime: 483,
					Duplicating:    true,
					GOP:            "gop",
					Mode:           "mode",
				},
			},
		},
		Output: []app.ProgressIO{
			{
				ID:       "id",
				Address:  "jfdk",
				Index:    4,
				Stream:   7,
				Format:   "rtmp",
				Type:     "video",
				Codec:    "x",
				Coder:    "y",
				Frame:    133,
				Keyframe: 39,
				Framerate: app.ProgressIOFramerate{
					Min:     12.5,
					Max:     30.0,
					Average: 25.9,
				},
				FPS:       25.3,
				Packet:    442,
				PPS:       45.5,
				Size:      45944 * 1024,
				Bitrate:   5848.22 * 1024,
				Extradata: 34,
				Pixfmt:    "yuv420p",
				Quantizer: 494.2,
				Width:     10393,
				Height:    4933,
				Samplefmt: "fltp",
				Sampling:  58483,
				Layout:    "atmos",
				Channels:  4944,
				AVstream:  nil,
			},
		},
		Mapping: app.StreamMapping{
			Graphs: []app.GraphElement{
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
			Mapping: []app.GraphMapping{
				{
					Input:  1,
					Output: 3,
					Index:  39,
					Name:   "foobar",
					Copy:   true,
				},
			},
		},
		Frame:     329,
		Packet:    4343,
		FPS:       84.2,
		Quantizer: 234.2,
		Size:      339393 * 1024,
		Time:      494,
		Bitrate:   33848.2 * 1024,
		Speed:     293.2,
		Drop:      2393,
		Dup:       5958,
	}

	p := Progress{}
	p.Unmarshal(&original)
	restored := p.Marshal()

	require.Equal(t, original, restored)
}
