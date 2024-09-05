package app

import "github.com/datarhei/core/v16/ffmpeg/parse"

type AVstreamIO struct {
	State  string
	Packet uint64 // counter
	Time   uint64 // sec
	Size   uint64 // bytes
}

func (a *AVstreamIO) UnmarshalParser(p *parse.AVstreamIO) {
	a.State = p.State
	a.Packet = p.Packet
	a.Time = p.Time
	a.Size = p.Size
}

func (a *AVstreamIO) MarshalParser() parse.AVstreamIO {
	p := parse.AVstreamIO{
		State:  a.State,
		Packet: a.Packet,
		Time:   a.Time,
		Size:   a.Size,
	}

	return p
}

type AVStreamSwap struct {
	URL       string
	Status    string
	LastURL   string
	LastError string
}

func (a *AVStreamSwap) UnmarshalParser(p *parse.AVStreamSwap) {
	a.URL = p.URL
	a.Status = p.Status
	a.LastURL = p.LastURL
	a.LastError = p.LastError
}

func (a *AVStreamSwap) MarshalParser() parse.AVStreamSwap {
	p := parse.AVStreamSwap{
		URL:       a.URL,
		Status:    a.Status,
		LastURL:   a.LastURL,
		LastError: a.LastError,
	}

	return p
}

type AVstream struct {
	Input          AVstreamIO
	Output         AVstreamIO
	Aqueue         uint64 // gauge
	Queue          uint64 // gauge
	Dup            uint64 // counter
	Drop           uint64 // counter
	Enc            uint64 // counter
	Looping        bool
	LoopingRuntime uint64 // sec
	Duplicating    bool
	GOP            string
	Mode           string // "file" or "live"
	Debug          interface{}
	Swap           AVStreamSwap

	// Codec parameter
	Codec     string
	Profile   int
	Level     int
	Pixfmt    string
	Width     uint64
	Height    uint64
	Samplefmt string
	Sampling  uint64
	Layout    string
	Channels  uint64
}

func (a *AVstream) UnmarshalParser(p *parse.AVstream) {
	if p == nil {
		return
	}

	a.Input.UnmarshalParser(&p.Input)
	a.Output.UnmarshalParser(&p.Output)

	a.Aqueue = p.Aqueue
	a.Queue = p.Queue
	a.Dup = p.Dup
	a.Drop = p.Drop
	a.Enc = p.Enc
	a.Looping = p.Looping
	a.LoopingRuntime = p.LoopingRuntime
	a.Duplicating = p.Duplicating
	a.GOP = p.GOP
	a.Mode = p.Mode
	a.Swap.UnmarshalParser(&p.Swap)

	a.Codec = p.Codec
	a.Profile = p.Profile
	a.Level = p.Level
	a.Pixfmt = p.Pixfmt
	a.Width = p.Width
	a.Height = p.Height
	a.Samplefmt = p.Samplefmt
	a.Sampling = p.Sampling
	a.Layout = p.Layout
	a.Channels = p.Channels
}

func (a *AVstream) MarshalParser() *parse.AVstream {
	p := &parse.AVstream{
		Input:          a.Input.MarshalParser(),
		Output:         a.Output.MarshalParser(),
		Aqueue:         a.Aqueue,
		Queue:          a.Queue,
		Dup:            a.Dup,
		Drop:           a.Drop,
		Enc:            a.Enc,
		Looping:        a.Looping,
		LoopingRuntime: a.LoopingRuntime,
		Duplicating:    a.Duplicating,
		GOP:            a.GOP,
		Mode:           a.Mode,
		Debug:          a.Debug,
		Swap:           a.Swap.MarshalParser(),
		Codec:          a.Codec,
		Profile:        a.Profile,
		Level:          a.Level,
		Pixfmt:         a.Pixfmt,
		Width:          a.Width,
		Height:         a.Height,
		Samplefmt:      a.Samplefmt,
		Sampling:       a.Sampling,
		Layout:         a.Layout,
		Channels:       a.Channels,
	}

	return p
}
