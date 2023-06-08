package cluster

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/datarhei/core/v16/cluster/proxy"
	"github.com/datarhei/core/v16/cluster/store"
	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/restream/app"
)

const NOTIFY_FOLLOWER = 0
const NOTIFY_LEADER = 1
const NOTIFY_EMERGENCY = 2

func (c *cluster) monitorLeadership() {
	// We use the notify channel we configured Raft with, NOT Raft's
	// leaderCh, which is only notified best-effort. Doing this ensures
	// that we get all notifications in order, which is required for
	// cleanup and to ensure we never run multiple leader loops.
	notifyCh := make(chan int, 10)
	var notifyLoop sync.WaitGroup

	notifyLoop.Add(1)

	go func() {
		raftNotifyCh := c.raftNotifyCh
		raftEmergencyNotifyCh := c.raftEmergencyNotifyCh

		isRaftLeader := false

		notifyCh <- NOTIFY_FOLLOWER

		notifyLoop.Done()

		for {
			select {
			case isEmergencyLeader := <-raftEmergencyNotifyCh:
				if isEmergencyLeader {
					isRaftLeader = false
					notifyCh <- NOTIFY_EMERGENCY
				} else {
					if !isRaftLeader {
						notifyCh <- NOTIFY_FOLLOWER
					}
				}
			case isLeader := <-raftNotifyCh:
				if isLeader {
					isRaftLeader = true
					notifyCh <- NOTIFY_LEADER
				} else {
					isRaftLeader = false
					notifyCh <- NOTIFY_FOLLOWER
				}
			case <-c.shutdownCh:
				return
			}
		}
	}()

	notifyLoop.Wait()

	var weAreLeaderCh chan struct{}
	var weAreEmergencyLeaderCh chan struct{}
	var weAreFollowerCh chan struct{}

	var leaderLoop sync.WaitGroup
	var emergencyLeaderLoop sync.WaitGroup
	var followerLoop sync.WaitGroup

	for {
		select {
		case notification := <-notifyCh:
			if notification == NOTIFY_FOLLOWER {
				if weAreFollowerCh != nil {
					// we are already follower, don't do anything
					continue
				}

				// shutdown any leader and emergency loop
				if weAreLeaderCh != nil {
					c.logger.Debug().Log("Shutting down leader loop")
					close(weAreLeaderCh)
					leaderLoop.Wait()
					weAreLeaderCh = nil
				}

				if weAreEmergencyLeaderCh != nil {
					c.logger.Debug().Log("Shutting down emergency leader loop")
					close(weAreEmergencyLeaderCh)
					emergencyLeaderLoop.Wait()
					weAreEmergencyLeaderCh = nil
				}

				weAreFollowerCh = make(chan struct{})
				followerLoop.Add(1)
				go func(ch chan struct{}) {
					defer followerLoop.Done()
					c.followerLoop(ch)
				}(weAreFollowerCh)

				c.logger.Info().Log("Cluster followship acquired")

				c.leaderLock.Lock()
				c.isRaftLeader = false
				c.isLeader = false
				c.leaderLock.Unlock()
			} else if notification == NOTIFY_LEADER {
				if weAreLeaderCh != nil {
					// we are already leader, don't do anything
					continue
				}

				// shutdown any follower and emergency loop
				if weAreFollowerCh != nil {
					c.logger.Debug().Log("Shutting down follower loop")
					close(weAreFollowerCh)
					followerLoop.Wait()
					weAreFollowerCh = nil
				}

				if weAreEmergencyLeaderCh != nil {
					c.logger.Debug().Log("Shutting down emergency leader loop")
					close(weAreEmergencyLeaderCh)
					emergencyLeaderLoop.Wait()
					weAreEmergencyLeaderCh = nil
				}

				weAreLeaderCh = make(chan struct{})
				leaderLoop.Add(1)
				go func(ch chan struct{}) {
					defer leaderLoop.Done()
					c.leaderLoop(ch, false)
				}(weAreLeaderCh)
				c.logger.Info().Log("Cluster leadership acquired")

				c.leaderLock.Lock()
				c.isRaftLeader = true
				c.isLeader = true
				c.leaderLock.Unlock()
			} else if notification == NOTIFY_EMERGENCY {
				if weAreEmergencyLeaderCh != nil {
					// we are already emergency leader, don't do anything
					continue
				}

				// shutdown any follower and leader loop
				if weAreFollowerCh != nil {
					c.logger.Debug().Log("Shutting down follower loop")
					close(weAreFollowerCh)
					followerLoop.Wait()
					weAreFollowerCh = nil
				}

				if weAreLeaderCh != nil {
					c.logger.Debug().Log("Shutting down leader loop")
					close(weAreLeaderCh)
					leaderLoop.Wait()
					weAreLeaderCh = nil
				}

				weAreEmergencyLeaderCh = make(chan struct{})
				emergencyLeaderLoop.Add(1)
				go func(ch chan struct{}) {
					defer emergencyLeaderLoop.Done()
					c.leaderLoop(ch, true)
				}(weAreEmergencyLeaderCh)
				c.logger.Info().Log("Cluster emergency leadership acquired")

				c.leaderLock.Lock()
				c.isRaftLeader = false
				c.isLeader = true
				c.leaderLock.Unlock()
			}
		case <-c.shutdownCh:
			return
		}
	}
}

