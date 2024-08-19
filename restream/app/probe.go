package app

import "github.com/datarhei/core/v16/ffmpeg/probe"

type ProbeIO struct {
	Address string

	// General
	Index    uint64
	Stream   uint64
	Language string
	Format   string
	Type     string
	Codec    string
	Coder    string
	Bitrate  float64 // kbit/s
	Duration float64

	// Video
	Pixfmt string
	Width  uint64
	Height uint64
	FPS    float64

	// Audio
	Sampling uint64
	Layout   string
	Channels uint64
}

func (p *ProbeIO) UnmarshalProber(pp *probe.ProbeIO) {
	p.Address = pp.Address
	p.Index = pp.Index
	p.Stream = pp.Stream
	p.Language = pp.Language
	p.Format = pp.Format
	p.Type = pp.Type
	p.Codec = pp.Codec
	p.Coder = pp.Coder
	p.Bitrate = pp.Bitrate
	p.Duration = pp.Duration
	p.Pixfmt = pp.Pixfmt
	p.Width = pp.Width
	p.Height = pp.Height
	p.FPS = pp.FPS
	p.Sampling = pp.Sampling
	p.Layout = pp.Layout
	p.Channels = pp.Channels
}

type Probe struct {
	Streams []ProbeIO
	Log     []string
}

func (p *Probe) UnmarshalProber(pp *probe.Probe) {
	p.Log = make([]string, len(pp.Log))
	copy(p.Log, pp.Log)

	p.Streams = make([]ProbeIO, len(pp.Streams))

	for i, s := range pp.Streams {
		p.Streams[i].UnmarshalProber(&s)
	}
}
