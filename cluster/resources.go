package cluster

import (
	"sort"

	"github.com/datarhei/core/v16/cluster/node"
)

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
// requested cpu and memory consumption.
func (r *resourcePlanner) HasNodeEnough(nodeid string, cpu float64, mem uint64) bool {
	res, hasNode := r.nodes[nodeid]
	if !hasNode {
		return false
	}

	if _, hasNode := r.blocked[nodeid]; hasNode {
		return false
	}

	if res.Error == nil && res.CPU+cpu < res.CPULimit && res.Mem+mem < res.MemLimit && !res.IsThrottling {
		return true
	}

	return false
}

// FindBestNodes returns an array of nodeids that can fit the requested cpu and memory requirements. If no
// such node is available, an empty array is returned. The array is sorted by the most suitable node first.
func (r *resourcePlanner) FindBestNodes(cpu float64, mem uint64) []string {
	nodes := []string{}

	for id := range r.nodes {
		if r.HasNodeEnough(id, cpu, mem) {
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

// Add adds the resources of the node according to the cpu and memory utilization.
func (r *resourcePlanner) Add(nodeid string, cpu float64, mem uint64) {
	res, hasRes := r.nodes[nodeid]
	if !hasRes {
		return
	}

	res.CPU += cpu
	res.Mem += mem
	r.nodes[nodeid] = res
}

// Remove subtracts the resources from the node according to the cpu and memory utilization.
func (r *resourcePlanner) Remove(nodeid string, cpu float64, mem uint64) {
	res, hasRes := r.nodes[nodeid]
	if !hasRes {
		return
	}

	res.CPU -= cpu
	if res.CPU < 0 {
		res.CPU = 0
	}
	if mem >= res.Mem {
		res.Mem = 0
	} else {
		res.Mem -= mem
	}
	r.nodes[nodeid] = res
}

// Move adjusts the resources from the target and source node according to the cpu and memory utilization.
func (r *resourcePlanner) Move(target, source string, cpu float64, mem uint64) {
	r.Add(target, cpu, mem)
	r.Remove(source, cpu, mem)
}

func (r *resourcePlanner) Map() map[string]node.Resources {
	return r.nodes
}