// leadershipTransfer tries to transfer the leadership to another node e.g. in order
// to do a graceful shutdown.
func (c *cluster) leadershipTransfer() error {
	retryCount := 3
	for i := 0; i < retryCount; i++ {
		err := c.raft.LeadershipTransfer()
		if err != nil {
			c.logger.Error().WithError(err).WithFields(log.Fields{
				"attempt":     i,
				"retry_limit": retryCount,
			}).Log("Transfer leadership attempt, will retry")
		} else {
			c.logger.Info().WithFields(log.Fields{
				"attempt":     i,
				"retry_limit": retryCount,
			}).Log("Successfully transferred leadership")

			for {
				c.logger.Debug().Log("Waiting for losing leadership")

				time.Sleep(50 * time.Millisecond)

				c.leaderLock.Lock()
				isLeader := c.isRaftLeader
				c.leaderLock.Unlock()

				if !isLeader {
					break
				}
			}

			return nil
		}
	}

	return fmt.Errorf("failed to transfer leadership in %d attempts", retryCount)
}

// leaderLoop runs as long as we are the leader to run various maintenance activities
// https://github.com/hashicorp/consul/blob/44b39240a86bc94ddc67bc105286ab450bd869a9/agent/consul/leader.go#L146
func (c *cluster) leaderLoop(stopCh chan struct{}, emergency bool) {
	establishedLeader := false

RECONCILE:
	// Setup a reconciliation timer
	interval := time.After(10 * time.Second)

	// Apply a raft barrier to ensure our FSM is caught up
	if !emergency {
		err := c.raft.Barrier(time.Minute)
		if err != nil {
			c.logger.Error().WithError(err).Log("Wait for barrier")
			goto WAIT
		}
	}

	// Check if we need to handle initial leadership actions
	if !establishedLeader {
		if err := c.establishLeadership(context.TODO(), emergency); err != nil {
			c.logger.Error().WithError(err).Log("Establish leadership")
			// Immediately revoke leadership since we didn't successfully
			// establish leadership.
			c.revokeLeadership()

			// attempt to transfer leadership. If successful it is
			// time to leave the leaderLoop since this node is no
			// longer the leader. If leadershipTransfer() fails, we
			// will try to acquire it again after
			// 5 seconds.
			if err := c.leadershipTransfer(); err != nil {
				c.logger.Error().WithError(err).Log("Transfer leadership")
				interval = time.After(5 * time.Second)
				goto WAIT
			}
			return
		}
		establishedLeader = true
		defer c.revokeLeadership()
	}

WAIT:
	// Poll the stop channel to give it priority so we don't waste time
	// trying to perform the other operations if we have been asked to shut
	// down.
	select {
	case <-stopCh:
		return
	default:
	}

	// Periodically reconcile as long as we are the leader
	for {
		select {
		case <-stopCh:
			return
		case <-c.shutdownCh:
			return
		case <-interval:
			goto RECONCILE
		}
	}
}

func (c *cluster) establishLeadership(ctx context.Context, emergency bool) error {
	c.logger.Debug().WithField("emergency", emergency).Log("Establishing leadership")

	ctx, cancel := context.WithCancel(ctx)
	c.cancelLeaderShip = cancel

	go c.startSynchronizeAndRebalance(ctx, c.syncInterval, emergency)

	return nil
}

func (c *cluster) revokeLeadership() {
	c.logger.Debug().Log("Revoking leadership")

	c.cancelLeaderShip()
}

