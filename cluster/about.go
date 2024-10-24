package cluster

import (
	"errors"
	"time"

	"github.com/datarhei/core/v16/cluster/raft"
	"github.com/datarhei/core/v16/slices"
)

type ClusterRaft struct {
	Address     string
	State       string
	LastContact time.Duration
	NumPeers    uint64
	LogTerm     uint64
	LogIndex    uint64
}

type ClusterNodeResources struct {
	IsThrottling bool                      // Whether this core is currently throttling
	NCPU         float64                   // Number of CPU on this node
	CPU          float64                   // Current CPU load, 0-100*ncpu
	CPULimit     float64                   // Defined CPU load limit, 0-100*ncpu
	CPUCore      float64                   // Current CPU load of the core itself, 0-100*ncpu
	Mem          uint64                    // Currently used memory in bytes
	MemLimit     uint64                    // Defined memory limit in bytes
	MemTotal     uint64                    // Total available memory in bytes
	MemCore      uint64                    // Current used memory of the core itself in bytes
	GPU          []ClusterNodeGPUResources // GPU resources
	Error        error
}

type ClusterNodeGPUResources struct {
	Mem        uint64  // Currently used memory in bytes
	MemLimit   uint64  // Defined memory limit in bytes
	MemTotal   uint64  // Total available memory in bytes
	Usage      float64 // Current general usage, 0-100
	UsageLimit float64 // Defined general usage limit, 0-100
	Encoder    float64 // Current encoder usage, 0-100
	Decoder    float64 // Current decoder usage, 0-100
}

type ClusterNode struct {
	ID          string
	Name        string
	Version     string
	State       string
	Error       error
	Voter       bool
	Leader      bool
	Address     string
	CreatedAt   time.Time
	Uptime      time.Duration
	LastContact time.Duration
	Latency     time.Duration
	Core        ClusterNodeCore
	Resources   ClusterNodeResources
}

type ClusterNodeCore struct {
	Address     string
	State       string
	Error       error
	LastContact time.Duration
	Latency     time.Duration
	Version     string
}

type ClusterAboutLeader struct {
	ID           string
	Address      string
	ElectedSince time.Duration
}

type ClusterAbout struct {
	ID      string
	Domains []string
	Leader  ClusterAboutLeader
	State   string
	Raft    ClusterRaft
	Nodes   []ClusterNode
	Version ClusterVersion
	Error   error
}

func (c *cluster) About() (ClusterAbout, error) {
	c.stateLock.RLock()
	hasLeader := c.hasRaftLeader
	domains := slices.Copy(c.hostnames)
	c.stateLock.RUnlock()

	about := ClusterAbout{
		ID:      c.id,
		Leader:  ClusterAboutLeader{},
		State:   "online",
		Version: Version,
		Domains: domains,
	}

	if !hasLeader {
		about.State = "offline"
		about.Error = errors.New("no raft leader elected")
	}

	stats := c.raft.Stats()

	about.Raft.Address = stats.Address
	about.Raft.State = stats.State
	about.Raft.LastContact = stats.LastContact
	about.Raft.NumPeers = stats.NumPeers
	about.Raft.LogIndex = stats.LogIndex
	about.Raft.LogTerm = stats.LogTerm

	servers, err := c.raft.Servers()
	if err != nil {
		c.logger.Warn().WithError(err).Log("Raft configuration")
	}

	serversMap := map[string]raft.Server{}

	for _, s := range servers {
		serversMap[s.ID] = s

		if s.Leader {
			about.Leader.ID = s.ID
			about.Leader.Address = s.Address
			about.Leader.ElectedSince = s.LastChange
		}
	}

	storeNodes := c.ListNodes()
	nodes := c.manager.NodeList()

	for _, node := range nodes {
		nodeAbout := node.About()

		node := ClusterNode{
			ID:          nodeAbout.ID,
			Name:        nodeAbout.Name,
			Version:     nodeAbout.Version,
			State:       nodeAbout.State,
			Error:       nodeAbout.Error,
			Address:     nodeAbout.Address,
			LastContact: time.Since(nodeAbout.LastContact),
			Latency:     nodeAbout.Latency,
			CreatedAt:   nodeAbout.Core.CreatedAt,
			Uptime:      nodeAbout.Core.Uptime,
			Core: ClusterNodeCore{
				Address:     nodeAbout.Core.Address,
				State:       nodeAbout.Core.State,
				Error:       nodeAbout.Core.Error,
				LastContact: time.Since(nodeAbout.Core.LastContact),
				Latency:     nodeAbout.Core.Latency,
				Version:     nodeAbout.Core.Version.Number,
			},
			Resources: ClusterNodeResources{
				IsThrottling: nodeAbout.Resources.IsThrottling,
				NCPU:         nodeAbout.Resources.NCPU,
				CPU:          nodeAbout.Resources.CPU,
				CPULimit:     nodeAbout.Resources.CPULimit,
				CPUCore:      nodeAbout.Resources.CPUCore,
				Mem:          nodeAbout.Resources.Mem,
				MemLimit:     nodeAbout.Resources.MemLimit,
				MemTotal:     nodeAbout.Resources.MemTotal,
				MemCore:      nodeAbout.Resources.MemCore,
				Error:        nodeAbout.Resources.Error,
			},
		}

		if len(nodeAbout.Resources.GPU) != 0 {
			node.Resources.GPU = make([]ClusterNodeGPUResources, len(nodeAbout.Resources.GPU))
			for i, gpu := range nodeAbout.Resources.GPU {
				node.Resources.GPU[i].Mem = gpu.Mem
				node.Resources.GPU[i].MemLimit = gpu.MemLimit
				node.Resources.GPU[i].MemTotal = gpu.MemTotal
				node.Resources.GPU[i].Usage = gpu.Usage
				node.Resources.GPU[i].UsageLimit = gpu.UsageLimit
				node.Resources.GPU[i].Encoder = gpu.Encoder
				node.Resources.GPU[i].Decoder = gpu.Decoder
			}
		}

		if s, ok := serversMap[nodeAbout.ID]; ok {
			node.Voter = s.Voter
			node.Leader = s.Leader
		}

		if storeNode, hasStoreNode := storeNodes[nodeAbout.ID]; hasStoreNode {
			if storeNode.State == "maintenance" {
				node.State = storeNode.State
			}
		}

		if about.State == "online" && node.State != "online" {
			about.State = "degraded"
		}

		about.Nodes = append(about.Nodes, node)
	}

	return about, nil
}
