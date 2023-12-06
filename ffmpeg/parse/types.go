package parse

import (
	"encoding/json"
	"errors"
	"time"
)

// Duration represents a time.Duration
type Duration struct {
	time.Duration
}

// MarshalJSON marshals a time.Duration to JSON
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.Seconds())
}

// UnmarshalJSON unmarshals a JSON value to time.Duration. The JSON value
// can be either a float which is interpreted as seconds or a string
// that is interpreted as a formatted duration.
func (d *Duration) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch value := v.(type) {
	case float64:
		d.Duration = time.Duration(value)
		return nil
	case string:
		var err error
		d.Duration, err = time.ParseDuration(value)
		if err != nil {
			return err
		}
		return nil
	default:
		return errors.New("invalid duration")
	}
}

type ffmpegAVstreamIO struct {
	State  string `json:"state"`
	Packet uint64 `json:"packet"` // counter
	Time   uint64 `json:"time"`
	Size   uint64 `json:"size_kb"` // kbytes
}

func (avio *ffmpegAVstreamIO) export() AVstreamIO {
	return AVstreamIO{
		State:  avio.State,
		Packet: avio.Packet,
		Time:   avio.Time,
		Size:   avio.Size * 1024,
	}
}

type ffmpegAVStreamSwap struct {
	URL       string `json:"url"`
	Status    string `json:"status"`
	LastURL   string `json:"lasturl"`
	LastError string `json:"lasterror"`
}

func (avswap *ffmpegAVStreamSwap) export() AVStreamSwap {
	return AVStreamSwap{
		URL:       avswap.URL,
		Status:    avswap.Status,
		LastURL:   avswap.LastURL,
		LastError: avswap.LastError,
	}
}

type ffmpegAVstream struct {
	Input          ffmpegAVstreamIO   `json:"input"`
	Output         ffmpegAVstreamIO   `json:"output"`
	Address        string             `json:"id"`
	URL            string             `json:"url"`
	Stream         uint64             `json:"stream"`
	Aqueue         uint64             `json:"aqueue"`
	Queue          uint64             `json:"queue"`
	Dup            uint64             `json:"dup"`
	Drop           uint64             `json:"drop"`
	Enc            uint64             `json:"enc"`
	Looping        bool               `json:"looping"`
	LoopingRuntime uint64             `json:"looping_runtime"`
	Duplicating    bool               `json:"duplicating"`
	GOP            string             `json:"gop"`
	Mode           string             `json:"mode"`
	Debug          interface{}        `json:"debug"`
	Swap           ffmpegAVStreamSwap `json:"swap"`
}

func (av *ffmpegAVstream) export() *AVstream {
	return &AVstream{
		Aqueue:         av.Aqueue,
		Queue:          av.Queue,
		Drop:           av.Drop,
		Dup:            av.Dup,
		Enc:            av.Enc,
		Looping:        av.Looping,
		LoopingRuntime: av.LoopingRuntime,
		Duplicating:    av.Duplicating,
		GOP:            av.GOP,
		Mode:           av.Mode,
		Input:          av.Input.export(),
		Output:         av.Output.export(),
		Debug:          av.Debug,
		Swap:           av.Swap.export(),
	}
}

type ffmpegProgressIO struct {
	// common
	Index     uint64  `json:"index"`
	Stream    uint64  `json:"stream"`
	SizeKB    uint64  `json:"size_kb"`    // kbytes
	Size      uint64  `json:"size_bytes"` // bytes
	Bitrate   float64 `json:"-"`          // bit/s
	Frame     uint64  `json:"frame"`      // counter
	Keyframe  uint64  `json:"keyframe"`   // counter
	Framerate struct {
		Min     float64 `json:"min"`
		Max     float64 `json:"max"`
		Average float64 `json:"avg"`
	} `json:"framerate"`
	Packet    uint64  `json:"packet"`               // counter
	Extradata uint64  `json:"extradata_size_bytes"` // bytes
	FPS       float64 `json:"-"`                    // rate, frames per second
	PPS       float64 `json:"-"`                    // rate, packets per second

	// video
	Quantizer float64 `json:"q"`
}