// startSynchronizeAndRebalance synchronizes and rebalances the processes in a given interval. Synchronizing
// takes care that all processes in the cluster DB are running on one node. It writes the process->node mapping
// to the cluster DB such that when a new leader gets elected it knows where which process should be running.
// This is checked against the actual state. If a node is not reachable, the leader still knows which processes
// should be running on that node. For a certain duration (nodeRecoverTimeout) this is tolerated in case the
// node comes back. If not, the processes will be distributed to the remaining nodes. The actual observed state
// is stored back into the cluster DB.
//
// It follows the rebalancing which takes care that processes are taken from overloaded nodes. In each iteration
// only one process is taken away from a node. If a node is not reachable, its processes will be not part of the
// rebalancing and no attempt will be made to move processes from and to that node.
//
// All this happens if there's a leader. If there's no leader election possible, the node goes into the
// emergency leadership mode after a certain duration (emergencyLeaderTimeout). The synchronization phase will
// happen based on the last known list of processes from the cluster DB. Until nodeRecoverTimeout is reached,
// process that would run on unreachable nodes will not be moved to the node. After that, all processes will
// end on the node, but only if there are enough resources. Not to bog down the node. Rebalancing will be
// disabled.
//
// The goal of synchronizing and rebalancing is to make as little moves as possible and to be tolerant for
// a while if a node is not reachable.
func (c *cluster) startSynchronizeAndRebalance(ctx context.Context, interval time.Duration, emergency bool) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.doSynchronize(emergency)

			if !emergency {
				c.doRebalance(emergency)
			}
		}
	}
}

var errNotEnoughResources = errors.New("no node with enough resources is available")
var errNotEnoughResourcesForRebalancing = errors.New("no other node to move the process to is available")
var errNoLimitsDefined = errors.New("no process limits are defined")

type processOpDelete struct {
	nodeid    string
	processid app.ProcessID
}

type processOpMove struct {
	fromNodeid string
	toNodeid   string
	config     *app.Config
	metadata   map[string]interface{}
}

type processOpStart struct {
	nodeid    string
	processid app.ProcessID
}

type processOpAdd struct {
	nodeid   string
	config   *app.Config
	metadata map[string]interface{}
}

type processOpUpdate struct {
	nodeid    string
	processid app.ProcessID
	config    *app.Config
	metadata  map[string]interface{}
}

type processOpReject struct {
	processid app.ProcessID
	err       error
}

type processOpSkip struct {
	nodeid    string
	processid app.ProcessID
	err       error
}

