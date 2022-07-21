package api

import (
	"github.com/datarhei/core/v16/restream/app"
)

type AVstreamIO struct {
	State  string `json:"state" enums:"running,idle" jsonschema:"enum=running,enum=idle"`
	Packet uint64 `json:"packet"`
	Time   uint64 `json:"time"`
	Size   uint64 `json:"size_kb"`
}

func (i *AVstreamIO) Unmarshal(io *app.AVstreamIO) {
	if io == nil {
		return
	}

	i.State = io.State
	i.Packet = io.Packet
	i.Time = io.Time
	i.Size = io.Size
}

type AVstream struct {
	Input       AVstreamIO `json:"input"`
	Output      AVstreamIO `json:"output"`
	Aqueue      uint64     `json:"aqueue"`
	Queue       uint64     `json:"queue"`
	Dup         uint64     `json:"dup"`
	Drop        uint64     `json:"drop"`
	Enc         uint64     `json:"enc"`
	Looping     bool       `json:"looping"`
	Duplicating bool       `json:"duplicating"`
	GOP         string     `json:"gop"`
}

func (a *AVstream) Unmarshal(av *app.AVstream) {
	if av == nil {
		return
	}

	a.Aqueue = av.Aqueue
	a.Queue = av.Queue
	a.Dup = av.Dup
	a.Drop = av.Drop
	a.Enc = av.Enc
	a.Looping = av.Looping
	a.Duplicating = av.Duplicating
	a.GOP = av.GOP

	a.Input.Unmarshal(&av.Input)
	a.Output.Unmarshal(&av.Output)
}
