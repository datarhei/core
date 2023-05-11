package cluster

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/datarhei/core/v16/cluster/proxy"
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
					c.logger.Debug().Log("shutting down leader loop")
					close(weAreLeaderCh)
					leaderLoop.Wait()
					weAreLeaderCh = nil
				}

				if weAreEmergencyLeaderCh != nil {
					c.logger.Debug().Log("shutting down emergency leader loop")
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

				c.logger.Info().Log("cluster followship acquired")

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
					c.logger.Debug().Log("shutting down follower loop")
					close(weAreFollowerCh)
					followerLoop.Wait()
					weAreFollowerCh = nil
				}

				if weAreEmergencyLeaderCh != nil {
					c.logger.Debug().Log("shutting down emergency leader loop")
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
				c.logger.Info().Log("cluster leadership acquired")

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
					c.logger.Debug().Log("shutting down follower loop")
					close(weAreFollowerCh)
					followerLoop.Wait()
					weAreFollowerCh = nil
				}

				if weAreLeaderCh != nil {
					c.logger.Debug().Log("shutting down leader loop")
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
				c.logger.Info().Log("cluster emergency leadership acquired")

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
			}).Log("failed to transfer leadership attempt, will retry")
		} else {
			c.logger.Info().WithFields(log.Fields{
				"attempt":     i,
				"retry_limit": retryCount,
			}).Log("successfully transferred leadership")

			for {
				c.logger.Debug().Log("waiting for losing leadership")

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
			c.logger.Error().WithError(err).Log("failed to wait for barrier")
			goto WAIT
		}
	}

	// Check if we need to handle initial leadership actions
	if !establishedLeader {
		if err := c.establishLeadership(context.TODO()); err != nil {
			c.logger.Error().WithError(err).Log("failed to establish leadership")
			// Immediately revoke leadership since we didn't successfully
			// establish leadership.
			c.revokeLeadership()

			// attempt to transfer leadership. If successful it is
			// time to leave the leaderLoop since this node is no
			// longer the leader. If leadershipTransfer() fails, we
			// will try to acquire it again after
			// 5 seconds.
			if err := c.leadershipTransfer(); err != nil {
				c.logger.Error().WithError(err).Log("failed to transfer leadership")
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

func (c *cluster) establishLeadership(ctx context.Context) error {
	c.logger.Debug().Log("establishing leadership")

	ctx, cancel := context.WithCancel(ctx)
	c.cancelLeaderShip = cancel

	go c.startRebalance(ctx)

	return nil
}

func (c *cluster) revokeLeadership() {
	c.logger.Debug().Log("revoking leadership")

	c.cancelLeaderShip()
}

func (c *cluster) startRebalance(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.doSynchronize()
			c.doRebalance()
		}
	}
}

var errNotEnoughResources = errors.New("no node with enough resources is available")
var errNotEnoughResourcesForRebalancing = errors.New("no other node to move the process to is available")
var errNoLimitsDefined = errors.New("no process limits are defined")

type processOpDelete struct {
	nodeid    string
	processid string
}

type processOpMove struct {
	fromNodeid string
	toNodeid   string
	config     *app.Config
}

type processOpStart struct {
	nodeid    string
	processid string
}

type processOpAdd struct {
	nodeid string
	config *app.Config
}

type processOpReject struct {
	processid string
	err       error
}

type processOpSkip struct {
	nodeid    string
	processid string
	err       error
}

func (c *cluster) applyOpStack(stack []interface{}) {
	for _, op := range stack {
		switch v := op.(type) {
		case processOpAdd:
			err := c.proxy.ProcessAdd(v.nodeid, v.config)
			if err != nil {
				c.logger.Info().WithError(err).WithFields(log.Fields{
					"processid": v.config.ID,
					"nodeid":    v.nodeid,
				}).Log("Adding process failed")
				break
			}
			err = c.proxy.ProcessStart(v.nodeid, v.config.ID)
			if err != nil {
				c.logger.Info().WithError(err).WithFields(log.Fields{
					"processid": v.config.ID,
					"nodeid":    v.nodeid,
				}).Log("Starting process failed")
				break
			}
			c.logger.Info().WithFields(log.Fields{
				"processid": v.config.ID,
				"nodeid":    v.nodeid,
			}).Log("Adding process")
		case processOpDelete:
			err := c.proxy.ProcessDelete(v.nodeid, v.processid)
			if err != nil {
				c.logger.Info().WithError(err).WithFields(log.Fields{
					"processid": v.processid,
					"nodeid":    v.nodeid,
				}).Log("Removing process failed")
				break
			}
			c.logger.Info().WithFields(log.Fields{
				"processid": v.processid,
				"nodeid":    v.nodeid,
			}).Log("Removing process")
		case processOpMove:
			err := c.proxy.ProcessAdd(v.toNodeid, v.config)
			if err != nil {
				c.logger.Info().WithError(err).WithFields(log.Fields{
					"processid":  v.config.ID,
					"fromnodeid": v.fromNodeid,
					"tonodeid":   v.toNodeid,
				}).Log("Moving process, adding process failed")
				break
			}
			err = c.proxy.ProcessDelete(v.fromNodeid, v.config.ID)
			if err != nil {
				c.logger.Info().WithError(err).WithFields(log.Fields{
					"processid":  v.config.ID,
					"fromnodeid": v.fromNodeid,
					"tonodeid":   v.toNodeid,
				}).Log("Moving process, removing process failed")
				break
			}
			err = c.proxy.ProcessStart(v.toNodeid, v.config.ID)
			if err != nil {
				c.logger.Info().WithError(err).WithFields(log.Fields{
					"processid":  v.config.ID,
					"fromnodeid": v.fromNodeid,
					"tonodeid":   v.toNodeid,
				}).Log("Moving process, starting process failed")
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
				}).Log("Starting process failed")
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

func (c *cluster) doSynchronize() {
	want := c.store.ProcessList()
	have := c.proxy.ProcessList()
	resources := c.proxy.Resources()

	opStack := synchronize(want, have, resources)

	c.applyOpStack(opStack)
}

func (c *cluster) doRebalance() {
	have := c.proxy.ProcessList()
	resources := c.proxy.Resources()

	opStack := rebalance(have, resources)

	c.applyOpStack(opStack)
}

// normalizeProcessesAndResources normalizes the CPU and memory consumption of the processes and resources in-place.
func normalizeProcessesAndResources(processes []proxy.ProcessConfig, resources map[string]proxy.NodeResources) {
	maxNCPU := .0
	maxMemTotal := .0

	for _, r := range resources {
		if r.NCPU > maxNCPU {
			maxNCPU = r.NCPU
		}
		if r.MemTotal > maxMemTotal {
			maxMemTotal = r.MemTotal
		}
	}

	for id, r := range resources {
		factor := maxNCPU / r.NCPU
		r.CPU = 100 - (100-r.CPU)/factor

		factor = maxMemTotal / r.MemTotal
		r.Mem = 100 - (100-r.Mem)/factor

		resources[id] = r
	}

	for i, p := range processes {
		r, ok := resources[p.NodeID]
		if !ok {
			p.CPU = 100
			p.Mem = 100
		}

		factor := maxNCPU / r.NCPU
		p.CPU = 100 - (100-p.CPU)/factor

		factor = maxMemTotal / r.MemTotal
		p.Mem = 100 - (100-p.Mem)/factor

		processes[i] = p
	}

	for id, r := range resources {
		r.NCPU = maxNCPU
		r.MemTotal = maxMemTotal

		resources[id] = r
	}
}

// synchronize returns a list of operations in order to adjust the "have" list to the "want" list
// with taking the available resources on each node into account.
func synchronize(want []app.Config, have []proxy.ProcessConfig, resources map[string]proxy.NodeResources) []interface{} {
	normalizeProcessesAndResources(have, resources)

	// A map from the process ID to the process config of the processes
	// we want to be running on the nodes.
	wantMap := map[string]*app.Config{}
	for _, config := range want {
		wantMap[config.ID] = &config
	}

	opStack := []interface{}{}

	// Now we iterate through the processes we actually have running on the nodes
	// and remove them from the wantMap. We also make sure that they are running.
	// If a process is not on the wantMap, it will be deleted from the nodes.
	haveAfterRemove := []proxy.ProcessConfig{}

	for _, p := range have {
		if _, ok := wantMap[p.Config.ID]; !ok {
			opStack = append(opStack, processOpDelete{
				nodeid:    p.NodeID,
				processid: p.Config.ID,
			})

			// Adjust the resources
			r, ok := resources[p.NodeID]
			if ok {
				r.CPU -= p.CPU
				r.Mem -= p.Mem
				resources[p.NodeID] = r
			}

			continue
		}

		delete(wantMap, p.Config.ID)

		if p.Order != "start" {
			opStack = append(opStack, processOpStart{
				nodeid:    p.NodeID,
				processid: p.Config.ID,
			})
		}

		haveAfterRemove = append(haveAfterRemove, p)
	}

	have = haveAfterRemove

	createReferenceAffinityMap(have)

	// A map from the process reference to the node it is running on
	haveReferenceAffinityMap := createReferenceAffinityMap(have)

	// Now all remaining processes in the wantMap must be added to one of the nodes
	for _, config := range wantMap {
		// If a process doesn't have any limits defined, reject that process
		if config.LimitCPU <= 0 || config.LimitMemory <= 0 {
			opStack = append(opStack, processOpReject{
				processid: config.ID,
				err:       errNoLimitsDefined,
			})

			continue
		}

		// Check if there are already processes with the same reference, and if so
		// chose this node. Then check the node if it has enough resources left. If
		// not, then select a node with the most available resources.
		nodeid := ""

		// Try to add the process to a node where other processes with the same
		// reference currently reside.
		if len(config.Reference) != 0 {
			for _, count := range haveReferenceAffinityMap[config.Reference] {
				r := resources[count.nodeid]
				cpu := config.LimitCPU / r.NCPU
				mem := float64(config.LimitMemory) / r.MemTotal * 100

				if r.CPU+cpu < r.CPULimit && r.Mem+mem < r.MemLimit {
					nodeid = count.nodeid
					break
				}
			}
		}

		// Find the node with the most resources available
		if len(nodeid) == 0 {
			for id, r := range resources {
				cpu := config.LimitCPU / r.NCPU
				mem := float64(config.LimitMemory) / float64(r.MemTotal) * 100

				if len(nodeid) == 0 {
					if r.CPU+cpu < r.CPULimit && r.Mem+mem < r.MemLimit {
						nodeid = id
					}

					continue
				}

				if r.CPU+r.Mem < resources[nodeid].CPU+resources[nodeid].Mem {
					nodeid = id
				}
				/*
					if r.CPU < resources[nodeid].CPU && r.Mem < resources[nodeid].Mem {
						nodeid = id
					} else if r.Mem < resources[nodeid].Mem {
						nodeid = id
					} else if r.CPU < resources[nodeid].CPU {
						nodeid = id
					}
				*/
			}
		}

		if len(nodeid) != 0 {
			opStack = append(opStack, processOpAdd{
				nodeid: nodeid,
				config: config,
			})

			// Adjust the resources
			r, ok := resources[nodeid]
			if ok {
				r.CPU += config.LimitCPU / r.NCPU
				r.Mem += float64(config.LimitMemory) / float64(r.MemTotal) * 100
				resources[nodeid] = r
			}
		} else {
			opStack = append(opStack, processOpReject{
				processid: config.ID,
				err:       errNotEnoughResources,
			})
		}
	}

	return opStack
}

type referenceAffinityNodeCount struct {
	nodeid string
	count  uint64
}

func createReferenceAffinityMap(processes []proxy.ProcessConfig) map[string][]referenceAffinityNodeCount {
	referenceAffinityMap := map[string][]referenceAffinityNodeCount{}
	for _, p := range processes {
		if len(p.Config.Reference) == 0 {
			continue
		}

		// Here we count how often a reference is present on a node. When
		// moving processes to a different node, the node with the highest
		// count of same references will be the first candidate.
		found := false
		arr := referenceAffinityMap[p.Config.Reference]
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

		referenceAffinityMap[p.Config.Reference] = arr
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

// rebalance returns a list of operations that will move running processes away from nodes
// that are overloaded.
func rebalance(have []proxy.ProcessConfig, resources map[string]proxy.NodeResources) []interface{} {
	normalizeProcessesAndResources(have, resources)

	// Group the processes by node
	processNodeMap := map[string][]proxy.ProcessConfig{}

	for _, p := range have {
		processNodeMap[p.NodeID] = append(processNodeMap[p.NodeID], p)
	}

	// Sort the processes by their runtime (if they are running) for each node
	for nodeid, processes := range processNodeMap {
		sort.SliceStable(processes, func(a, b int) bool {
			if processes[a].State == "running" {
				if processes[b].State != "running" {
					return false
				}

				return processes[a].Runtime < processes[b].Runtime
			}

			return false
		})

		processNodeMap[nodeid] = processes
	}

	// A map from the process reference to the nodes it is running on
	haveReferenceAffinityMap := createReferenceAffinityMap(have)

	opStack := []interface{}{}

	// Check if any of the nodes is overloaded
	for id, r := range resources {
		// Check if node is overloaded
		if r.CPU < r.CPULimit && r.Mem < r.MemLimit {
			continue
		}

		// Move processes from this noed to another node with enough free resources.
		// The processes are ordered ascending by their runtime.
		processes := processNodeMap[id]
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
				for _, count := range haveReferenceAffinityMap[p.Config.Reference] {
					if count.nodeid == overloadedNodeid {
						continue
					}

					r := resources[count.nodeid]
					if r.CPU+p.CPU < r.CPULimit && r.Mem+p.Mem < r.MemLimit {
						availableNodeid = count.nodeid
						break
					}
				}
			}

			// Find another node with enough resources available
			if len(availableNodeid) == 0 {
				for id, r := range resources {
					if id == overloadedNodeid {
						// Skip the overloaded node
						continue
					}

					if r.CPU+p.CPU < r.CPULimit && r.Mem+p.Mem < r.MemLimit {
						availableNodeid = id
						break
					}
				}
			}

			if len(availableNodeid) == 0 {
				// There's no other node with enough resources to take over this process
				opStack = append(opStack, processOpSkip{
					nodeid:    overloadedNodeid,
					processid: p.Config.ID,
					err:       errNotEnoughResourcesForRebalancing,
				})
				continue
			}

			opStack = append(opStack, processOpMove{
				fromNodeid: overloadedNodeid,
				toNodeid:   availableNodeid,
				config:     p.Config,
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

			// If this node is not anymore overloaded, stop moving processes around
			if r.CPU < r.CPULimit && r.Mem < r.MemLimit {
				break
			}
		}
	}

	return opStack
}