func (c *cluster) applyOpStack(stack []interface{}) {
	for _, op := range stack {
		switch v := op.(type) {
		case processOpAdd:
			err := c.proxy.ProcessAdd(v.nodeid, v.config, v.metadata)
			if err != nil {
				c.logger.Info().WithError(err).WithFields(log.Fields{
					"processid": v.config.ProcessID(),
					"nodeid":    v.nodeid,
				}).Log("Adding process")
				break
			}

			err = c.proxy.ProcessStart(v.nodeid, v.config.ProcessID())
			if err != nil {
				c.logger.Info().WithError(err).WithFields(log.Fields{
					"processid": v.config.ID,
					"nodeid":    v.nodeid,
				}).Log("Starting process")
				break
			}
			c.logger.Info().WithFields(log.Fields{
				"processid": v.config.ID,
				"nodeid":    v.nodeid,
			}).Log("Adding process")
		case processOpUpdate:
			err := c.proxy.ProcessUpdate(v.nodeid, v.processid, v.config, v.metadata)
			if err != nil {
				c.logger.Info().WithError(err).WithFields(log.Fields{
					"processid": v.config.ID,
					"nodeid":    v.nodeid,
				}).Log("Updating process")
				break
			}

			c.logger.Info().WithFields(log.Fields{
				"processid": v.config.ID,
				"nodeid":    v.nodeid,
			}).Log("Updating process")
		case processOpDelete:
			err := c.proxy.ProcessDelete(v.nodeid, v.processid)
			if err != nil {
				c.logger.Info().WithError(err).WithFields(log.Fields{
					"processid": v.processid,
					"nodeid":    v.nodeid,
				}).Log("Removing process")
				break
			}

			c.logger.Info().WithFields(log.Fields{
				"processid": v.processid,
				"nodeid":    v.nodeid,
			}).Log("Removing process")
		case processOpMove:
			err := c.proxy.ProcessAdd(v.toNodeid, v.config, v.metadata)
			if err != nil {
				c.logger.Info().WithError(err).WithFields(log.Fields{
					"processid":  v.config.ID,
					"fromnodeid": v.fromNodeid,
					"tonodeid":   v.toNodeid,
				}).Log("Moving process, adding process")
				break
			}

			err = c.proxy.ProcessDelete(v.fromNodeid, v.config.ProcessID())
			if err != nil {
				c.logger.Info().WithError(err).WithFields(log.Fields{
					"processid":  v.config.ID,
					"fromnodeid": v.fromNodeid,
					"tonodeid":   v.toNodeid,
				}).Log("Moving process, removing process")
				break
			}

			err = c.proxy.ProcessStart(v.toNodeid, v.config.ProcessID())
			if err != nil {
				c.logger.Info().WithError(err).WithFields(log.Fields{
					"processid":  v.config.ID,
					"fromnodeid": v.fromNodeid,
					"tonodeid":   v.toNodeid,
				}).Log("Moving process, starting process")
				break
			}

			c.logger.Info().WithFields(log.Fields{
				"processid":  v.config.ID,
				"fromnodeid": v.fromNodeid,
				"tonodeid":   v.toNodeid,
			}).Log("Moving process")
		case processOpStart:
			err := c.proxy.ProcessStart(v.nodeid, v.processid)
			if err != nil {
				c.logger.Info().WithError(err).WithFields(log.Fields{
					"processid": v.processid,
					"nodeid":    v.nodeid,
				}).Log("Starting process")
				break
			}

			c.logger.Info().WithFields(log.Fields{
				"processid": v.processid,
				"nodeid":    v.nodeid,
			}).Log("Starting process")
		case processOpReject:
			c.logger.Warn().WithError(v.err).WithField("processid", v.processid).Log("Process rejected")
		case processOpSkip:
			c.logger.Warn().WithError(v.err).WithFields(log.Fields{
				"nodeid":    v.nodeid,
				"processid": v.processid,
			}).Log("Process skipped")
		default:
			c.logger.Warn().Log("Unknown operation on stack: %+v", v)
		}
	}
}

func (c *cluster) doSynchronize(emergency bool) {
	wish := c.store.GetProcessNodeMap()
	want := c.store.ProcessList()
	have := c.proxy.ListProcesses()
	nodes := c.proxy.ListNodes()

	nodesMap := map[string]proxy.NodeAbout{}

	for _, node := range nodes {
		about := node.About()
		nodesMap[about.ID] = about
	}

	c.logger.Debug().WithFields(log.Fields{
		"want":  want,
		"have":  have,
		"nodes": nodesMap,
	}).Log("Synchronize")

	opStack, _, reality := synchronize(wish, want, have, nodesMap, c.nodeRecoverTimeout)

	if !emergency {
		cmd := &store.Command{
			Operation: store.OpSetProcessNodeMap,
			Data: store.CommandSetProcessNodeMap{
				Map: reality,
			},
		}

		c.applyCommand(cmd)
	}

	c.applyOpStack(opStack)
}

func (c *cluster) doRebalance(emergency bool) {
	have := c.proxy.ListProcesses()
	nodes := c.proxy.ListNodes()

	nodesMap := map[string]proxy.NodeAbout{}

	for _, node := range nodes {
		about := node.About()
		nodesMap[about.ID] = about
	}

	c.logger.Debug().WithFields(log.Fields{
		"have":  have,
		"nodes": nodes,
	}).Log("Rebalance")

	opStack, _ := rebalance(have, nodesMap)

	c.applyOpStack(opStack)
}

