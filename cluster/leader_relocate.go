package cluster

import (
	"github.com/datarhei/core/v16/cluster/node"
	"github.com/datarhei/core/v16/cluster/store"
	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/restream/app"
)

func (c *cluster) doRelocate(emergency bool, term uint64) {
	if emergency {
		// Don't relocate in emergency mode.
		return
	}

	logger := c.logger.WithField("term", term)

	logger.Debug().WithField("emergency", emergency).Log("Relocating")

	relocateMap := c.store.ProcessGetRelocateMap()
	storeNodes := c.store.NodeList()
	have := c.manager.ClusterProcessList()
	nodes := c.manager.NodeList()

	nodesMap := map[string]node.About{}

	for _, node := range nodes {
		about := node.About()

		if storeNode, hasStoreNode := storeNodes[about.ID]; hasStoreNode {
			about.State = storeNode.State
		}

		nodesMap[about.ID] = about
	}

	logger.Debug().WithFields(log.Fields{
		"relocate": relocate,
		"have":     have,
		"nodes":    nodesMap,
	}).Log("Rebalance")

	opStack, _, relocatedProcessIDs := relocate(have, nodesMap, relocateMap)

	errors := c.applyOpStack(opStack, term)

	for _, e := range errors {
		// Only apply the command if the error is different.
		process, err := c.store.ProcessGet(e.processid)
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

	cmd := store.CommandUnsetRelocateProcess{
		ID: []app.ProcessID{},
	}

	for _, processid := range relocatedProcessIDs {
		cmd.ID = append(cmd.ID, app.ParseProcessID(processid))
	}

	if len(cmd.ID) != 0 {
		c.applyCommand(&store.Command{
			Operation: store.OpUnsetRelocateProcess,
			Data:      cmd,
		})
	}
}

// relocate returns a list of operations that will move deployed processes to different nodes.
func relocate(have []node.Process, nodes map[string]node.About, relocateMap map[string]string) ([]interface{}, map[string]node.Resources, []string) {
	resources := NewResourcePlanner(nodes)

	// Mark nodes as throttling where at least one process is still throttling
	for _, haveP := range have {
		if haveP.Throttling {
			resources.Throttling(haveP.NodeID, true)
		}
	}

	relocatedProcessIDs := []string{}

	// A map from the process reference to the nodes it is running on.
	haveReferenceAffinity := NewReferenceAffinity(have)

	opStack := []interface{}{}

	// Check for any requested relocations.
	for processid, targetNodeid := range relocateMap {
		process := node.Process{}

		found := false
		for _, p := range have {
			if processid == p.Config.ProcessID().String() {
				process = p
				found = true
				break
			}
		}

		if !found {
			relocatedProcessIDs = append(relocatedProcessIDs, processid)
			continue
		}

		sourceNodeid := process.NodeID

		if sourceNodeid == targetNodeid {
			relocatedProcessIDs = append(relocatedProcessIDs, processid)
			continue
		}

		if len(targetNodeid) != 0 {
			_, hasNode := nodes[targetNodeid]

			if !hasNode || !resources.HasNodeEnough(targetNodeid, process.Config.LimitCPU, process.Config.LimitMemory) {
				targetNodeid = ""
			}
		}

		if len(targetNodeid) == 0 {
			// Try to move the process to a node where other processes with the same
			// reference currently reside.
			if len(process.Config.Reference) != 0 {
				raNodes := haveReferenceAffinity.Nodes(process.Config.Reference, process.Config.Domain)
				for _, raNodeid := range raNodes {
					// Do not move the process to the node it is currently on.
					if raNodeid == sourceNodeid {
						continue
					}

					if resources.HasNodeEnough(raNodeid, process.Config.LimitCPU, process.Config.LimitMemory) {
						targetNodeid = raNodeid
						break
					}
				}
			}

			// Find the best node with enough resources available.
			if len(targetNodeid) == 0 {
				nodes := resources.FindBestNodes(process.Config.LimitCPU, process.Config.LimitMemory)
				for _, nodeid := range nodes {
					if nodeid == sourceNodeid {
						continue
					}

					targetNodeid = nodeid
					break
				}
			}

			if len(targetNodeid) == 0 {
				// There's no other node with enough resources to take over this process.
				opStack = append(opStack, processOpSkip{
					nodeid:    sourceNodeid,
					processid: process.Config.ProcessID(),
					err:       errNotEnoughResourcesForRelocating,
				})
				continue
			}
		}

		opStack = append(opStack, processOpMove{
			fromNodeid: sourceNodeid,
			toNodeid:   targetNodeid,
			config:     process.Config,
			metadata:   process.Metadata,
			order:      process.Order,
		})

		// Adjust the resources.
		resources.Move(targetNodeid, sourceNodeid, process.CPU, process.Mem)

		// Adjust the reference affinity.
		haveReferenceAffinity.Move(process.Config.Reference, process.Config.Domain, sourceNodeid, targetNodeid)

		relocatedProcessIDs = append(relocatedProcessIDs, processid)
	}

	return opStack, resources.Map(), relocatedProcessIDs
}
