package cluster

import (
	"github.com/datarhei/core/v16/cluster/node"
	"github.com/datarhei/core/v16/cluster/store"
)

func (c *cluster) doRebalance(emergency bool, term uint64) {
	if emergency {
		// Don't rebalance in emergency mode.
		return
	}

	logger := c.logger.WithField("term", term)

	logger.Debug().WithField("emergency", emergency).Log("Rebalancing")

	storeNodes := c.store.NodeList()
	nodes := c.manager.NodeList()
	have, err := c.manager.ClusterProcessList()
	if err != nil {
		logger.Warn().WithError(err).Log("Failed to retrieve complete process list")
		return
	}

	nodesMap := map[string]node.About{}

	for _, node := range nodes {
		about := node.About()

		if storeNode, hasStoreNode := storeNodes[about.ID]; hasStoreNode {
			about.State = storeNode.State
		}

		nodesMap[about.ID] = about
	}

	opStack, _ := rebalance(have, nodesMap)

	errors := c.applyOpStack(opStack, term, 5)

	for _, e := range errors {
		// Only apply the command if the error is different.
		process, _, err := c.store.ProcessGet(e.processid)
		if err != nil {
			continue
		}

		var errmessage string = ""

		if e.err != nil {
			if process.Error == e.err.Error() {
				continue
			}

			errmessage = e.err.Error()
		} else {
			if len(process.Error) == 0 {
				continue
			}
		}

		cmd := &store.Command{
			Operation: store.OpSetProcessError,
			Data: store.CommandSetProcessError{
				ID:    e.processid,
				Error: errmessage,
			},
		}

		c.applyCommand(cmd)
	}
}

// rebalance returns a list of operations that will move running processes away from nodes that are overloaded.
func rebalance(have []node.Process, nodes map[string]node.About) ([]interface{}, map[string]node.Resources) {
	resources := NewResourcePlanner(nodes)

	// Mark nodes as throttling where at least one process is still throttling
	for _, haveP := range have {
		if haveP.Resources.Throttling {
			resources.Throttling(haveP.NodeID, true)
		}
	}

	// Group all running processes by node and sort them by their runtime in ascending order.
	nodeProcessMap := createNodeProcessMap(have)

	// A map from the process reference to the nodes it is running on.
	haveReferenceAffinity := NewReferenceAffinity(have)

	opStack := []interface{}{}

	// Check if any of the nodes is overloaded.
	for id, r := range resources.Map() {
		// Ignore this node if the resource values are not reliable.
		if r.Error != nil {
			continue
		}

		// Check if node is overloaded.
		if r.CPU < r.CPULimit && r.Mem < r.MemLimit && !r.IsThrottling {
			continue
		}

		// Move processes from this node to another node with enough free resources.
		// The processes are ordered ascending by their runtime.
		processes := nodeProcessMap[id]
		if len(processes) == 0 {
			// If there are no processes on that node, we can't do anything.
			continue
		}

		overloadedNodeid := id

		for i, p := range processes {
			availableNodeid := ""

			// Try to move the process to a node where other processes with the same
			// reference currently reside.
			if len(p.Config.Reference) != 0 {
				raNodes := haveReferenceAffinity.Nodes(p.Config.Reference, p.Config.Domain)
				for _, raNodeid := range raNodes {
					// Do not move the process to the node it is currently on.
					if raNodeid == overloadedNodeid {
						continue
					}

					if resources.HasNodeEnough(raNodeid, ResourcesFromConfig(p.Config)) {
						availableNodeid = raNodeid
						break
					}
				}
			}

			// Find the best node with enough resources available.
			if len(availableNodeid) == 0 {
				nodes := resources.FindBestNodes(ResourcesFromConfig(p.Config))
				for _, nodeid := range nodes {
					if nodeid == overloadedNodeid {
						continue
					}

					availableNodeid = nodeid
					break
				}
			}

			if len(availableNodeid) == 0 {
				// There's no other node with enough resources to take over this process.
				opStack = append(opStack, processOpSkip{
					nodeid:    overloadedNodeid,
					processid: p.Config.ProcessID(),
					err:       errNotEnoughResourcesForRebalancing,
				})
				continue
			}

			opStack = append(opStack, processOpMove{
				fromNodeid: overloadedNodeid,
				toNodeid:   availableNodeid,
				config:     p.Config,
				metadata:   p.Metadata,
				order:      p.Order,
			})

			// Adjust the process.
			p.NodeID = availableNodeid
			processes[i] = p

			// Adjust the resources.
			resources.Move(availableNodeid, overloadedNodeid, ResourcesFromProcess(p.Resources))

			// Adjust the reference affinity.
			haveReferenceAffinity.Move(p.Config.Reference, p.Config.Domain, overloadedNodeid, availableNodeid)

			// Move only one process at a time.
			break
		}
	}

	return opStack, resources.Map()
}
