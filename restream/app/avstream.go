package app

type AVstreamIO struct {
	State  string
	Packet uint64 // counter
	Time   uint64 // sec
	Size   uint64 // bytes
}

type AVStreamDebug struct {
	Sentinel struct {
		Reopen uint64
	}
	Track struct {
		FPS     int
		Bitrate uint64
	}
	Counter []uint64
	Locks   []int
	Version string
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
	Debug          AVStreamDebug
	Swap           AVStreamSwap
}
