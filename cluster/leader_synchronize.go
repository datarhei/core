package cluster

import (
	"bytes"
	"maps"
	"time"

	"github.com/datarhei/core/v16/cluster/node"
	"github.com/datarhei/core/v16/cluster/store"
	"github.com/datarhei/core/v16/encoding/json"
)

func (c *cluster) doSynchronize(emergency bool, term uint64) {
	logger := c.logger.WithField("term", term)

	logger.Debug().WithField("emergency", emergency).Log("Synchronizing")

	wish := c.store.ProcessGetNodeMap()
	want := c.store.ProcessList()
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

	opStack, _, reality := synchronize(wish, want, have, nodesMap, c.nodeRecoverTimeout)

	if !emergency && !maps.Equal(wish, reality) {
		cmd := &store.Command{
			Operation: store.OpSetProcessNodeMap,
			Data: store.CommandSetProcessNodeMap{
				Map: reality,
			},
		}

		c.applyCommand(cmd)
	}

	errors := c.applyOpStack(opStack, term, 5)

	if !emergency {
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
	}
}

// isMetadataUpdateRequired compares two metadata. It relies on the documented property that json.Marshal
// sorts the map keys prior encoding.
func isMetadataUpdateRequired(wantMap map[string]interface{}, haveMap map[string]interface{}) (bool, map[string]interface{}) {
	hasChanges := false
	changeMap := map[string]interface{}{}

	haveMapKeys := map[string]struct{}{}

	for key := range haveMap {
		haveMapKeys[key] = struct{}{}
	}

	for key, wantMapValue := range wantMap {
		haveMapValue, ok := haveMap[key]
		if !ok {
			// A key in map1 exists, that doesn't exist in map2, we need to update.
			hasChanges = true
		}

		// Compare the values
		changesData, err := json.Marshal(wantMapValue)
		if err != nil {
			continue
		}

		completeData, err := json.Marshal(haveMapValue)
		if err != nil {
			continue
		}

		if !bytes.Equal(changesData, completeData) {
			// The values are not equal, we need to update.
			hasChanges = true
		}

		delete(haveMapKeys, key)

		changeMap[key] = wantMapValue
	}

	for key := range haveMapKeys {
		// If there keys in map2 that are not in map1, we have to update.
		hasChanges = true
		changeMap[key] = nil
	}

	return hasChanges, changeMap
}