// synchronize returns a list of operations in order to adjust the "have" list to the "want" list
// with taking the available resources on each node into account.
func synchronize(wish map[string]string, want []store.Process, have []proxy.Process, nodes map[string]proxy.NodeAbout, nodeRecoverTimeout time.Duration) ([]interface{}, map[string]proxy.NodeResources, map[string]string) {
	resources := map[string]proxy.NodeResources{}
	for nodeid, about := range nodes {
		resources[nodeid] = about.Resources
	}

	// A map same as wish, but reflecting the actual situation.
	reality := map[string]string{}

	// A map from the process ID to the process config of the processes
	// we want to be running on the nodes.
	wantMap := map[string]store.Process{}
	for _, process := range want {
		pid := process.Config.ProcessID().String()
		wantMap[pid] = process
	}

	opStack := []interface{}{}

	// Now we iterate through the processes we actually have running on the nodes
	// and remove them from the wantMap. We also make sure that they are running.
	// If a process cannot be found on the wantMap, it will be deleted from the nodes.
	haveAfterRemove := []proxy.Process{}

	for _, haveP := range have {
		pid := haveP.Config.ProcessID().String()
		if wantP, ok := wantMap[pid]; !ok {
			// The process is not on the wantMap. Delete it and adjust the resources.
			opStack = append(opStack, processOpDelete{
				nodeid:    haveP.NodeID,
				processid: haveP.Config.ProcessID(),
			})

			r, ok := resources[haveP.NodeID]
			if ok {
				r.CPU -= haveP.CPU
				r.Mem -= haveP.Mem
				resources[haveP.NodeID] = r
			}

			continue
		} else {
			// The process is on the wantMap. Update the process if the configuration differ.
			if !wantP.Config.Equal(haveP.Config) {
				opStack = append(opStack, processOpUpdate{
					nodeid:    haveP.NodeID,
					processid: haveP.Config.ProcessID(),
					config:    wantP.Config,
					metadata:  wantP.Metadata,
				})
			}
		}

		delete(wantMap, pid)
		reality[pid] = haveP.NodeID

		if haveP.Order != "start" {
			opStack = append(opStack, processOpStart{
				nodeid:    haveP.NodeID,
				processid: haveP.Config.ProcessID(),
			})
		}

		haveAfterRemove = append(haveAfterRemove, haveP)
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
				if time.Since(node.LastContact) <= nodeRecoverTimeout {
					reality[pid] = nodeid
					delete(wantMap, pid)
				}
			}
		}
	}

	// The wantMap now contains only those processes that need to be installed on a node.

	// A map from the process reference to the node it is running on.
	haveReferenceAffinityMap := createReferenceAffinityMap(have)

	// Now all remaining processes in the wantMap must be added to one of the nodes.
	for pid, process := range wantMap {
		// If a process doesn't have any limits defined, reject that process
		if process.Config.LimitCPU <= 0 || process.Config.LimitMemory <= 0 {
			opStack = append(opStack, processOpReject{
				processid: process.Config.ProcessID(),
				err:       errNoLimitsDefined,
			})

			continue
		}

		// Check if there are already processes with the same reference, and if so
		// choose this node. Then check the node if it has enough resources left. If
		// not, then select a node with the most available resources.
		nodeid := ""

		// Try to add the process to a node where other processes with the same
		// reference currently reside.
		if len(process.Config.Reference) != 0 {
			for _, count := range haveReferenceAffinityMap[process.Config.Reference+"@"+process.Config.Domain] {
				r := resources[count.nodeid]
				cpu := process.Config.LimitCPU
				mem := process.Config.LimitMemory

				if r.CPU+cpu < r.CPULimit && r.Mem+mem < r.MemLimit && !r.IsThrottling {
					nodeid = count.nodeid
					break
				}
			}
		}

		// Find the node with the most resources available
		if len(nodeid) == 0 {
			for id, r := range resources {
				cpu := process.Config.LimitCPU
				mem := process.Config.LimitMemory

				if len(nodeid) == 0 {
					if r.CPU+cpu < r.CPULimit && r.Mem+mem < r.MemLimit && !r.IsThrottling {
						nodeid = id
					}

					continue
				}

				if r.CPU < resources[nodeid].CPU && r.Mem <= resources[nodeid].Mem {
					nodeid = id
				}
			}
		}

		if len(nodeid) != 0 {
			opStack = append(opStack, processOpAdd{
				nodeid:   nodeid,
				config:   process.Config,
				metadata: process.Metadata,
			})

			// Adjust the resources
			r, ok := resources[nodeid]
			if ok {
				r.CPU += process.Config.LimitCPU
				r.Mem += process.Config.LimitMemory
				resources[nodeid] = r
			}

			reality[pid] = nodeid
		} else {
			opStack = append(opStack, processOpReject{
				processid: process.Config.ProcessID(),
				err:       errNotEnoughResources,
			})
		}
	}

	return opStack, resources, reality
}

type referenceAffinityNodeCount struct {
	nodeid string
	count  uint64
}

