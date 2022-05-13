package app

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
