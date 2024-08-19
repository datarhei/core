package api

import (
	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/restream/app"
)

type ProgressIOFramerate struct {
	Min     json.Number `json:"min" swaggertype:"number" jsonschema:"type=number"`
	Max     json.Number `json:"max" swaggertype:"number" jsonschema:"type=number"`
	Average json.Number `json:"avg" swaggertype:"number" jsonschema:"type=number"`
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
	FPS       json.Number         `json:"fps" swaggertype:"number" jsonschema:"type=number"`
	Packet    uint64              `json:"packet" format:"uint64"`
	PPS       json.Number         `json:"pps" swaggertype:"number" jsonschema:"type=number"`
	Size      uint64              `json:"size_kb" format:"uint64"`                                    // kbytes
	Bitrate   json.Number         `json:"bitrate_kbit" swaggertype:"number" jsonschema:"type=number"` // kbit/s
	Extradata uint64              `json:"extradata_size_bytes" format:"uint64"`                       // bytes

	// Video
	Pixfmt    string      `json:"pix_fmt,omitempty"`
	Quantizer json.Number `json:"q,omitempty" swaggertype:"number" jsonschema:"type=number"`
	Width     uint64      `json:"width,omitempty" format:"uint64"`
	Height    uint64      `json:"height,omitempty" format:"uint64"`

	// Audio
	Sampling uint64 `json:"sampling_hz,omitempty" format:"uint64"`
	Layout   string `json:"layout,omitempty"`
	Channels uint64 `json:"channels,omitempty" format:"uint64"`

	// avstream
	AVstream *AVstream `json:"avstream" jsonschema:"anyof_type=null;object"`
}

// Unmarshal converts a core ProgressIO to a ProgressIO in API representation
func (i *ProgressIO) Unmarshal(io *app.ProgressIO) {
	if io == nil {
		return
	}

	i.ID = io.ID
	i.Address = io.Address
	i.Index = io.Index
	i.Stream = io.Stream
	i.Format = io.Format
	i.Type = io.Type
	i.Codec = io.Codec
	i.Coder = io.Coder
	i.Frame = io.Frame
	i.Keyframe = io.Keyframe
	i.Framerate.Min = json.ToNumber(io.Framerate.Min)
	i.Framerate.Max = json.ToNumber(io.Framerate.Max)
	i.Framerate.Average = json.ToNumber(io.Framerate.Average)
	i.FPS = json.ToNumber(io.FPS)
	i.Packet = io.Packet
	i.PPS = json.ToNumber(io.PPS)
	i.Size = io.Size / 1024
	i.Bitrate = json.ToNumber(io.Bitrate / 1024)
	i.Extradata = io.Extradata
	i.Pixfmt = io.Pixfmt
	i.Quantizer = json.ToNumber(io.Quantizer)
	i.Width = io.Width
	i.Height = io.Height
	i.Sampling = io.Sampling
	i.Layout = io.Layout
	i.Channels = io.Channels

	if io.AVstream != nil {
		i.AVstream = &AVstream{}
		i.AVstream.Unmarshal(io.AVstream)
	}
}

func (i *ProgressIO) Marshal() app.ProgressIO {
	p := app.ProgressIO{
		ID:        i.ID,
		Address:   i.Address,
		Index:     i.Index,
		Stream:    i.Stream,
		Format:    i.Format,
		Type:      i.Type,
		Codec:     i.Codec,
		Coder:     i.Coder,
		Frame:     i.Frame,
		Keyframe:  i.Keyframe,
		Packet:    i.Packet,
		Size:      i.Size * 1024,
		Extradata: i.Extradata,
		Pixfmt:    i.Pixfmt,
		Width:     i.Width,
		Height:    i.Height,
		Sampling:  i.Sampling,
		Layout:    i.Layout,
		Channels:  i.Channels,
	}

	if x, err := i.Framerate.Min.Float64(); err == nil {
		p.Framerate.Min = x
	}

	if x, err := i.Framerate.Max.Float64(); err == nil {
		p.Framerate.Max = x
	}

	if x, err := i.Framerate.Average.Float64(); err == nil {
		p.Framerate.Average = x
	}

	if x, err := i.FPS.Float64(); err == nil {
		p.FPS = x
	}

	if x, err := i.PPS.Float64(); err == nil {
		p.PPS = x
	}

	if x, err := i.Bitrate.Float64(); err == nil {
		p.Bitrate = x * 1024
	}

	if x, err := i.Quantizer.Float64(); err == nil {
		p.Quantizer = x
	}

	if i.AVstream != nil {
		p.AVstream = i.AVstream.Marshal()
	}

	return p
}

// Progress represents the progress of an ffmpeg process
type Progress struct {
	Started   bool          `json:"started"`
	Input     []ProgressIO  `json:"inputs"`
	Output    []ProgressIO  `json:"outputs"`
	Mapping   StreamMapping `json:"mapping"`
	Frame     uint64        `json:"frame" format:"uint64"`
	Packet    uint64        `json:"packet" format:"uint64"`
	FPS       json.Number   `json:"fps" swaggertype:"number" jsonschema:"type=number"`
	Quantizer json.Number   `json:"q" swaggertype:"number" jsonschema:"type=number"`
	Size      uint64        `json:"size_kb" format:"uint64"` // kbytes
	Time      json.Number   `json:"time" swaggertype:"number" jsonschema:"type=number"`
	Bitrate   json.Number   `json:"bitrate_kbit" swaggertype:"number" jsonschema:"type=number"` // kbit/s
	Speed     json.Number   `json:"speed" swaggertype:"number" jsonschema:"type=number"`
	Drop      uint64        `json:"drop" format:"uint64"`
	Dup       uint64        `json:"dup" format:"uint64"`
}

