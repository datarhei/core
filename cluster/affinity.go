package cluster

import (
	"sort"

	"github.com/datarhei/core/v16/cluster/node"
)

type referenceAffinityNodeCount struct {
	nodeid string
	count  uint64
}

type referenceAffinity struct {
	m map[string][]referenceAffinityNodeCount
}

// NewReferenceAffinity returns a referenceAffinity. This is a map of references (per domain) to an array of
// nodes this reference is found on and their count.
func NewReferenceAffinity(processes []node.Process) *referenceAffinity {
	ra := &referenceAffinity{
		m: map[string][]referenceAffinityNodeCount{},
	}

	for _, p := range processes {
		if len(p.Config.Reference) == 0 {
			continue
		}

		key := p.Config.Reference + "@" + p.Config.Domain

		// Here we count how often a reference is present on a node. When
		// moving processes to a different node, the node with the highest
		// count of same references will be the first candidate.
		found := false
		arr := ra.m[key]
		for i, count := range arr {
			if count.nodeid == p.NodeID {
				count.count++
				arr[i] = count
				found = true
				break
			}
		}

		if !found {
			arr = append(arr, referenceAffinityNodeCount{
				nodeid: p.NodeID,
				count:  1,
			})
		}

		ra.m[key] = arr
	}

	// Sort every reference count in decreasing order for each reference.
	for ref, count := range ra.m {
		sort.SliceStable(count, func(a, b int) bool {
			return count[a].count > count[b].count
		})

		ra.m[ref] = count
	}

	return ra
}

// Nodes returns a list of node IDs for the provided reference and domain. The list
// is ordered by how many references are on the nodes in descending order.
func (ra *referenceAffinity) Nodes(reference, domain string) []string {
	if len(reference) == 0 {
		return nil
	}

	key := reference + "@" + domain

	counts, ok := ra.m[key]
	if !ok {
		return nil
	}

	nodes := []string{}

	for _, count := range counts {
		nodes = append(nodes, count.nodeid)
	}

	return nodes
}

// Add adds a reference on a node to an existing reference affinity.
func (ra *referenceAffinity) Add(reference, domain, nodeid string) {
	if len(reference) == 0 {
		return
	}

	key := reference + "@" + domain

	counts, ok := ra.m[key]
	if !ok {
		ra.m[key] = []referenceAffinityNodeCount{
			{
				nodeid: nodeid,
				count:  1,
			},
		}

		return
	}

	found := false
	for i, count := range counts {
		if count.nodeid == nodeid {
			count.count++
			counts[i] = count
			found = true
			break
		}
	}

	if !found {
		counts = append(counts, referenceAffinityNodeCount{
			nodeid: nodeid,
			count:  1,
		})
	}

	ra.m[key] = counts
}

// Move moves a reference from one node to another node in an existing reference affinity.
func (ra *referenceAffinity) Move(reference, domain, fromnodeid, tonodeid string) {
	if len(reference) == 0 {
		return
	}

	key := reference + "@" + domain

	counts, ok := ra.m[key]
	if !ok {
		ra.m[key] = []referenceAffinityNodeCount{
			{
				nodeid: tonodeid,
				count:  1,
			},
		}

		return
	}

	found := false
	for i, count := range counts {
		if count.nodeid == tonodeid {
			count.count++
			counts[i] = count
			found = true
		} else if count.nodeid == fromnodeid {
			count.count--
			counts[i] = count
		}
	}

	if !found {
		counts = append(counts, referenceAffinityNodeCount{
			nodeid: tonodeid,
			count:  1,
		})
	}

	newCounts := []referenceAffinityNodeCount{}

	for _, count := range counts {
		if count.count == 0 {
			continue
		}

		newCounts = append(newCounts, count)
	}

	ra.m[key] = newCounts
}
