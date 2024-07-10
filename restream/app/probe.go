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

/*
	func (app *ProbeIO) MarshallAPI() api.ProbeIO {
		return api.ProbeIO{
			Address:  app.Address,
			Format:   app.Format,
			Index:    app.Index,
			Stream:   app.Stream,
			Language: app.Language,
			Type:     app.Type,
			Codec:    app.Codec,
			Coder:    app.Coder,
			Bitrate:  json.ToNumber(app.Bitrate),
			Duration: json.ToNumber(app.Duration),
			FPS:      json.ToNumber(app.FPS),
			Pixfmt:   app.Pixfmt,
			Width:    app.Width,
			Height:   app.Height,
			Sampling: app.Sampling,
			Layout:   app.Layout,
			Channels: app.Channels,
		}
	}
*/
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

/*
// Unmarshal converts a restreamer Probe to a Probe in API representation
func (app *Probe) MarshallAPI() api.Probe {
	p := api.Probe{
		Streams: make([]api.ProbeIO, len(app.Streams)),
		Log:     make([]string, len(app.Log)),
	}

	for i, io := range app.Streams {
		p.Streams[i] = io.MarshallAPI()
	}

	copy(p.Log, app.Log)

	return p
}
*/
