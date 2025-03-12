package app

import "github.com/datarhei/core/v16/ffmpeg/parse"

type ProgressIOFramerate struct {
	Min     float64
	Max     float64
	Average float64
}

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
	Profile   int
	Level     int
	Frame     uint64 // counter
	Keyframe  uint64 // counter
	Framerate ProgressIOFramerate
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
	Samplefmt string
	Sampling  uint64
	Layout    string
	Channels  uint64

	// avstream
	AVstream *AVstream
}

func (p *ProgressIO) UnmarshalParser(pp *parse.ProgressIO) {
	p.Address = pp.Address
	p.Index = pp.Index
	p.Stream = pp.Stream
	p.Format = pp.Format
	p.Type = pp.Type
	p.Codec = pp.Codec
	p.Coder = pp.Coder
	p.Profile = pp.Profile
	p.Level = pp.Level
	p.Frame = pp.Frame
	p.Keyframe = pp.Keyframe
	p.Framerate = pp.Framerate
	p.FPS = pp.FPS
	p.Packet = pp.Packet
	p.PPS = pp.PPS
	p.Size = pp.Size
	p.Bitrate = pp.Bitrate
	p.Extradata = pp.Extradata
	p.Pixfmt = pp.Pixfmt
	p.Quantizer = pp.Quantizer
	p.Width = pp.Width
	p.Height = pp.Height
	p.Samplefmt = pp.Samplefmt
	p.Sampling = pp.Sampling
	p.Layout = pp.Layout
	p.Channels = pp.Channels

	if pp.AVstream != nil {
		p.AVstream = &AVstream{}
		p.AVstream.UnmarshalParser(pp.AVstream)
	} else {
		p.AVstream = nil
	}
}

func (p *ProgressIO) MarshalParser() parse.ProgressIO {
	pp := parse.ProgressIO{
		Address:   p.Address,
		Index:     p.Index,
		Stream:    p.Stream,
		Format:    p.Format,
		Type:      p.Type,
		Codec:     p.Codec,
		Coder:     p.Coder,
		Profile:   p.Profile,
		Level:     p.Level,
		Frame:     p.Frame,
		Keyframe:  p.Keyframe,
		Framerate: p.Framerate,
		FPS:       p.FPS,
		Packet:    p.Packet,
		PPS:       p.PPS,
		Size:      p.Size,
		Bitrate:   p.Bitrate,
		Extradata: p.Extradata,
		Pixfmt:    p.Pixfmt,
		Quantizer: p.Quantizer,
		Width:     p.Width,
		Height:    p.Height,
		Samplefmt: p.Samplefmt,
		Sampling:  p.Sampling,
		Layout:    p.Layout,
		Channels:  p.Channels,
	}

	if p.AVstream != nil {
		pp.AVstream = p.AVstream.MarshalParser()
	}

	return pp
}

type Progress struct {
	Started   bool
	Input     []ProgressIO
	Output    []ProgressIO
	Mapping   StreamMapping
	Frame     uint64  // counter
	Packet    uint64  // counter
	FPS       float64 // rate, frames per second
	PPS       float64 // rate, packets per second
	Quantizer float64 // gauge
	Size      uint64  // bytes
	Time      float64 // seconds with fractions
	Bitrate   float64 // bit/s
	Speed     float64 // gauge
	Drop      uint64  // counter
	Dup       uint64  // counter
}

func (p *Progress) UnmarshalParser(pp *parse.Progress) {
	p.Started = pp.Started
	p.Frame = pp.Frame
	p.Packet = pp.Packet
	p.FPS = pp.FPS
	p.PPS = pp.PPS
	p.Quantizer = pp.Quantizer
	p.Size = pp.Size
	p.Time = pp.Time
	p.Bitrate = pp.Bitrate
	p.Speed = pp.Speed
	p.Drop = pp.Drop
	p.Dup = pp.Dup

	p.Input = make([]ProgressIO, len(pp.Input))

	for i, pinput := range pp.Input {
		p.Input[i].UnmarshalParser(&pinput)
	}

	p.Output = make([]ProgressIO, len(pp.Output))

	for i, poutput := range pp.Output {
		p.Output[i].UnmarshalParser(&poutput)
	}

	p.Mapping.UnmarshalParser(&pp.Mapping)
}