// synchronize returns a list of operations in order to adjust the "have" list to the "want" list
// with taking the available resources on each node into account.
func synchronize(wish map[string]string, want []store.Process, have []node.Process, nodes map[string]node.About, nodeRecoverTimeout time.Duration) ([]interface{}, map[string]node.Resources, map[string]string) {
	resources := NewResourcePlanner(nodes)

	// Mark nodes as throttling where at least one process is still throttling
	for _, haveP := range have {
		if haveP.Throttling {
			resources.Throttling(haveP.NodeID, true)
		}
	}

	// A map same as wish, but reflecting the actual situation.
	reality := map[string]string{}

	// A map from the process ID to the process config of the processes
	// we want to be running on the nodes.
	wantMap := map[string]store.Process{}
	for _, wantP := range want {
		pid := wantP.Config.ProcessID().String()
		wantMap[pid] = wantP
	}

	opStack := []interface{}{}

	// Now we iterate through the processes we actually have running on the nodes
	// and remove them from the wantMap. We also make sure that they have the correct order.
	// If a process cannot be found on the wantMap, it will be deleted from the nodes.
	haveAfterRemove := []node.Process{}
	wantOrderStart := []node.Process{}

	for _, haveP := range have {
		pid := haveP.Config.ProcessID().String()
		wantP, ok := wantMap[pid]
		if !ok {
			// The process is not on the wantMap. Delete it and adjust the resources.
			opStack = append(opStack, processOpDelete{
				nodeid:    haveP.NodeID,
				processid: haveP.Config.ProcessID(),
			})

			resources.Remove(haveP.NodeID, haveP.CPU, haveP.Mem)

			continue
		}

		// The process is on the wantMap. Update the process if the configuration and/or metadata differ.
		hasConfigChanges := !wantP.Config.Equal(haveP.Config)
		hasMetadataChanges, metadata := isMetadataUpdateRequired(wantP.Metadata, haveP.Metadata)
		if hasConfigChanges || hasMetadataChanges {
			// TODO: When the required resources increase, should we move this process to a node
			// that has them available? Otherwise, this node might start throttling. However, this
			// will result in rebalancing.
			opStack = append(opStack, processOpUpdate{
				nodeid:    haveP.NodeID,
				processid: haveP.Config.ProcessID(),
				config:    wantP.Config,
				metadata:  metadata,
			})
		}

		delete(wantMap, pid)
		reality[pid] = haveP.NodeID

		if haveP.Order != wantP.Order {
			if wantP.Order == "start" {
				// Delay pushing them to the stack in order to have
				// all resources released first.
				wantOrderStart = append(wantOrderStart, haveP)
			} else {
				opStack = append(opStack, processOpStop{
					nodeid:    haveP.NodeID,
					processid: haveP.Config.ProcessID(),
				})

				// Release the resources.
				resources.Remove(haveP.NodeID, haveP.CPU, haveP.Mem)
			}
		}

		haveAfterRemove = append(haveAfterRemove, haveP)
	}

	for _, haveP := range wantOrderStart {
		nodeid := haveP.NodeID

		resources.Add(nodeid, haveP.Config.LimitCPU, haveP.Config.LimitMemory)

		// TODO: check if the current node has actually enough resources available,
		// otherwise it needs to be moved somewhere else. If the node doesn't
		// have enough resources available, the process will be prevented
		// from starting.

		/*
			if hasNodeEnoughResources(r, haveP.Config.LimitCPU, haveP.Config.LimitMemory) {
				// Consume the resources
				r.CPU += haveP.Config.LimitCPU
				r.Mem += haveP.Config.LimitMemory
				resources[nodeid] = r
			} else {
				nodeid = findBestNodeForProcess(resources, haveP.Config.LimitCPU, haveP.Config.LimitMemory)
				if len(nodeid) == 0 {
					// Start it anyways and let it run into an error
					opStack = append(opStack, processOpStart{
						nodeid:    nodeid,
						processid: haveP.Config.ProcessID(),
					})

					continue
				}

				if nodeid != haveP.NodeID {
					opStack = append(opStack, processOpMove{
						fromNodeid: haveP.NodeID,
						toNodeid:   nodeid,
						config:     haveP.Config,
						metadata:   haveP.Metadata,
						order:      haveP.Order,
					})
				}

				// Consume the resources
				r, ok := resources[nodeid]
				if ok {
					r.CPU += haveP.Config.LimitCPU
					r.Mem += haveP.Config.LimitMemory
					resources[nodeid] = r
				}
			}
		*/

		opStack = append(opStack, processOpStart{
			nodeid:    nodeid,
			processid: haveP.Config.ProcessID(),
		})
	}

	have = haveAfterRemove

	// In case a node didn't respond, some PID are still on the wantMap, that would run on
	// the currently not responding nodes. We use the wish map to assign them to the node.
	// If the node is unavailable for too long, keep these processes on the wantMap, otherwise
	// remove them and hope that they will reappear during the nodeRecoverTimeout.
	for pid := range wantMap {
		// Check if this PID is be assigned to a node.
		if nodeid, ok := wish[pid]; ok {
			// Check for how long the node hasn't been contacted, or if it still exists.
			if node, ok := nodes[nodeid]; ok {
				if node.State == "online" {
					continue
				}

				if time.Since(node.LastContact) <= nodeRecoverTimeout {
					reality[pid] = nodeid
					delete(wantMap, pid)
				}
			}
		}
	}

	// The wantMap now contains only those processes that need to be installed on a node.
	// We will rebuild the "want" array from the wantMap in the same order as the original
	// "want" array to make the resulting opStack deterministic.
	wantReduced := []store.Process{}
	for _, wantP := range want {
		pid := wantP.Config.ProcessID().String()
		if _, ok := wantMap[pid]; !ok {
			continue
		}

		wantReduced = append(wantReduced, wantP)
	}

	// Create a map from the process reference to the node it is running on.
	haveReferenceAffinity := NewReferenceAffinity(have)

	// Now, all remaining processes in the wantMap must be added to one of the nodes.
	for _, wantP := range wantReduced {
		pid := wantP.Config.ProcessID().String()

		// If a process doesn't have any limits defined, reject that process.
		if wantP.Config.LimitCPU <= 0 || wantP.Config.LimitMemory <= 0 {
			opStack = append(opStack, processOpReject{
				processid: wantP.Config.ProcessID(),
				err:       errNoLimitsDefined,
			})

			continue
		}

		// Check if there are already processes with the same reference, and if so
		// choose this node. Then check the node if it has enough resources left. If
		// not, then select a node with the most available resources.
		nodeid := ""

		// Try to add the process to a node where other processes with the same reference currently reside.
		raNodes := haveReferenceAffinity.Nodes(wantP.Config.Reference, wantP.Config.Domain)
		for _, raNodeid := range raNodes {
			if resources.HasNodeEnough(raNodeid, wantP.Config.LimitCPU, wantP.Config.LimitMemory) {
				nodeid = raNodeid
				break
			}
		}

		// Find the node with the most resources available.
		if len(nodeid) == 0 {
			nodes := resources.FindBestNodes(wantP.Config.LimitCPU, wantP.Config.LimitMemory)
			if len(nodes) > 0 {
				nodeid = nodes[0]
			}
		}

		if len(nodeid) != 0 {
			opStack = append(opStack, processOpAdd{
				nodeid:   nodeid,
				config:   wantP.Config,
				metadata: wantP.Metadata,
				order:    wantP.Order,
			})

			// Consume the resources
			resources.Add(nodeid, wantP.Config.LimitCPU, wantP.Config.LimitMemory)

			reality[pid] = nodeid

			haveReferenceAffinity.Add(wantP.Config.Reference, wantP.Config.Domain, nodeid)
		} else {
			opStack = append(opStack, processOpReject{
				processid: wantP.Config.ProcessID(),
				err:       errNotEnoughResourcesForDeployment,
			})
		}
	}

	return opStack, resources.Map(), reality
}
