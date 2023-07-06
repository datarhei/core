package api

import (
	"time"
)

type ClusterNode struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Address     string               `json:"address"`
	CreatedAt   string               `json:"created_at"`
	Uptime      int64                `json:"uptime_seconds"`
	LastContact int64                `json:"last_contact"` // unix timestamp
	Latency     float64              `json:"latency_ms"`   // milliseconds
	State       string               `json:"state"`
	Resources   ClusterNodeResources `json:"resources"`
}

type ClusterNodeResources struct {
	IsThrottling bool    `json:"is_throttling"`
	NCPU         float64 `json:"ncpu"`
	CPU          float64 `json:"cpu_used"`           // percent 0-100*npcu
	CPULimit     float64 `json:"cpu_limit"`          // percent 0-100*npcu
	Mem          uint64  `json:"memory_used_bytes"`  // bytes
	MemLimit     uint64  `json:"memory_limit_bytes"` // bytes
}

type ClusterNodeFiles struct {
	LastUpdate int64               `json:"last_update"` // unix timestamp
	Files      map[string][]string `json:"files"`
}

type ClusterRaftServer struct {
	ID      string `json:"id"`
	Address string `json:"address"` // raft address
	Voter   bool   `json:"voter"`
	Leader  bool   `json:"leader"`
}

type ClusterRaftStats struct {
	State       string  `json:"state"`
	LastContact float64 `json:"last_contact_ms"`
	NumPeers    uint64  `json:"num_peers"`
}

type ClusterRaft struct {
	Server []ClusterRaftServer `json:"server"`
	Stats  ClusterRaftStats    `json:"stats"`
}

type ClusterAbout struct {
	ID                string        `json:"id"`
	Address           string        `json:"address"`
	ClusterAPIAddress string        `json:"cluster_api_address"`
	CoreAPIAddress    string        `json:"core_api_address"`
	Raft              ClusterRaft   `json:"raft"`
	Nodes             []ClusterNode `json:"nodes"`
	Version           string        `json:"version"`
	Degraded          bool          `json:"degraded"`
	DegradedErr       string        `json:"degraded_error"`
}

type ClusterProcess struct {
	ID        string  `json:"id"`
	Owner     string  `json:"owner"`
	Domain    string  `json:"domain"`
	NodeID    string  `json:"node_id"`
	Reference string  `json:"reference"`
	Order     string  `json:"order"`
	State     string  `json:"state"`
	CPU       float64 `json:"cpu" swaggertype:"number" jsonschema:"type=number"` // percent 0-100*ncpu
	Memory    uint64  `json:"memory_bytes"`                                      // bytes
	Runtime   int64   `json:"runtime_seconds"`                                   // seconds
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
