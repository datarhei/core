package api

type AVstreamIO struct {
	State  string `json:"state" enums:"running,idle" jsonschema:"enum=running,enum=idle"`
	Packet uint64 `json:"packet" format:"uint64"`
	Time   uint64 `json:"time"`
	Size   uint64 `json:"size_kb"`
}

type AVstream struct {
	Input          AVstreamIO `json:"input"`
	Output         AVstreamIO `json:"output"`
	Aqueue         uint64     `json:"aqueue" format:"uint64"`
	Queue          uint64     `json:"queue" format:"uint64"`
	Dup            uint64     `json:"dup" format:"uint64"`
	Drop           uint64     `json:"drop" format:"uint64"`
	Enc            uint64     `json:"enc" format:"uint64"`
	Looping        bool       `json:"looping"`
	LoopingRuntime uint64     `json:"looping_runtime" format:"uint64"`
	Duplicating    bool       `json:"duplicating"`
	GOP            string     `json:"gop"`
	Mode           string     `json:"mode"`
}
