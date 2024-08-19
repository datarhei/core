package api

import (
	"testing"
	"time"

	"github.com/datarhei/core/v16/restream/app"

	"github.com/stretchr/testify/require"
)

func TestProcessReportEntry(t *testing.T) {
	original := app.ReportEntry{
		CreatedAt: time.Unix(12345, 0),
		Prelude:   []string{"lalala", "lululu"},
		Log: []app.LogLine{
			{
				Timestamp: time.Unix(123, 0),
				Data:      "xxx",
			},
			{
				Timestamp: time.Unix(124, 0),
				Data:      "yyy",
			},
		},
		Matches: []string{"match1", "match2", "match3"},
	}

	p := ProcessReportEntry{}
	p.Unmarshal(&original)
	restored := p.Marshal()

	require.Equal(t, original, restored)
}

func TestProcessReportHistoryEntry(t *testing.T) {
	original := app.ReportHistoryEntry{
		ReportEntry: app.ReportEntry{
			CreatedAt: time.Unix(12345, 0),
			Prelude:   []string{"lalala", "lululu"},
			Log: []app.LogLine{
				{
					Timestamp: time.Unix(123, 0),
					Data:      "xxx",
				},
				{
					Timestamp: time.Unix(124, 0),
					Data:      "yyy",
				},
			},
			Matches: []string{"match1", "match2", "match3"},
		},
		ExitedAt:  time.Unix(394949, 0),
		ExitState: "kaputt",
		Progress: app.Progress{
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
		},
		Usage: app.ProcessUsage{
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
		},
	}

	p := ProcessReportHistoryEntry{}
	p.Unmarshal(&original)
	restored := p.Marshal()

	require.Equal(t, original, restored)
}

func TestProcessReport(t *testing.T) {
	original := app.Report{
		ReportEntry: app.ReportEntry{
			CreatedAt: time.Unix(12345, 0),
			Prelude:   []string{"lalala", "lululu"},
			Log: []app.LogLine{
				{
					Timestamp: time.Unix(123, 0),
					Data:      "xxx",
				},
				{
					Timestamp: time.Unix(124, 0),
					Data:      "yyy",
				},
			},
			Matches: []string{"match1", "match2", "match3"},
		},
		History: []app.ReportHistoryEntry{
			{
				ReportEntry: app.ReportEntry{
					CreatedAt: time.Unix(12345, 0),
					Prelude:   []string{"lalala", "lululu"},
					Log: []app.LogLine{
						{
							Timestamp: time.Unix(123, 0),
							Data:      "xxx",
						},
						{
							Timestamp: time.Unix(124, 0),
							Data:      "yyy",
						},
					},
					Matches: []string{"match1", "match2", "match3"},
				},
				ExitedAt:  time.Unix(394949, 0),
				ExitState: "kaputt",
				Progress: app.Progress{
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
				},
				Usage: app.ProcessUsage{
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
				},
			},
		},
	}

	p := ProcessReport{}
	p.Unmarshal(&original)
	restored := p.Marshal()

	require.Equal(t, original, restored)
}
