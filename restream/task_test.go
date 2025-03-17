package restream

import (
	"testing"

	"github.com/datarhei/core/v16/restream/app"
	"github.com/stretchr/testify/require"
)

func TestAssignConfigID(t *testing.T) {
	config := app.Config{
		Input: []app.ConfigIO{
			{
				ID:      "0",
				Address: "https://example.com/3j3wr1ua_360.m3u8",
			},
			{
				ID:      "1",
				Address: "https://example.com/3j3wr1ua_720.m3u8",
			},
			{
				ID:      "2",
				Address: "https://example.com/3j3wr1ua_1080.m3u8",
			},
			{
				ID:      "3",
				Address: "anullsrc=r=44100:cl=mono",
			},
		},
		Output: []app.ConfigIO{
			{
				ID:      "0",
				Address: "/%v.m3u8",
			},
			{
				ID:      "snapshot",
				Address: "/%v.jpg",
			},
			{
				ID:      "snapshot_main",
				Address: "/main.jpg",
			},
			{
				ID:      "snapshot_main_240",
				Address: "/main_240.jpg",
			},
		},
	}

	progress := app.Progress{
		Input: []app.ProgressIO{
			{
				URL:     "https://example.com/3j3wr1ua_360.m3u8",
				Address: "https://example.com/3j3wr1ua_360.m3u8",
			},
			{
				URL:     "https://example.com/3j3wr1ua_720.m3u8",
				Address: "https://example.com/3j3wr1ua_720.m3u8",
			},
			{
				URL:     "https://example.com/3j3wr1ua_1080.m3u8",
				Address: "https://example.com/3j3wr1ua_1080.m3u8",
			},
			{
				URL:     "anullsrc=r=44100:cl=mono",
				Address: "anullsrc=r=44100:cl=mono",
			},
		},
		Output: []app.ProgressIO{
			{
				URL:     "/%v.m3u8",
				Address: "/0.m3u8",
			},
			{
				URL:     "/%v.m3u8",
				Address: "/1.m3u8",
			},
			{
				URL:     "/%v.m3u8",
				Address: "/2.m3u8",
			},
			{
				URL:     "/%v.m3u8",
				Address: "/0.m3u8",
			},
			{
				URL:     "/%v.m3u8",
				Address: "/1.m3u8",
			},
			{
				URL:     "/%v.m3u8",
				Address: "/2.m3u8",
			},
			{
				URL:     "/%v.jpg",
				Address: "/%v.jpg",
			},
			{
				URL:     "/%v.jpg",
				Address: "/%v.jpg",
			},
			{
				URL:     "/%v.jpg",
				Address: "/%v.jpg",
			},
			{
				URL:     "/main.jpg",
				Address: "/main.jpg",
			},
			{
				URL:     "/main_240.jpg",
				Address: "/main_240.jpg",
			},
		},
	}

	progress.Input = assignConfigID(progress.Input, config.Input)
	require.Equal(t, []app.ProgressIO{
		{
			ID:      "0",
			URL:     "https://example.com/3j3wr1ua_360.m3u8",
			Address: "https://example.com/3j3wr1ua_360.m3u8",
		},
		{
			ID:      "1",
			URL:     "https://example.com/3j3wr1ua_720.m3u8",
			Address: "https://example.com/3j3wr1ua_720.m3u8",
		},
		{
			ID:      "2",
			URL:     "https://example.com/3j3wr1ua_1080.m3u8",
			Address: "https://example.com/3j3wr1ua_1080.m3u8",
		},
		{
			ID:      "3",
			URL:     "anullsrc=r=44100:cl=mono",
			Address: "anullsrc=r=44100:cl=mono",
		},
	}, progress.Input)

	progress.Output = assignConfigID(progress.Output, config.Output)
	require.Equal(t, []app.ProgressIO{
		{
			ID:      "0",
			URL:     "/%v.m3u8",
			Address: "/0.m3u8",
		},
		{
			ID:      "0",
			URL:     "/%v.m3u8",
			Address: "/1.m3u8",
		},
		{
			ID:      "0",
			URL:     "/%v.m3u8",
			Address: "/2.m3u8",
		},
		{
			ID:      "0",
			URL:     "/%v.m3u8",
			Address: "/0.m3u8",
		},
		{
			ID:      "0",
			URL:     "/%v.m3u8",
			Address: "/1.m3u8",
		},
		{
			ID:      "0",
			URL:     "/%v.m3u8",
			Address: "/2.m3u8",
		},
		{
			ID:      "snapshot",
			URL:     "/%v.jpg",
			Address: "/%v.jpg",
		},
		{
			ID:      "snapshot",
			URL:     "/%v.jpg",
			Address: "/%v.jpg",
		},
		{
			ID:      "snapshot",
			URL:     "/%v.jpg",
			Address: "/%v.jpg",
		},
		{
			ID:      "snapshot_main",
			URL:     "/main.jpg",
			Address: "/main.jpg",
		},
		{
			ID:      "snapshot_main_240",
			URL:     "/main_240.jpg",
			Address: "/main_240.jpg",
		},
	}, progress.Output)
}