// createReferenceAffinityMap returns a map of references (per domain) to an array of nodes this reference
// is found on and their count. The array is sorted by the count, the highest first.
func createReferenceAffinityMap(processes []proxy.Process) map[string][]referenceAffinityNodeCount {
	referenceAffinityMap := map[string][]referenceAffinityNodeCount{}
	for _, p := range processes {
		if len(p.Config.Reference) == 0 {
			continue
		}

		ref := p.Config.Reference + "@" + p.Config.Domain

		// Here we count how often a reference is present on a node. When
		// moving processes to a different node, the node with the highest
		// count of same references will be the first candidate.
		found := false
		arr := referenceAffinityMap[ref]
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

		referenceAffinityMap[ref] = arr
	}

	// Sort every reference count in decreasing order for each reference
	for ref, count := range referenceAffinityMap {
		sort.SliceStable(count, func(a, b int) bool {
			return count[a].count > count[b].count
		})

		referenceAffinityMap[ref] = count
	}

	return referenceAffinityMap
}

// rebalance returns a list of operations that will move running processes away from nodes that are overloaded.
func rebalance(have []proxy.Process, nodes map[string]proxy.NodeAbout) ([]interface{}, map[string]proxy.NodeResources) {
	resources := map[string]proxy.NodeResources{}
	for nodeid, about := range nodes {
		resources[nodeid] = about.Resources
	}

	// Group the processes by node and sort them
	nodeProcessMap := createNodeProcessMap(have)

	// A map from the process reference to the nodes it is running on
	haveReferenceAffinityMap := createReferenceAffinityMap(have)

	opStack := []interface{}{}

	// Check if any of the nodes is overloaded
	for id, node := range nodes {
		r := node.Resources

		// Check if node is overloaded
		if r.CPU < r.CPULimit && r.Mem < r.MemLimit && !r.IsThrottling {
			continue
		}

		// Move processes from this noed to another node with enough free resources.
		// The processes are ordered ascending by their runtime.
		processes := nodeProcessMap[id]
		if len(processes) == 0 {
			// If there are no processes on that node, we can't do anything
			continue
		}

		overloadedNodeid := id

		for i, p := range processes {
			if p.State != "running" {
				// We consider only currently running processes
				continue
			}

			availableNodeid := ""

			// Try to move the process to a node where other processes with the same
			// reference currently reside.
			if len(p.Config.Reference) != 0 {
				for _, count := range haveReferenceAffinityMap[p.Config.Reference+"@"+p.Config.Domain] {
					if count.nodeid == overloadedNodeid {
						continue
					}

					r := resources[count.nodeid]
					if r.CPU+p.CPU < r.CPULimit && r.Mem+p.Mem < r.MemLimit && !r.IsThrottling {
						availableNodeid = count.nodeid
						break
					}
				}
			}

			// Find another node with enough resources available
			if len(availableNodeid) == 0 {
				for id, node := range nodes {
					if id == overloadedNodeid {
						// Skip the overloaded node
						continue
					}

					r := node.Resources

					if r.CPU+p.CPU < r.CPULimit && r.Mem+p.Mem < r.MemLimit && !r.IsThrottling {
						availableNodeid = id
						break
					}
				}
			}

			if len(availableNodeid) == 0 {
				// There's no other node with enough resources to take over this process
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
			})

			// Adjust the process
			p.NodeID = availableNodeid
			processes[i] = p

			// Adjust the resources
			r = resources[availableNodeid]
			r.CPU += p.CPU
			r.Mem += p.Mem
			resources[availableNodeid] = r

			r = resources[overloadedNodeid]
			r.CPU -= p.CPU
			r.Mem -= p.Mem
			resources[overloadedNodeid] = r

			// Move only one process at a time
			break
		}
	}

	return opStack, resources
}

// createNodeProcessMap takes a list of processes and groups them by the nodeid they
// are running on. Each group gets sorted by their preference to be moved somewhere
// else, decreasing.
func createNodeProcessMap(processes []proxy.Process) map[string][]proxy.Process {
	nodeProcessMap := map[string][]proxy.Process{}

	for _, p := range processes {
		nodeProcessMap[p.NodeID] = append(nodeProcessMap[p.NodeID], p)
	}

	// Sort the processes by their runtime (if they are running) for each node
	for nodeid, processes := range nodeProcessMap {
		sort.SliceStable(processes, func(a, b int) bool {
			if processes[a].State == "running" {
				if processes[b].State != "running" {
					return false
				}

				return processes[a].Runtime < processes[b].Runtime
			}

			return false
		})

		nodeProcessMap[nodeid] = processes
	}

	return nodeProcessMap
}