func (io *ffmpegProgressIO) exportTo(progress *ProgressIO) {
	progress.Frame = io.Frame
	progress.Keyframe = io.Keyframe
	progress.Framerate.Min = io.Framerate.Min
	progress.Framerate.Max = io.Framerate.Max
	progress.Framerate.Average = io.Framerate.Average
	progress.Packet = io.Packet
	progress.FPS = io.FPS
	progress.PPS = io.PPS
	progress.Quantizer = io.Quantizer
	progress.Bitrate = io.Bitrate
	progress.Extradata = io.Extradata

	if io.Size == 0 {
		progress.Size = io.SizeKB * 1024
	} else {
		progress.Size = io.Size
	}
}

type ffmpegProgress struct {
	Input     []ffmpegProgressIO `json:"inputs"`
	Output    []ffmpegProgressIO `json:"outputs"`
	Frame     uint64             `json:"frame"`  // counter
	Packet    uint64             `json:"packet"` // counter
	FPS       float64            `json:"-"`      // rate, frames per second
	PPS       float64            `json:"-"`      // rate, packets per second
	Quantizer float64            `json:"q"`
	SizeKB    uint64             `json:"size_kb"`    // kbytes
	Size      uint64             `json:"size_bytes"` // bytes
	Bitrate   float64            `json:"-"`          // bit/s
	Time      Duration           `json:"time"`
	Speed     float64            `json:"speed"`
	Drop      uint64             `json:"drop"` // counter
	Dup       uint64             `json:"dup"`  // counter
}

func (p *ffmpegProgress) exportTo(progress *Progress) {
	progress.Frame = p.Frame
	progress.Packet = p.Packet
	progress.FPS = p.FPS
	progress.PPS = p.PPS
	progress.Quantizer = p.Quantizer
	progress.Time = p.Time.Seconds()
	progress.Bitrate = p.Bitrate
	progress.Speed = p.Speed
	progress.Drop = p.Drop
	progress.Dup = p.Dup

	if p.Size == 0 {
		progress.Size = p.SizeKB * 1024
	} else {
		progress.Size = p.Size
	}

	for i := range p.Input {
		if len(progress.Input) <= i {
			break
		}

		p.Input[i].exportTo(&progress.Input[i])
	}

	for i := range p.Output {
		if len(progress.Output) <= i {
			break
		}

		p.Output[i].exportTo(&progress.Output[i])
	}
}

type ffmpegProcessIO struct {
	// common
	Address string `json:"url"`
	IP      string `json:"-"`
	Format  string `json:"format"`
	Index   uint64 `json:"index"`
	Stream  uint64 `json:"stream"`
	Type    string `json:"type"`
	Codec   string `json:"codec"`
	Coder   string `json:"coder"`

	// video
	Pixfmt string `json:"pix_fmt"`
	Width  uint64 `json:"width"`
	Height uint64 `json:"height"`

	// audio
	Sampling uint64 `json:"sampling_hz"`
	Layout   string `json:"layout"`
	Channels uint64 `json:"channels"`
}

func (io *ffmpegProcessIO) export() ProgressIO {
	return ProgressIO{
		Address:  io.Address,
		Format:   io.Format,
		Index:    io.Index,
		Stream:   io.Stream,
		Type:     io.Type,
		Codec:    io.Codec,
		Coder:    io.Coder,
		Pixfmt:   io.Pixfmt,
		Width:    io.Width,
		Height:   io.Height,
		Sampling: io.Sampling,
		Layout:   io.Layout,
		Channels: io.Channels,
	}
}

type ffmpegGraphElement struct {
	SrcName   string `json:"src_name"`
	SrcFilter string `json:"src_filter"`
	DstName   string `json:"dst_name"`
	DstFilter string `json:"dst_filter"`
	Inpad     string `json:"inpad"`
	Outpad    string `json:"outpad"`
	Timebase  string `json:"timebase"`
	Type      string `json:"type"`
	Format    string `json:"format"`
	Sampling  uint64 `json:"sampling_hz"`
	Layout    string `json:"layout"`
	Width     uint64 `json:"width"`
	Height    uint64 `json:"height"`
}

type ffmpegGraph struct {
	Index int                  `json:"index"`
	Graph []ffmpegGraphElement `json:"graph"`
}

type ffmpegGraphMapping struct {
	Input  *ffmpegMappingIO `json:"input"`
	Output *ffmpegMappingIO `json:"output"`
	Graph  struct {
		Index int    `json:"index"`
		Name  string `json:"name"`
	} `json:"graph"`
	Copy bool `json:"copy"`
}

