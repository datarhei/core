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
	Packet uint64 `json:"packet"`
	Time   uint64 `json:"time"`
	Size   uint64 `json:"size_kb"`
}

func (avio *ffmpegAVstreamIO) export() AVstreamIO {
	return AVstreamIO{
		State:  avio.State,
		Packet: avio.Packet,
		Time:   avio.Time,
		Size:   avio.Size,
	}
}

type ffmpegAVstream struct {
	Input       ffmpegAVstreamIO `json:"input"`
	Output      ffmpegAVstreamIO `json:"output"`
	Address     string           `json:"id"`
	URL         string           `json:"url"`
	Stream      uint64           `json:"stream"`
	Aqueue      uint64           `json:"aqueue"`
	Queue       uint64           `json:"queue"`
	Dup         uint64           `json:"dup"`
	Drop        uint64           `json:"drop"`
	Enc         uint64           `json:"enc"`
	Looping     bool             `json:"looping"`
	Duplicating bool             `json:"duplicating"`
	GOP         string           `json:"gop"`
}

func (av *ffmpegAVstream) export() *AVstream {
	return &AVstream{
		Aqueue:      av.Aqueue,
		Queue:       av.Queue,
		Drop:        av.Drop,
		Dup:         av.Dup,
		Enc:         av.Enc,
		Looping:     av.Looping,
		Duplicating: av.Duplicating,
		GOP:         av.GOP,
		Input:       av.Input.export(),
		Output:      av.Output.export(),
	}
}

type ffmpegProgressIO struct {
	// common
	Index   uint64  `json:"index"`
	Stream  uint64  `json:"stream"`
	Size    uint64  `json:"size_kb"` // kbytes
	Bitrate float64 `json:"-"`       // kbit/s
	Frame   uint64  `json:"frame"`
	Packet  uint64  `json:"packet"`
	FPS     float64 `json:"-"`
	PPS     float64 `json:"-"`

	// video
	Quantizer float64 `json:"q"`
}

func (io *ffmpegProgressIO) exportTo(progress *ProgressIO) {
	progress.Index = io.Index
	progress.Stream = io.Stream
	progress.Frame = io.Frame
	progress.Packet = io.Packet
	progress.FPS = io.FPS
	progress.PPS = io.PPS
	progress.Quantizer = io.Quantizer
	progress.Size = io.Size * 1024
	progress.Bitrate = io.Bitrate * 1024
}

type ffmpegProgress struct {
	Input     []ffmpegProgressIO `json:"inputs"`
	Output    []ffmpegProgressIO `json:"outputs"`
	Frame     uint64             `json:"frame"`
	Packet    uint64             `json:"packet"`
	FPS       float64            `json:"-"`
	PPS       float64            `json:"-"`
	Quantizer float64            `json:"q"`
	Size      uint64             `json:"size_kb"` // kbytes
	Bitrate   float64            `json:"-"`       // kbit/s
	Time      Duration           `json:"time"`
	Speed     float64            `json:"speed"`
	Drop      uint64             `json:"drop"`
	Dup       uint64             `json:"dup"`
}

func (p *ffmpegProgress) exportTo(progress *Progress) {
	progress.Frame = p.Frame
	progress.Packet = p.Packet
	progress.FPS = p.FPS
	progress.PPS = p.PPS
	progress.Quantizer = p.Quantizer
	progress.Size = p.Size * 1024
	progress.Time = p.Time.Seconds()
	progress.Bitrate = p.Bitrate * 1024
	progress.Speed = p.Speed
	progress.Drop = p.Drop
	progress.Dup = p.Dup

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

type ffmpegProcess struct {
	input  []ffmpegProcessIO
	output []ffmpegProcessIO
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

	return progress
}

type ProgressIO struct {
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

type AVstreamIO struct {
	State  string
	Packet uint64
	Time   uint64
	Size   uint64
}

type AVstream struct {
	Input       AVstreamIO
	Output      AVstreamIO
	Aqueue      uint64
	Queue       uint64
	Dup         uint64
	Drop        uint64
	Enc         uint64
	Looping     bool
	Duplicating bool
	GOP         string
}
