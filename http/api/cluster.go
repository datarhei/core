package api

import (
	"encoding/json"
	"time"
)

type ClusterNode struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Version     string               `json:"version"`
	Status      string               `json:"status"`
	Error       string               `json:"error"`
	Voter       bool                 `json:"voter"`
	Leader      bool                 `json:"leader"`
	Address     string               `json:"address"`
	CreatedAt   string               `json:"created_at"` // RFC 3339
	Uptime      int64                `json:"uptime_seconds"`
	LastContact float64              `json:"last_contact_ms"`
	Latency     float64              `json:"latency_ms"`
	Core        ClusterNodeCore      `json:"core"`
	Resources   ClusterNodeResources `json:"resources"`
}

type ClusterNodeCore struct {
	Address     string  `json:"address"`
	Status      string  `json:"status"`
	Error       string  `json:"error"`
	LastContact float64 `json:"last_contact_ms"`
	Latency     float64 `json:"latency_ms"`
}

type ClusterNodeResources struct {
	IsThrottling bool    `json:"is_throttling"`
	NCPU         float64 `json:"ncpu"`
	CPU          float64 `json:"cpu_used"`           // percent 0-100*npcu
	CPULimit     float64 `json:"cpu_limit"`          // percent 0-100*npcu
	Mem          uint64  `json:"memory_used_bytes"`  // bytes
	MemLimit     uint64  `json:"memory_limit_bytes"` // bytes
}

type ClusterRaft struct {
	Address     string  `json:"address"`
	State       string  `json:"state"`
	LastContact float64 `json:"last_contact_ms"` // milliseconds
	NumPeers    uint64  `json:"num_peers"`
	LogTerm     uint64  `json:"log_term"`
	LogIndex    uint64  `json:"log_index"`
}

type ClusterAbout struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Leader      bool          `json:"leader"`
	Address     string        `json:"address"`
	Raft        ClusterRaft   `json:"raft"`
	Nodes       []ClusterNode `json:"nodes"`
	Version     string        `json:"version"`
	Degraded    bool          `json:"degraded"`
	DegradedErr string        `json:"degraded_error"`
}

type ClusterNodeFiles struct {
	LastUpdate int64               `json:"last_update"` // unix timestamp
	Files      map[string][]string `json:"files"`
}

type ClusterProcess struct {
	ID        string      `json:"id"`
	Owner     string      `json:"owner"`
	Domain    string      `json:"domain"`
	NodeID    string      `json:"node_id"`
	Reference string      `json:"reference"`
	Order     string      `json:"order"`
	State     string      `json:"state"`
	CPU       json.Number `json:"cpu" swaggertype:"number" jsonschema:"type=number"` // percent 0-100*ncpu
	Memory    uint64      `json:"memory_bytes"`                                      // bytes
	Runtime   int64       `json:"runtime_seconds"`                                   // seconds
}

type ClusterLock struct {
	Name       string    `json:"name"`
	ValidUntil time.Time `json:"valid_until"`
}

type ClusterKVSValue struct {
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ClusterKVS map[string]ClusterKVSValue

type ClusterProcessMap map[string]string
