package app

import (
	"testing"

	"github.com/datarhei/core/v16/ffmpeg/parse"
	"github.com/stretchr/testify/require"
)

func TestProgressIO(t *testing.T) {
	original := parse.ProgressIO{
		Address:  "",
		Index:    0,
		Stream:   0,
		Format:   "",
		Type:     "",
		Codec:    "",
		Coder:    "",
		Frame:    0,
		Keyframe: 0,
		Framerate: struct {
			Min     float64
			Max     float64
			Average float64
		}{},
		FPS:       0,
		Packet:    0,
		PPS:       0,
		Size:      0,
		Bitrate:   0,
		Extradata: 0,
		Pixfmt:    "",
		Quantizer: 0,
		Width:     0,
		Height:    0,
		Sampling:  0,
		Layout:    "",
		Channels:  0,
		AVstream:  &parse.AVstream{},
	}

	p := ProgressIO{
		AVstream: nil,
	}
	p.UnmarshalParser(&original)
	restored := p.MarshalParser()

	require.Equal(t, original, restored)
}
