package api

// SessionStats are the accumulated numbers for the session summary
type SessionStats struct {
	TotalSessions uint64 `json:"sessions" format:"uint64"`
	TotalRxBytes  uint64 `json:"traffic_rx_mb" format:"uint64"`
	TotalTxBytes  uint64 `json:"traffic_tx_mb" format:"uint64"`
}

// SessionPeers is for the grouping by peers in the summary
type SessionPeers struct {
	SessionStats

	Locations map[string]SessionStats `json:"local"`
}

// Session represents an active session
type Session struct {
	ID        string  `json:"id"`
	Reference string  `json:"reference"`
	CreatedAt int64   `json:"created_at" format:"int64"`
	Location  string  `json:"local"`
	Peer      string  `json:"remote"`
	Extra     string  `json:"extra"`
	RxBytes   uint64  `json:"bytes_rx" format:"uint64"`
	TxBytes   uint64  `json:"bytes_tx" format:"uint64"`
	RxBitrate float64 `json:"bandwidth_rx_kbit" swaggertype:"number" jsonschema:"type=number"` // kbit/s
	TxBitrate float64 `json:"bandwidth_tx_kbit" swaggertype:"number" jsonschema:"type=number"` // kbit/s
}

// SessionSummaryActive represents the currently active sessions
type SessionSummaryActive struct {
	SessionList  []Session `json:"list"`
	Sessions     uint64    `json:"sessions" format:"uint64"`
	RxBitrate    float64   `json:"bandwidth_rx_mbit" swaggertype:"number" jsonschema:"type=number"` // mbit/s
	TxBitrate    float64   `json:"bandwidth_tx_mbit" swaggertype:"number" jsonschema:"type=number"` // mbit/s
	MaxSessions  uint64    `json:"max_sessions" format:"uint64"`
	MaxRxBitrate float64   `json:"max_bandwidth_rx_mbit" swaggertype:"number" jsonschema:"type=number"` // mbit/s
	MaxTxBitrate float64   `json:"max_bandwidth_tx_mbit" swaggertype:"number" jsonschema:"type=number"` // mbit/s
}

// SessionSummarySummary represents the summary (history) of all finished sessions
type SessionSummarySummary struct {
	Peers      map[string]SessionPeers `json:"remote"`
	Locations  map[string]SessionStats `json:"local"`
	References map[string]SessionStats `json:"reference"`
	SessionStats
}

// SessionSummary is the API representation of a session.Summary
type SessionSummary struct {
	Active SessionSummaryActive `json:"active"`

	Summary SessionSummarySummary `json:"summary"`
}

type SessionsSummary map[string]SessionSummary

// SessionsActive is the API representation of all active sessions
type SessionsActive map[string][]Session
