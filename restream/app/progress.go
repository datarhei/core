package app

type ProgressIO struct {
	ID      string
	Address string

	// General
	Index     uint64
	Stream    uint64
	Format    string
	Type      string
	Codec     string
	Coder     string
	Frame     uint64 // counter
	Keyframe  uint64 // counter
	Framerate struct {
		Min     float64
		Max     float64
		Average float64
	}
	FPS       float64 // rate, frames per second
	Packet    uint64  // counter
	PPS       float64 // rate, packets per second
	Size      uint64  // bytes
	Bitrate   float64 // bit/s
	Extradata uint64  // bytes

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
	Initialized bool
	Input       []ProgressIO
	Output      []ProgressIO
	Mapping     StreamMapping
	Frame       uint64  // counter
	Packet      uint64  // counter
	FPS         float64 // rate, frames per second
	PPS         float64 // rate, packets per second
	Quantizer   float64 // gauge
	Size        uint64  // bytes
	Time        float64 // seconds with fractions
	Bitrate     float64 // bit/s
	Speed       float64 // gauge
	Drop        uint64  // counter
	Dup         uint64  // counter
}

type GraphElement struct {
	Index     int
	Name      string
	Filter    string
	DstName   string
	DstFilter string
	Inpad     string
	Outpad    string
	Timebase  string
	Type      string // audio or video
	Format    string
	Sampling  uint64 // Hz
	Layout    string
	Width     uint64
	Height    uint64
}

type GraphMapping struct {
	Input  int    // Index of input stream, negative if output element
	Output int    // Index of output stream, negative if input element
	Index  int    // Index of the graph, negative if streamcopy
	Name   string // Name of the source resp. destination, empty if streamcopy
	Copy   bool   // Whether it's a streamcopy i.e. there's no graph
}

type StreamMapping struct {
	Graphs  []GraphElement
	Mapping []GraphMapping
}
