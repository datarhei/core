package api

import (
	"time"
)

type ClusterNodeID struct {
	ID string `json:"id"`
}

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

type ClusterNodeState struct {
	State string `json:"state" validate:"required" enums:"online,maintenance,leave" jsonschema:"enum=online,enum=maintenance,enum=leave"`
}

type ClusterNodeCore struct {
	Address     string  `json:"address"`
	Status      string  `json:"status"`
	Error       string  `json:"error"`
	LastContact float64 `json:"last_contact_ms"` // milliseconds
	Latency     float64 `json:"latency_ms"`      // milliseconds
	Version     string  `json:"version"`
}

type ClusterNodeResources struct {
	IsThrottling bool    `json:"is_throttling"`
	NCPU         float64 `json:"ncpu"`
	CPU          float64 `json:"cpu_used"`           // percent 0-100*npcu
	CPULimit     float64 `json:"cpu_limit"`          // percent 0-100*npcu
	CPUCore      float64 `json:"cpu_core"`           // percent 0-100*ncpu
	Mem          uint64  `json:"memory_used_bytes"`  // bytes
	MemLimit     uint64  `json:"memory_limit_bytes"` // bytes
	MemTotal     uint64  `json:"memory_total_bytes"` // bytes
	MemCore      uint64  `json:"memory_core_bytes"`  // bytes
	Error        string  `json:"error"`
}

type ClusterRaft struct {
	Address     string  `json:"address"`
	State       string  `json:"state"`
	LastContact float64 `json:"last_contact_ms"` // milliseconds
	NumPeers    uint64  `json:"num_peers"`
	LogTerm     uint64  `json:"log_term"`
	LogIndex    uint64  `json:"log_index"`
}

type ClusterAboutLeader struct {
	ID           string `json:"id"`
	Address      string `json:"address"`
	ElectedSince uint64 `json:"elected_seconds"`
}

type ClusterAbout struct {
	ID          string             `json:"id"`
	Domains     []string           `json:"public_domains"`
	Leader      ClusterAboutLeader `json:"leader"`
	Status      string             `json:"status"`
	Raft        ClusterRaft        `json:"raft"`
	Nodes       []ClusterNode      `json:"nodes"`
	Version     string             `json:"version"`
	Degraded    bool               `json:"degraded"`
	DegradedErr string             `json:"degraded_error"`
}

type ClusterNodeFiles struct {
	LastUpdate int64               `json:"last_update"` // unix timestamp
	Files      map[string][]string `json:"files"`
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

type ClusterProcessReallocate struct {
	TargetNodeID string      `json:"target_node_id"`
	Processes    []ProcessID `json:"process_ids"`
}

type ClusterStoreNode struct {
	ID        string    `json:"id"`
	State     string    `json:"state"`
	UpdatedAt time.Time `json:"updated_at"`
}
