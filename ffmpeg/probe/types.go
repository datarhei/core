package probe

type probeIO struct {
	// common
	Address  string  `json:"url"`
	Format   string  `json:"format"`
	Index    uint64  `json:"index"`
	Stream   uint64  `json:"stream"`
	Language string  `json:"language"`
	Type     string  `json:"type"`
	Codec    string  `json:"codec"`
	Coder    string  `json:"coder"`
	Bitrate  float64 `json:"bitrate_kbps"`
	Duration float64 `json:"duration_sec"`

	// video
	FPS    float64 `json:"fps"`
	Pixfmt string  `json:"pix_fmt"`
	Width  uint64  `json:"width"`
	Height uint64  `json:"height"`

	// audio
	Sampling uint64 `json:"sampling_hz"`
	Layout   string `json:"layout"`
	Channels uint64 `json:"channels"`
}

func (io *probeIO) export() ProbeIO {
	return ProbeIO{
		Address:  io.Address,
		Format:   io.Format,
		Index:    io.Index,
		Stream:   io.Stream,
		Language: io.Language,
		Type:     io.Type,
		Codec:    io.Codec,
		Coder:    io.Coder,
		Bitrate:  io.Bitrate,
		Duration: io.Duration,
		FPS:      io.FPS,
		Pixfmt:   io.Pixfmt,
		Width:    io.Width,
		Height:   io.Height,
		Sampling: io.Sampling,
		Layout:   io.Layout,
		Channels: io.Channels,
	}
}

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

type Probe struct {
	Streams []ProbeIO
	Log     []string
}