type ffmpegMappingIO struct {
	Index  uint64 `json:"index"`
	Stream uint64 `json:"stream"`
}

type ffmpegStreamMapping struct {
	Graphs  []ffmpegGraph        `json:"graphs"`
	Mapping []ffmpegGraphMapping `json:"mapping"`
}

type ffmpegProcess struct {
	input      []ffmpegProcessIO
	output     []ffmpegProcessIO
	mapping    ffmpegStreamMapping
	hlsMapping *ffmpegHLSStreamMap
}

func (f *ffmpegProcess) ExportMapping() StreamMapping {
	sm := StreamMapping{}

	for _, graph := range f.mapping.Graphs {
		for _, g := range graph.Graph {
			e := GraphElement{
				Index:     graph.Index,
				Name:      g.SrcName,
				Filter:    g.SrcFilter,
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

			sm.Graphs = append(sm.Graphs, e)
		}
	}

	for _, fm := range f.mapping.Mapping {
		m := GraphMapping{
			Input:  -1,
			Output: -1,
			Index:  fm.Graph.Index,
			Name:   fm.Graph.Name,
			Copy:   fm.Copy,
		}

		if len(m.Name) == 0 {
			m.Index = -1
		}

		if fm.Input != nil {
			for i, in := range f.input {
				if in.Index != fm.Input.Index || in.Stream != fm.Input.Stream {
					continue
				}

				m.Input = i
				break
			}
		}

		if fm.Output != nil {
			for i, out := range f.output {
				if out.Index != fm.Output.Index || out.Stream != fm.Output.Stream {
					continue
				}

				m.Output = i
				break
			}
		}

		sm.Mapping = append(sm.Mapping, m)
	}

	return sm
}

func (p *ffmpegProcess) export() Progress {
	progress := Progress{}

	for _, io := range p.input {
		aio := io.export()

		progress.Input = append(progress.Input, aio)
	}

	for _, io := range p.output {
		aio := io.export()

		progress.Output = append(progress.Output, aio)
	}

	progress.Mapping = p.ExportMapping()

	if p.hlsMapping != nil {
		for _, variant := range p.hlsMapping.Variants {
			for s, stream := range variant.Streams {
				if stream >= len(progress.Output) {
					continue
				}

				output := progress.Output[stream]

				output.Address = variant.Address
				output.Index = variant.Variant
				output.Stream = uint64(s)

				progress.Output[stream] = output
			}
		}
	}

	return progress
}

type ffmpegHLSStreamMap struct {
	Address  string             `json:"address"`
	Variants []ffmpegHLSVariant `json:"variants"`
}

type ffmpegHLSVariant struct {
	Variant uint64 `json:"variant"`
	Address string `json:"address"`
	Streams []int  `json:"streams"`
}

type ProgressIO struct {
	Address string

	// General
	Index     uint64
	Stream    uint64
	Format    string
	Type      string
	Codec     string
	Coder     string
	Frame     uint64
	Keyframe  uint64
	Framerate struct {
		Min     float64
		Max     float64
		Average float64
	}
	FPS       float64
	Packet    uint64
	PPS       float64
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
	Input     []ProgressIO
	Output    []ProgressIO
	Mapping   StreamMapping
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

type AVstreamIO struct {
	State  string
	Packet uint64
	Time   uint64
	Size   uint64
}

type AVStreamSwap struct {
	URL       string
	Status    string
	LastURL   string
	LastError string
}

type AVstream struct {
	Input          AVstreamIO
	Output         AVstreamIO
	Aqueue         uint64
	Queue          uint64
	Dup            uint64
	Drop           uint64
	Enc            uint64
	Looping        bool
	LoopingRuntime uint64
	Duplicating    bool
	GOP            string
	Mode           string
	Debug          interface{}
	Swap           AVStreamSwap
}

type Usage struct {
	CPU struct {
		NCPU    float64
		Average float64
		Max     float64
		Limit   float64
	}
	Memory struct {
		Average float64
		Max     uint64
		Limit   uint64
	}
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
	Input  int
	Output int
	Index  int
	Name   string
	Copy   bool
}

type StreamMapping struct {
	Graphs  []GraphElement
	Mapping []GraphMapping
}