// Unmarshal converts a core Progress to a Progress in API representation
func (p *Progress) Unmarshal(pp *app.Progress) {
	p.Input = []ProgressIO{}
	p.Output = []ProgressIO{}

	if pp == nil {
		return
	}

	p.Started = pp.Started
	p.Input = make([]ProgressIO, len(pp.Input))
	p.Output = make([]ProgressIO, len(pp.Output))
	p.Frame = pp.Frame
	p.Packet = pp.Packet
	p.FPS = json.ToNumber(pp.FPS)
	p.Quantizer = json.ToNumber(pp.Quantizer)
	p.Size = pp.Size / 1024
	p.Time = json.ToNumber(pp.Time)
	p.Bitrate = json.ToNumber(pp.Bitrate / 1024)
	p.Speed = json.ToNumber(pp.Speed)
	p.Drop = pp.Drop
	p.Dup = pp.Dup

	for i, io := range pp.Input {
		p.Input[i].Unmarshal(&io)
	}

	for i, io := range pp.Output {
		p.Output[i].Unmarshal(&io)
	}

	p.Mapping.Unmarshal(&pp.Mapping)
}

func (p *Progress) Marshal() app.Progress {
	pp := app.Progress{
		Started: p.Started,
		Input:   make([]app.ProgressIO, 0, len(p.Input)),
		Output:  make([]app.ProgressIO, 0, len(p.Output)),
		Mapping: p.Mapping.Marshal(),
		Frame:   p.Frame,
		Packet:  p.Packet,
		Size:    p.Size * 1024,
		Drop:    p.Drop,
		Dup:     p.Dup,
	}

	if x, err := p.FPS.Float64(); err == nil {
		pp.FPS = x
	}

	if x, err := p.Quantizer.Float64(); err == nil {
		pp.Quantizer = x
	}

	if x, err := p.Time.Float64(); err == nil {
		pp.Time = x
	}

	if x, err := p.Bitrate.Float64(); err == nil {
		pp.Bitrate = x * 1024
	}

	if x, err := p.Speed.Float64(); err == nil {
		pp.Speed = x
	}

	for _, io := range p.Input {
		pp.Input = append(pp.Input, io.Marshal())
	}

	for _, io := range p.Output {
		pp.Output = append(pp.Output, io.Marshal())
	}

	return pp
}

type GraphElement struct {
	Index     int    `json:"index"`
	Name      string `json:"name"`
	Filter    string `json:"filter"`
	DstName   string `json:"dst_name"`
	DstFilter string `json:"dst_filter"`
	Inpad     string `json:"inpad"`
	Outpad    string `json:"outpad"`
	Timebase  string `json:"timebase"`
	Type      string `json:"type"` // audio or video
	Format    string `json:"format"`
	Sampling  uint64 `json:"sampling"` // Hz
	Layout    string `json:"layout"`
	Width     uint64 `json:"width"`
	Height    uint64 `json:"height"`
}

func (g *GraphElement) Unmarshal(a *app.GraphElement) {
	g.Index = a.Index
	g.Name = a.Name
	g.Filter = a.Filter
	g.DstName = a.DstName
	g.DstFilter = a.DstFilter
	g.Inpad = a.Inpad
	g.Outpad = a.Outpad
	g.Timebase = a.Timebase
	g.Type = a.Type
	g.Format = a.Format
	g.Sampling = a.Sampling
	g.Layout = a.Layout
	g.Width = a.Width
	g.Height = a.Height
}

func (g *GraphElement) Marshal() app.GraphElement {
	a := app.GraphElement{
		Index:     g.Index,
		Name:      g.Name,
		Filter:    g.Filter,
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

	return a
}

type GraphMapping struct {
	Input  int    `json:"input"`
	Output int    `json:"output"`
	Index  int    `json:"index"`
	Name   string `json:"name"`
	Copy   bool   `json:"copy"`
}

func (g *GraphMapping) Unmarshal(a *app.GraphMapping) {
	g.Input = a.Input
	g.Output = a.Output
	g.Index = a.Index
	g.Name = a.Name
	g.Copy = a.Copy
}

func (g *GraphMapping) Marshal() app.GraphMapping {
	a := app.GraphMapping{
		Input:  g.Input,
		Output: g.Output,
		Index:  g.Index,
		Name:   g.Name,
		Copy:   g.Copy,
	}

	return a
}

type StreamMapping struct {
	Graphs  []GraphElement `json:"graphs"`
	Mapping []GraphMapping `json:"mapping"`
}

// Unmarshal converts a core StreamMapping to a StreamMapping in API representation
func (s *StreamMapping) Unmarshal(m *app.StreamMapping) {
	s.Graphs = make([]GraphElement, len(m.Graphs))
	for i, graph := range m.Graphs {
		s.Graphs[i].Unmarshal(&graph)
	}

	s.Mapping = make([]GraphMapping, len(m.Mapping))
	for i, mapping := range m.Mapping {
		s.Mapping[i].Unmarshal(&mapping)
	}
}

func (s *StreamMapping) Marshal() app.StreamMapping {
	m := app.StreamMapping{
		Graphs:  make([]app.GraphElement, 0, len(s.Graphs)),
		Mapping: make([]app.GraphMapping, 0, len(s.Mapping)),
	}

	for _, graph := range s.Graphs {
		m.Graphs = append(m.Graphs, graph.Marshal())
	}

	for _, mapping := range s.Mapping {
		m.Mapping = append(m.Mapping, mapping.Marshal())
	}

	return m
}
