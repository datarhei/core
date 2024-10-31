package cluster

import (
	"sort"

	"github.com/datarhei/core/v16/cluster/node"
	"github.com/datarhei/core/v16/restream/app"
)

type Resources struct {
	CPU float64      // CPU 0-100*ncpu
	Mem uint64       // Memoryin bytes
	GPU ResourcesGPU // GPU resources
}

type ResourcesGPU struct {
	Index   int     // GPU number
	Usage   float64 // GPU general, 0-100
	Encoder float64 // GPU encoder, 0-100
	Decoder float64 // GPU decoder, 0-100
	Mem     uint64  // GPU memory in bytes
}

func ResourcesFromConfig(c *app.Config) Resources {
	r := Resources{}
	r.MarshalConfig(c)
	return r
}

func ResourcesFromProcess(c node.ProcessResources) Resources {
	r := Resources{}
	r.MarshalProcess(c)
	return r
}

func (r *Resources) MarshalConfig(c *app.Config) {
	r.CPU = c.LimitCPU
	r.Mem = c.LimitMemory
	r.GPU.Usage = c.LimitGPU.Usage
	r.GPU.Encoder = c.LimitGPU.Encoder
	r.GPU.Decoder = c.LimitGPU.Decoder
	r.GPU.Index = -1
}

func (r *Resources) MarshalProcess(c node.ProcessResources) {
	r.CPU = c.CPU
	r.Mem = c.Mem
	r.GPU.Usage = c.GPU.Usage
	r.GPU.Encoder = c.GPU.Encoder
	r.GPU.Decoder = c.GPU.Decoder
	r.GPU.Index = c.GPU.Index
}

func (r *Resources) HasGPU() bool {
	if r.GPU.Usage > 0 || r.GPU.Encoder > 0 || r.GPU.Decoder > 0 || r.GPU.Mem > 0 {
		return true
	}

	return false
}

func (r *Resources) DoesFitGPU(g node.ResourcesGPU) bool {
	if g.Usage+r.GPU.Usage < g.UsageLimit && g.Encoder+r.GPU.Encoder < g.UsageLimit && g.Decoder+r.GPU.Decoder < g.UsageLimit && g.Mem+r.GPU.Mem < g.MemLimit {
		return true
	}

	return false
}

type resourcePlanner struct {
	nodes   map[string]node.Resources
	blocked map[string]struct{}
}

func NewResourcePlanner(nodes map[string]node.About) *resourcePlanner {
	r := &resourcePlanner{
		nodes:   map[string]node.Resources{},
		blocked: map[string]struct{}{},
	}

	for nodeid, about := range nodes {
		r.nodes[nodeid] = about.Resources
		if about.State != "online" {
			r.blocked[nodeid] = struct{}{}
		}
	}

	return r
}

func (r *resourcePlanner) Throttling(nodeid string, throttling bool) {
	res, hasNode := r.nodes[nodeid]
	if !hasNode {
		return
	}

	res.IsThrottling = throttling

	r.nodes[nodeid] = res
}

// HasNodeEnough returns whether a node has enough resources available for the
// requested cpu, memory, anf gpu consumption.
func (r *resourcePlanner) HasNodeEnough(nodeid string, req Resources) bool {
	res, hasNode := r.nodes[nodeid]
	if !hasNode {
		return false
	}

	if _, hasNode := r.blocked[nodeid]; hasNode {
		return false
	}

	if res.Error != nil || res.IsThrottling {
		return false
	}

	if res.CPU+req.CPU >= res.CPULimit || res.Mem+req.Mem >= res.MemLimit {
		return false
	}

	if req.HasGPU() {
		found := false

		for _, g := range res.GPU {
			if req.DoesFitGPU(g) {
				found = true
				break
			}
		}

		if !found {
			return false
		}
	}

	return true
}

// FindBestNodes returns an array of nodeids that can fit the requested cpu, memory, and gpu requirements. If no
// such node is available, an empty array is returned. The array is sorted by the most suitable node first.
func (r *resourcePlanner) FindBestNodes(req Resources) []string {
	nodes := []string{}

	for id := range r.nodes {
		if r.HasNodeEnough(id, req) {
			nodes = append(nodes, id)
		}
	}

	sort.SliceStable(nodes, func(i, j int) bool {
		nodeA, nodeB := nodes[i], nodes[j]

		if r.nodes[nodeA].CPU != r.nodes[nodeB].CPU {
			return r.nodes[nodeA].CPU < r.nodes[nodeB].CPU
		}

		return r.nodes[nodeA].Mem <= r.nodes[nodeB].Mem
	})

	return nodes
}

// Add adds the resources of the node according to the cpu, memory, and gpu utilization.
func (r *resourcePlanner) Add(nodeid string, req Resources) {
	res, hasRes := r.nodes[nodeid]
	if !hasRes {
		return
	}

	res.CPU += req.CPU
	res.Mem += req.Mem

	if req.HasGPU() {
		for i, g := range res.GPU {
			if req.DoesFitGPU(g) {
				g.Usage += req.GPU.Usage
				g.Encoder += req.GPU.Encoder
				g.Decoder += req.GPU.Decoder
				g.Mem += req.GPU.Mem
				res.GPU[i] = g
				break
			}
		}
	}

	r.nodes[nodeid] = res
}

// Remove subtracts the resources from the node according to the cpu, memory, and gpu utilization.
func (r *resourcePlanner) Remove(nodeid string, req Resources) {
	res, hasRes := r.nodes[nodeid]
	if !hasRes {
		return
	}

	res.CPU -= min(res.CPU, req.CPU)
	res.Mem -= min(res.Mem, req.Mem)

	if req.HasGPU() {
		if req.GPU.Index > 0 && req.GPU.Index < len(res.GPU) {
			gpu := res.GPU[req.GPU.Index]
			gpu.Usage -= min(gpu.Usage, req.GPU.Usage)
			gpu.Encoder -= min(gpu.Encoder, req.GPU.Encoder)
			gpu.Decoder -= min(gpu.Decoder, req.GPU.Decoder)
			gpu.Mem -= min(gpu.Mem, req.GPU.Mem)
			res.GPU[req.GPU.Index] = gpu
		}
	}

	r.nodes[nodeid] = res
}

// Move adjusts the resources from the target and source node according to the cpu and memory utilization.
func (r *resourcePlanner) Move(target, source string, req Resources) {
	r.Add(target, req)
	r.Remove(source, req)
}

func (r *resourcePlanner) Map() map[string]node.Resources {
	return r.nodes
}

func (r *resourcePlanner) Blocked() []string {
	nodes := []string{}

	for nodeid := range r.blocked {
		nodes = append(nodes, nodeid)
	}

	return nodes
}
