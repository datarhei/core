package parse

import (
	"errors"
	"time"

	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/process"
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

type ffmpegAVStreamTrack struct {
	Queue     uint64           `json:"queue"`
	Dup       uint64           `json:"dup"`
	Drop      uint64           `json:"drop"`
	Enc       uint64           `json:"enc"`
	Input     ffmpegAVstreamIO `json:"input"`
	Output    ffmpegAVstreamIO `json:"output"`
	Codec     string           `json:"codec"`
	Profile   int              `json:"profile"`
	Level     int              `json:"level"`
	Pixfmt    string           `json:"pix_fmt"`
	Width     uint64           `json:"width"`
	Height    uint64           `json:"height"`
	Samplefmt string           `json:"sample_fmt"`
	Sampling  uint64           `json:"sampling_hz"`
	Layout    string           `json:"layout"`
	Channels  uint64           `json:"channels"`
}

type ffmpegAVstream struct {
	Input          ffmpegAVstreamIO    `json:"input"`
	Output         ffmpegAVstreamIO    `json:"output"`
	Audio          ffmpegAVStreamTrack `json:"audio"`
	Video          ffmpegAVStreamTrack `json:"video"`
	Address        string              `json:"id"`
	URL            string              `json:"url"`
	Stream         uint64              `json:"stream"`
	Aqueue         uint64              `json:"aqueue"`
	Queue          uint64              `json:"queue"`
	Dup            uint64              `json:"dup"`
	Drop           uint64              `json:"drop"`
	Enc            uint64              `json:"enc"`
	Looping        bool                `json:"looping"`
	LoopingRuntime uint64              `json:"looping_runtime"`
	Duplicating    bool                `json:"duplicating"`
	GOP            string              `json:"gop"`
	Mode           string              `json:"mode"`
	Debug          interface{}         `json:"debug"`
	Swap           ffmpegAVStreamSwap  `json:"swap"`
}

func (av *ffmpegAVstream) export(trackType string) *AVstream {
	avs := &AVstream{
		Looping:        av.Looping,
		LoopingRuntime: av.LoopingRuntime,
		Duplicating:    av.Duplicating,
		GOP:            av.GOP,
		Mode:           av.Mode,
		Debug:          av.Debug,
		Swap:           av.Swap.export(),
	}

	hasTracks := len(av.Video.Codec) != 0

	if hasTracks {
		var track *ffmpegAVStreamTrack = nil

		if trackType == "audio" {
			track = &av.Audio
		} else {
			track = &av.Video
		}

		avs.Queue = track.Queue
		avs.Drop = track.Drop
		avs.Dup = track.Dup
		avs.Enc = track.Enc
		avs.Input = track.Input.export()
		avs.Output = track.Output.export()

		avs.Codec = track.Codec
		avs.Profile = track.Profile
		avs.Level = track.Level
		avs.Pixfmt = track.Pixfmt
		avs.Width = track.Width
		avs.Height = track.Height
		avs.Samplefmt = track.Samplefmt
		avs.Sampling = track.Sampling
		avs.Layout = track.Layout
		avs.Channels = track.Channels
	} else {
		avs.Queue = av.Queue
		avs.Aqueue = av.Aqueue
		avs.Drop = av.Drop
		avs.Dup = av.Dup
		avs.Enc = av.Enc
		avs.Input = av.Input.export()
		avs.Output = av.Output.export()
	}

	return avs
}

type ffmpegProgressIOTee struct {
	State                string `json:"state"`
	FifoRecoveryAttempts uint64 `json:"fifo_recovery_attempts_total"`
	FifoState            string `json:"fifo_state"`
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

	// format specific
	Tee []ffmpegProgressIOTee `json:"tee"`
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

	if len(progress.Tee) == len(io.Tee) {
		for i, t := range io.Tee {
			tee := progress.Tee[i]
			tee.State = t.State
			tee.FifoRecoveryAttempts = t.FifoRecoveryAttempts
			tee.FifoState = t.FifoState
			progress.Tee[i] = tee
		}
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

type ffmpegTee struct {
	ID      string `json:"id"`
	Address string `json:"address"`
	Format  string `json:"format"`
	Fifo    bool   `json:"fifo_enabled"`
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
	Profile int    `json:"profile"`
	Level   int    `json:"level"`

	// video
	Pixfmt string `json:"pix_fmt"`
	Width  uint64 `json:"width"`
	Height uint64 `json:"height"`

	// audio
	Samplefmt string `json:"sample_fmt"`
	Sampling  uint64 `json:"sampling_hz"`
	Layout    string `json:"layout"`
	Channels  uint64 `json:"channels"`

	// format specific
	Tee []ffmpegTee `json:"tee"`
}

func (io *ffmpegProcessIO) export() ProgressIO {
	pio := ProgressIO{
		URL:       io.Address,
		Address:   io.Address,
		Format:    io.Format,
		Index:     io.Index,
		Stream:    io.Stream,
		Type:      io.Type,
		Codec:     io.Codec,
		Coder:     io.Coder,
		Profile:   io.Profile,
		Level:     io.Level,
		Pixfmt:    io.Pixfmt,
		Width:     io.Width,
		Height:    io.Height,
		Samplefmt: io.Samplefmt,
		Sampling:  io.Sampling,
		Layout:    io.Layout,
		Channels:  io.Channels,
	}

	for _, t := range io.Tee {
		pio.Tee = append(pio.Tee, ProgressIOTee{
			ID:      t.ID,
			Address: t.Address,
			Format:  t.Format,
			Fifo:    t.Fifo,
		})
	}

	return pio
}

type ffmpegGraphElement struct {
	SrcID     string `json:"src_id"`
	SrcName   string `json:"src_name"`
	SrcFilter string `json:"src_filter"`
	DstID     string `json:"dst_id"`
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
		ID    string `json:"id"`
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
	hlsMapping []ffmpegHLSStreamMap
}

func (f *ffmpegProcess) ExportMapping() StreamMapping {
	sm := StreamMapping{}

	for _, graph := range f.mapping.Graphs {
		for _, g := range graph.Graph {
			e := GraphElement{
				Index:     graph.Index,
				ID:        g.SrcID,
				Name:      g.SrcName,
				Filter:    g.SrcFilter,
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

			sm.Graphs = append(sm.Graphs, e)
		}
	}

	for _, fm := range f.mapping.Mapping {
		m := GraphMapping{
			Input:  -1,
			Output: -1,
			Index:  fm.Graph.Index,
			ID:     fm.Graph.ID,
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

	for _, hlsmapping := range p.hlsMapping {
		progress.Output = applyHLSMapping(progress.Output, hlsmapping)
	}

	return progress
}

func applyHLSMapping(output []ProgressIO, hlsMapping ffmpegHLSStreamMap) []ProgressIO {
	minVariantIndex := uint64(len(output))
	maxVariantIndex := uint64(0)

	pivot := -1

	// Find all outputs matching the address
	for i, io := range output {
		if io.Address != hlsMapping.Address {
			continue
		}

		pivot = i

	bla:
		for _, variant := range hlsMapping.Variants {
			for s, stream := range variant.Streams {
				if io.Stream != uint64(stream) {
					continue
				}

				if io.Index < minVariantIndex {
					minVariantIndex = io.Index
				}

				io.Address = variant.Address
				io.Index = io.Index + variant.Variant
				io.Stream = uint64(s)

				if io.Index > maxVariantIndex {
					maxVariantIndex = io.Index
				}

				break bla
			}
		}

		output[i] = io
	}

	offset := maxVariantIndex - minVariantIndex

	if offset > 0 {
		pivot++

		// Fix all following index values
		for i, io := range output[pivot:] {
			io.Index += offset

			output[pivot+i] = io
		}
	}

	return output
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

type ProgressIOTee struct {
	ID                   string
	Address              string
	Format               string
	State                string
	Fifo                 bool
	FifoRecoveryAttempts uint64
	FifoState            string
}

type ProgressIO struct {
	URL     string
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
	Samplefmt string
	Sampling  uint64 // Hz
	Layout    string // mono, stereo, ...
	Channels  uint64

	// avstream
	AVstream *AVstream

	// Format specific
	Tee []ProgressIOTee
}

type Progress struct {
	Started   bool
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
	Codec          string
	Profile        int
	Level          int
	Pixfmt         string
	Width          uint64
	Height         uint64
	Samplefmt      string
	Sampling       uint64
	Layout         string
	Channels       uint64
}

type Usage struct {
	CPU    UsageCPU
	Memory UsageMemory
	GPU    UsageGPU
}

type UsageCPU struct {
	NCPU    float64
	Average float64
	Max     float64
	Limit   float64
}

type UsageMemory struct {
	Average uint64
	Max     uint64
	Limit   uint64
}

type UsageGPU struct {
	Index   int
	Usage   UsageGPUUsage
	Encoder UsageGPUUsage
	Decoder UsageGPUUsage
	Memory  UsageGPUMemory
}

type UsageGPUUsage struct {
	Average float64
	Max     float64
	Limit   float64
}

type UsageGPUMemory struct {
	Average uint64
	Max     uint64
	Limit   uint64
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

type GraphMapping struct {
	Input  int
	Output int
	Index  int
	ID     string
	Name   string
	Copy   bool
}

type StreamMapping struct {
	Graphs  []GraphElement
	Mapping []GraphMapping
}

// Report represents a log report, including the prelude and the last log lines of the process.
type Report struct {
	CreatedAt time.Time
	Prelude   []string
	Log       []process.Line
	Matches   []string
}

// ReportHistoryEntry represents an historical log report, including the exit status of the
// process and the last progress data.
type ReportHistoryEntry struct {
	Report

	ExitedAt  time.Time
	ExitState string
	Progress  Progress
	Usage     Usage
}

type ReportHistorySearchResult struct {
	CreatedAt time.Time
	ExitedAt  time.Time
	ExitState string
}
