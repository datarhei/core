package api

import "github.com/datarhei/core/v16/playout"

type PlayoutStatusIO struct {
	State  string `json:"state" enums:"running,idle" jsonschema:"enum=running,enum=idle"`
	Packet uint64 `json:"packet"`
	Time   uint64 `json:"time"`
	Size   uint64 `json:"size_kb"`
}

func (i *PlayoutStatusIO) Unmarshal(io playout.StatusIO) {
	i.State = io.State
	i.Packet = io.Packet
	i.Time = io.Time
	i.Size = io.Size
}

type PlayoutStatusSwap struct {
	Address     string `json:"url"`
	Status      string `json:"status"`
	LastAddress string `json:"lasturl"`
	LastError   string `json:"lasterror"`
}

func (s *PlayoutStatusSwap) Unmarshal(swap playout.StatusSwap) {
	s.Address = swap.Address
	s.Status = swap.Status
	s.LastAddress = swap.LastAddress
	s.LastError = swap.LastError
}

type PlayoutStatus struct {
	ID          string            `json:"id"`
	Address     string            `json:"url"`
	Stream      uint64            `json:"stream"`
	Queue       uint64            `json:"queue"`
	AQueue      uint64            `json:"aqueue"`
	Dup         uint64            `json:"dup"`
	Drop        uint64            `json:"drop"`
	Enc         uint64            `json:"enc"`
	Looping     bool              `json:"looping"`
	Duplicating bool              `json:"duplicating"`
	GOP         string            `json:"gop"`
	Debug       interface{}       `json:"debug"`
	Input       PlayoutStatusIO   `json:"input"`
	Output      PlayoutStatusIO   `json:"output"`
	Swap        PlayoutStatusSwap `json:"swap"`
}

func (s *PlayoutStatus) Unmarshal(status playout.Status) {
	s.ID = status.ID
	s.Address = status.Address
	s.Stream = status.Stream
	s.Queue = status.Queue
	s.AQueue = status.AQueue
	s.Dup = status.Dup
	s.Drop = status.Drop
	s.Enc = status.Enc
	s.Looping = status.Looping
	s.Duplicating = status.Duplicating
	s.GOP = status.GOP
	s.Debug = status.Debug

	s.Input.Unmarshal(status.Input)
	s.Output.Unmarshal(status.Output)
	s.Swap.Unmarshal(status.Swap)
}
