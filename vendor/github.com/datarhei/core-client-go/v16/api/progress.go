package api

type ProgressIOFramerate struct {
	Min     float64 `json:"min" swaggertype:"number" jsonschema:"type=number"`
	Max     float64 `json:"max" swaggertype:"number" jsonschema:"type=number"`
	Average float64 `json:"avg" swaggertype:"number" jsonschema:"type=number"`
}

// ProgressIO represents the progress of an ffmpeg input or output
type ProgressIO struct {
	ID      string `json:"id" jsonschema:"minLength=1"`
	Address string `json:"address" jsonschema:"minLength=1"`

	// General
	Index     uint64              `json:"index" format:"uint64"`
	Stream    uint64              `json:"stream" format:"uint64"`
	Format    string              `json:"format"`
	Type      string              `json:"type"`
	Codec     string              `json:"codec"`
	Coder     string              `json:"coder"`
	Frame     uint64              `json:"frame" format:"uint64"`
	Keyframe  uint64              `json:"keyframe" format:"uint64"`
	Framerate ProgressIOFramerate `json:"framerate"`
	FPS       float64             `json:"fps" swaggertype:"number" jsonschema:"type=number"`
	Packet    uint64              `json:"packet" format:"uint64"`
	PPS       float64             `json:"pps" swaggertype:"number" jsonschema:"type=number"`
	Size      uint64              `json:"size_kb" format:"uint64"`                                    // kbytes
	Bitrate   float64             `json:"bitrate_kbit" swaggertype:"number" jsonschema:"type=number"` // kbit/s
	Extradata uint64              `json:"extradata_size_bytes" format:"uint64"`                       // bytes

	// Video
	Pixfmt    string  `json:"pix_fmt,omitempty"`
	Quantizer float64 `json:"q,omitempty" swaggertype:"number" jsonschema:"type=number"`
	Width     uint64  `json:"width,omitempty" format:"uint64"`
	Height    uint64  `json:"height,omitempty" format:"uint64"`

	// Audio
	Sampling uint64 `json:"sampling_hz,omitempty" format:"uint64"`
	Layout   string `json:"layout,omitempty"`
	Channels uint64 `json:"channels,omitempty" format:"uint64"`

	// avstream
	AVstream *AVstream `json:"avstream"`
}

// Progress represents the progress of an ffmpeg process
type Progress struct {
	Input     []ProgressIO `json:"inputs"`
	Output    []ProgressIO `json:"outputs"`
	Frame     uint64       `json:"frame" format:"uint64"`
	Packet    uint64       `json:"packet" format:"uint64"`
	FPS       float64      `json:"fps" swaggertype:"number" jsonschema:"type=number"`
	Quantizer float64      `json:"q" swaggertype:"number" jsonschema:"type=number"`
	Size      uint64       `json:"size_kb" format:"uint64"` // kbytes
	Time      float64      `json:"time" swaggertype:"number" jsonschema:"type=number"`
	Bitrate   float64      `json:"bitrate_kbit" swaggertype:"number" jsonschema:"type=number"` // kbit/s
	Speed     float64      `json:"speed" swaggertype:"number" jsonschema:"type=number"`
	Drop      uint64       `json:"drop" format:"uint64"`
	Dup       uint64       `json:"dup" format:"uint64"`
}
