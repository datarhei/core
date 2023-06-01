package api

type PlayoutStatusIO struct {
	State  string `json:"state" enums:"running,idle" jsonschema:"enum=running,enum=idle"`
	Packet uint64 `json:"packet" format:"uint64"`
	Time   uint64 `json:"time" format:"uint64"`
	Size   uint64 `json:"size_kb" format:"uint64"`
}

type PlayoutStatusSwap struct {
	Address     string `json:"url"`
	Status      string `json:"status"`
	LastAddress string `json:"lasturl"`
	LastError   string `json:"lasterror"`
}

type PlayoutStatus struct {
	ID          string            `json:"id"`
	Address     string            `json:"url"`
	Stream      uint64            `json:"stream" format:"uint64"`
	Queue       uint64            `json:"queue" format:"uint64"`
	AQueue      uint64            `json:"aqueue" format:"uint64"`
	Dup         uint64            `json:"dup" format:"uint64"`
	Drop        uint64            `json:"drop" format:"uint64"`
	Enc         uint64            `json:"enc" format:"uint64"`
	Looping     bool              `json:"looping"`
	Duplicating bool              `json:"duplicating"`
	GOP         string            `json:"gop"`
	Debug       interface{}       `json:"debug"`
	Input       PlayoutStatusIO   `json:"input"`
	Output      PlayoutStatusIO   `json:"output"`
	Swap        PlayoutStatusSwap `json:"swap"`
}
