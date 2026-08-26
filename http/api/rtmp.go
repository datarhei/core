package api

type RTMPChannelName struct {
	Name string `json:"name" jsonschema:"minLength=1"`
}

// RTMPChannel represents details about a currently connected RTMP publisher and subscribers
type RTMPChannel struct {
	Name       string           `json:"name" jsonschema:"minLength=1"`
	IsProxy    bool             `json:"is_proxy"`
	Publisher  RTMPConnection   `json:"publisher"`
	Subscriber []RTMPConnection `json:"subscriber"`
}

type RTMPConnection struct {
	Remote    string `json:"remote"`
	CreatedAt int64  `json:"created_at" format:"int64"`
	RxBytes   uint64 `json:"rx_bytes" format:"uint64"`
	TxBytes   uint64 `json:"tx_bytes" format:"uint64"`
}
