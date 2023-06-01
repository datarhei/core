package api

// ProbeIO represents a stream of a probed file
type ProbeIO struct {
	// common
	Address  string  `json:"url"`
	Format   string  `json:"format"`
	Index    uint64  `json:"index" format:"uint64"`
	Stream   uint64  `json:"stream" format:"uint64"`
	Language string  `json:"language"`
	Type     string  `json:"type"`
	Codec    string  `json:"codec"`
	Coder    string  `json:"coder"`
	Bitrate  float64 `json:"bitrate_kbps" swaggertype:"number" jsonschema:"type=number"`
	Duration float64 `json:"duration_sec"  swaggertype:"number" jsonschema:"type=number"`

	// video
	FPS    float64 `json:"fps" swaggertype:"number" jsonschema:"type=number"`
	Pixfmt string  `json:"pix_fmt"`
	Width  uint64  `json:"width" format:"uint64"`
	Height uint64  `json:"height" format:"uint64"`

	// audio
	Sampling uint64 `json:"sampling_hz" format:"uint64"`
	Layout   string `json:"layout"`
	Channels uint64 `json:"channels" format:"uint64"`
}

// Probe represents the result of probing a file. It has a list of detected streams
// and a list of log lone from the probe process.
type Probe struct {
	Streams []ProbeIO `json:"streams"`
	Log     []string  `json:"log"`
}