func (p *Progress) MarshalParser() parse.Progress {
	pp := parse.Progress{
		Started:   p.Started,
		Input:     []parse.ProgressIO{},
		Output:    []parse.ProgressIO{},
		Mapping:   p.Mapping.MarshalParser(),
		Frame:     p.Frame,
		Packet:    p.Packet,
		FPS:       p.FPS,
		PPS:       p.PPS,
		Quantizer: p.Quantizer,
		Size:      p.Size,
		Time:      p.Time,
		Bitrate:   p.Bitrate,
		Speed:     p.Speed,
		Drop:      p.Drop,
		Dup:       p.Dup,
	}

	pp.Input = make([]parse.ProgressIO, len(p.Input))

	for i, pinput := range p.Input {
		pp.Input[i] = pinput.MarshalParser()
	}

	pp.Output = make([]parse.ProgressIO, len(p.Output))

	for i, poutput := range p.Output {
		pp.Output[i] = poutput.MarshalParser()
	}

	return pp
}

type GraphElement struct {
	Index     int
	ID        string
	Name      string
	Filter    string
	DstID     string
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

func (g *GraphElement) UnmarshalParser(p *parse.GraphElement) {
	g.Index = p.Index
	g.ID = p.ID
	g.Name = p.Name
	g.Filter = p.Filter
	g.DstID = p.DstID
	g.DstName = p.DstName
	g.DstFilter = p.DstFilter
	g.Inpad = p.Inpad
	g.Outpad = p.Outpad
	g.Timebase = p.Timebase
	g.Type = p.Type
	g.Format = p.Format
	g.Sampling = p.Sampling
	g.Layout = p.Layout
	g.Width = p.Width
	g.Height = p.Height
}

func (g *GraphElement) MarshalParser() parse.GraphElement {
	p := parse.GraphElement{
		Index:     g.Index,
		ID:        g.ID,
		Name:      g.Name,
		Filter:    g.Filter,
		DstID:     g.DstID,
		DstName:   g.DstName,
		DstFilter: g.DstFilter,
		Inpad:     g.Inpad,
		Outpad:    g.Outpad,
		Timebase:  g.Timebase,
		Type:      g.Type,
		Format:    g.Format,
		Sampling:  g.Sampling,
		Layout:    g.Layout,
		Width:     g.Width,
		Height:    g.Height,
	}

	return p
}

type GraphMapping struct {
	Input  int    // Index of input stream, negative if output element
	Output int    // Index of output stream, negative if input element
	Index  int    // Index of the graph, negative if streamcopy
	ID     string // ID of the source resp. destination, empty if streamcopy, the name can be ambigous
	Name   string // Name of the source resp. destination, empty if streamcopy
	Copy   bool   // Whether it's a streamcopy i.e. there's no graph
}

func (g *GraphMapping) UnmarshalParser(p *parse.GraphMapping) {
	g.Input = p.Input
	g.Output = p.Output
	g.Index = p.Index
	g.ID = p.ID
	g.Name = p.Name
	g.Copy = p.Copy
}

func (g *GraphMapping) MarshalParser() parse.GraphMapping {
	p := parse.GraphMapping{
		Input:  g.Input,
		Output: g.Output,
		Index:  g.Index,
		ID:     g.ID,
		Name:   g.Name,
		Copy:   g.Copy,
	}

	return p
}

type StreamMapping struct {
	Graphs  []GraphElement
	Mapping []GraphMapping
}

func (s *StreamMapping) UnmarshalParser(p *parse.StreamMapping) {
	s.Graphs = make([]GraphElement, len(p.Graphs))

	for i, graph := range p.Graphs {
		s.Graphs[i].UnmarshalParser(&graph)
	}

	s.Mapping = make([]GraphMapping, len(p.Mapping))

	for i, mapping := range p.Mapping {
		s.Mapping[i].UnmarshalParser(&mapping)
	}
}

func (s *StreamMapping) MarshalParser() parse.StreamMapping {
	p := parse.StreamMapping{
		Graphs:  make([]parse.GraphElement, len(s.Graphs)),
		Mapping: make([]parse.GraphMapping, len(s.Mapping)),
	}

	for i, graph := range s.Graphs {
		p.Graphs[i] = graph.MarshalParser()
	}

	for i, mapping := range s.Mapping {
		p.Mapping[i] = mapping.MarshalParser()
	}

	return p
}
