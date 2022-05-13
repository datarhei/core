package app

type ProgressIO struct {
	ID      string
	Address string

	// General
	Index   uint64
	Stream  uint64
	Format  string
	Type    string
	Codec   string
	Coder   string
	Frame   uint64
	FPS     float64
	Packet  uint64
	PPS     float64
	Size    uint64  // bytes
	Bitrate float64 // bit/s

	// Video
	Pixfmt    string
	Quantizer float64
	Width     uint64
	Height    uint64

	// Audio
	Sampling uint64
	Layout   string
	Channels uint64

	// avstream
	AVstream *AVstream
}

type Progress struct {
	Input     []ProgressIO
	Output    []ProgressIO
	Frame     uint64
	Packet    uint64
	FPS       float64
	PPS       float64
	Quantizer float64
	Size      uint64 // bytes
	Time      float64
	Bitrate   float64 // bit/s
	Speed     float64
	Drop      uint64
	Dup       uint64
}
