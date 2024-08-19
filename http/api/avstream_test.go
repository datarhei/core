package api

import (
	"testing"

	"github.com/datarhei/core/v16/restream/app"

	"github.com/stretchr/testify/require"
)

func TestAVStreamIO(t *testing.T) {
	original := app.AVstreamIO{
		State:  "xxx",
		Packet: 100,
		Time:   42,
		Size:   95744,
	}

	p := AVstreamIO{}
	p.Unmarshal(&original)
	restored := p.Marshal()

	require.Equal(t, original, restored)
}

func TestAVStream(t *testing.T) {
	original := app.AVstream{
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
	}

	p := AVstream{}
	p.Unmarshal(&original)
	restored := p.Marshal()

	require.Equal(t, &original, restored)
}
