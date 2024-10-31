package parse

import (
	"time"

	"github.com/datarhei/core/v16/math/average"
)

type averager struct {
	fps     *average.SMA
	pps     *average.SMA
	bitrate *average.SMA
}

func (a *averager) init(window, granularity time.Duration) {
	a.fps, _ = average.NewSMA(window, granularity)
	a.pps, _ = average.NewSMA(window, granularity)
	a.bitrate, _ = average.NewSMA(window, granularity)
}

func (a *averager) stop() {
	a.fps.Reset()
	a.pps.Reset()
	a.bitrate.Reset()
}
