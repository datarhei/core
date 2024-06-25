package cluster

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/datarhei/core/v16/cluster/proxy"
	"github.com/datarhei/core/v16/cluster/store"
	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/maps"
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
func (c *cluster) leadershipTransfer(id string) error {
	if id == c.nodeID {
		return nil
	}

	retryCount := 3
	for i := 0; i < retryCount; i++ {
		err := c.raft.LeadershipTransfer(id)
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
			if err := c.leadershipTransfer(""); err != nil {
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
	c.logger.Info().WithField("emergency", emergency).Log("Establishing leadership")

	ctx, cancel := context.WithCancel(ctx)
	c.cancelLeaderShip = cancel

	go c.synchronizeAndRebalance(ctx, c.syncInterval, emergency)

	if !emergency {
		go c.clearLocks(ctx, time.Minute)
	}

	return nil
}

func (c *cluster) revokeLeadership() {
	c.logger.Info().Log("Revoking leadership")

	if c.cancelLeaderShip != nil {
		c.cancelLeaderShip()
		c.cancelLeaderShip = nil
	}
}

// synchronizeAndRebalance synchronizes and rebalances the processes in a given interval. Synchronizing
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
func (c *cluster) synchronizeAndRebalance(ctx context.Context, interval time.Duration, emergency bool) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	term := uint64(0)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !emergency {
				if ok, _ := c.IsDegraded(); ok {
					break
				}

				c.doSynchronize(emergency, term)
				c.doRebalance(emergency, term)
				c.doRelocate(emergency, term)
			} else {
				c.doSynchronize(emergency, term)
			}
		}

		term++
	}
}

func (c *cluster) clearLocks(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			locks := c.ListLocks()
			hasExpiredLocks := false

			for _, validUntil := range locks {
				if time.Now().Before(validUntil) {
					continue
				}

				hasExpiredLocks = true
				break
			}

			if hasExpiredLocks {
				c.logger.Debug().Log("Clearing locks")
				c.applyCommand(&store.Command{
					Operation: store.OpClearLocks,
					Data:      &store.CommandClearLocks{},
				})
			}
		}
	}
}

var errNotEnoughResourcesForDeployment = errors.New("no node with enough resources for deployment is available")
var errNotEnoughResourcesForRebalancing = errors.New("no node with enough resources for rebalancing is available")
var errNotEnoughResourcesForRelocating = errors.New("no node with enough resources for relocating is available")
var errNoLimitsDefined = errors.New("this process has no limits defined")

type processOpDelete struct {
	nodeid    string
	processid app.ProcessID
}

type processOpMove struct {
	fromNodeid string
	toNodeid   string
	config     *app.Config
	metadata   map[string]interface{}
	order      string
}

type processOpStart struct {
	nodeid    string
	processid app.ProcessID
}

type processOpStop struct {
	nodeid    string
	processid app.ProcessID
}

type processOpAdd struct {
	nodeid   string
	config   *app.Config
	metadata map[string]interface{}
	order    string
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

type processOpError struct {
	processid app.ProcessID
	err       error
}

func (c *cluster) applyOpStack(stack []interface{}, term uint64) []processOpError {
	errors := []processOpError{}

	logger := c.logger.WithField("term", term)

	for _, op := range stack {
		switch v := op.(type) {
		case processOpAdd:
			err := c.proxy.AddProcess(v.nodeid, v.config, v.metadata)
			if err != nil {
				errors = append(errors, processOpError{
					processid: v.config.ProcessID(),
					err:       err,
				})
				logger.Info().WithError(err).WithFields(log.Fields{
					"processid": v.config.ProcessID(),
					"nodeid":    v.nodeid,
				}).Log("Adding process")
				break
			}

			if v.order == "start" {
				err = c.proxy.CommandProcess(v.nodeid, v.config.ProcessID(), "start")
				if err != nil {
					errors = append(errors, processOpError{
						processid: v.config.ProcessID(),
						err:       err,
					})
					logger.Info().WithError(err).WithFields(log.Fields{
						"processid": v.config.ProcessID(),
						"nodeid":    v.nodeid,
					}).Log("Starting process")
					break
				}
			}

			errors = append(errors, processOpError{
				processid: v.config.ProcessID(),
				err:       nil,
			})

			logger.Info().WithFields(log.Fields{
				"processid": v.config.ProcessID(),
				"nodeid":    v.nodeid,
			}).Log("Adding process")
		case processOpUpdate:
			err := c.proxy.UpdateProcess(v.nodeid, v.processid, v.config, v.metadata)
			if err != nil {
				errors = append(errors, processOpError{
					processid: v.processid,
					err:       err,
				})
				logger.Info().WithError(err).WithFields(log.Fields{
					"processid": v.processid,
					"nodeid":    v.nodeid,
				}).Log("Updating process")
				break
			}

			errors = append(errors, processOpError{
				processid: v.processid,
				err:       nil,
			})

			logger.Info().WithFields(log.Fields{
				"processid": v.config.ProcessID(),
				"nodeid":    v.nodeid,
			}).Log("Updating process")
		case processOpDelete:
			err := c.proxy.DeleteProcess(v.nodeid, v.processid)
			if err != nil {
				errors = append(errors, processOpError{
					processid: v.processid,
					err:       err,
				})
				logger.Info().WithError(err).WithFields(log.Fields{
					"processid": v.processid,
					"nodeid":    v.nodeid,
				}).Log("Removing process")
				break
			}

			errors = append(errors, processOpError{
				processid: v.processid,
				err:       nil,
			})

			logger.Info().WithFields(log.Fields{
				"processid": v.processid,
				"nodeid":    v.nodeid,
			}).Log("Removing process")
		case processOpMove:
			err := c.proxy.AddProcess(v.toNodeid, v.config, v.metadata)
			if err != nil {
				errors = append(errors, processOpError{
					processid: v.config.ProcessID(),
					err:       err,
				})
				logger.Info().WithError(err).WithFields(log.Fields{
					"processid":  v.config.ProcessID(),
					"fromnodeid": v.fromNodeid,
					"tonodeid":   v.toNodeid,
				}).Log("Moving process, adding process")
				break
			}

			err = c.proxy.DeleteProcess(v.fromNodeid, v.config.ProcessID())
			if err != nil {
				errors = append(errors, processOpError{
					processid: v.config.ProcessID(),
					err:       err,
				})
				logger.Info().WithError(err).WithFields(log.Fields{
					"processid":  v.config.ProcessID(),
					"fromnodeid": v.fromNodeid,
					"tonodeid":   v.toNodeid,
				}).Log("Moving process, removing process")
				break
			}

			if v.order == "start" {
				err = c.proxy.CommandProcess(v.toNodeid, v.config.ProcessID(), "start")
				if err != nil {
					errors = append(errors, processOpError{
						processid: v.config.ProcessID(),
						err:       err,
					})
					logger.Info().WithError(err).WithFields(log.Fields{
						"processid":  v.config.ProcessID(),
						"fromnodeid": v.fromNodeid,
						"tonodeid":   v.toNodeid,
					}).Log("Moving process, starting process")
					break
				}
			}

			errors = append(errors, processOpError{
				processid: v.config.ProcessID(),
				err:       nil,
			})

			logger.Info().WithFields(log.Fields{
				"processid":  v.config.ProcessID(),
				"fromnodeid": v.fromNodeid,
				"tonodeid":   v.toNodeid,
			}).Log("Moving process")
		case processOpStart:
			err := c.proxy.CommandProcess(v.nodeid, v.processid, "start")
			if err != nil {
				errors = append(errors, processOpError{
					processid: v.processid,
					err:       err,
				})
				logger.Info().WithError(err).WithFields(log.Fields{
					"processid": v.processid,
					"nodeid":    v.nodeid,
				}).Log("Starting process")
				break
			}

			errors = append(errors, processOpError{
				processid: v.processid,
				err:       nil,
			})

			logger.Info().WithFields(log.Fields{
				"processid": v.processid,
				"nodeid":    v.nodeid,
			}).Log("Starting process")
		case processOpStop:
			err := c.proxy.CommandProcess(v.nodeid, v.processid, "stop")
			if err != nil {
				errors = append(errors, processOpError{
					processid: v.processid,
					err:       err,
				})
				logger.Info().WithError(err).WithFields(log.Fields{
					"processid": v.processid,
					"nodeid":    v.nodeid,
				}).Log("Stopping process")
				break
			}

			errors = append(errors, processOpError{
				processid: v.processid,
				err:       nil,
			})

			logger.Info().WithFields(log.Fields{
				"processid": v.processid,
				"nodeid":    v.nodeid,
			}).Log("Stopping process")
		case processOpReject:
			errors = append(errors, processOpError(v))
			logger.Warn().WithError(v.err).WithField("processid", v.processid).Log("Process rejected")
		case processOpSkip:
			errors = append(errors, processOpError{
				processid: v.processid,
				err:       v.err,
			})
			logger.Warn().WithError(v.err).WithFields(log.Fields{
				"nodeid":    v.nodeid,
				"processid": v.processid,
			}).Log("Process skipped")
		case processOpError:
			errors = append(errors, v)
		default:
			logger.Warn().Log("Unknown operation on stack: %+v", v)
		}
	}

	return errors
}

func (c *cluster) doSynchronize(emergency bool, term uint64) {
	wish := c.store.GetProcessNodeMap()
	want := c.store.ListProcesses()
	storeNodes := c.store.ListNodes()
	have := c.proxy.ListProxyProcesses()
	nodes := c.proxy.ListNodes()

	logger := c.logger.WithField("term", term)

	logger.Debug().WithField("emergency", emergency).Log("Synchronizing")

	nodesMap := map[string]proxy.NodeAbout{}

	for _, node := range nodes {
		about := node.About()

		if storeNode, hasStoreNode := storeNodes[about.ID]; hasStoreNode {
			about.State = storeNode.State
		}

		nodesMap[about.ID] = about
	}

	logger.Debug().WithFields(log.Fields{
		"want":  want,
		"have":  have,
		"nodes": nodesMap,
	}).Log("Synchronize")

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

	errors := c.applyOpStack(opStack, term)

	if !emergency {
		for _, e := range errors {
			// Only apply the command if the error is different.
			process, err := c.store.GetProcess(e.processid)
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

func (c *cluster) doRebalance(emergency bool, term uint64) {
	if emergency {
		// Don't rebalance in emergency mode.
		return
	}

	logger := c.logger.WithField("term", term)

	logger.Debug().WithField("emergency", emergency).Log("Rebalancing")

	storeNodes := c.store.ListNodes()
	have := c.proxy.ListProxyProcesses()
	nodes := c.proxy.ListNodes()

	nodesMap := map[string]proxy.NodeAbout{}

	for _, node := range nodes {
		about := node.About()

		if storeNode, hasStoreNode := storeNodes[about.ID]; hasStoreNode {
			about.State = storeNode.State
		}

		nodesMap[about.ID] = about
	}

	logger.Debug().WithFields(log.Fields{
		"have":  have,
		"nodes": nodesMap,
	}).Log("Rebalance")

	opStack, _ := rebalance(have, nodesMap)

	errors := c.applyOpStack(opStack, term)

	for _, e := range errors {
		// Only apply the command if the error is different.
		process, err := c.store.GetProcess(e.processid)
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

func (c *cluster) doRelocate(emergency bool, term uint64) {
	if emergency {
		// Don't relocate in emergency mode.
		return
	}

	logger := c.logger.WithField("term", term)

	logger.Debug().WithField("emergency", emergency).Log("Relocating")

	relocateMap := c.store.GetProcessRelocateMap()
	storeNodes := c.store.ListNodes()
	have := c.proxy.ListProxyProcesses()
	nodes := c.proxy.ListNodes()

	nodesMap := map[string]proxy.NodeAbout{}

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
		process, err := c.store.GetProcess(e.processid)
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

	c.applyCommand(&store.Command{
		Operation: store.OpUnsetRelocateProcess,
		Data:      cmd,
	})
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
func synchronize(wish map[string]string, want []store.Process, have []proxy.Process, nodes map[string]proxy.NodeAbout, nodeRecoverTimeout time.Duration) ([]interface{}, map[string]proxy.NodeResources, map[string]string) {
	resources := NewResources(nodes)

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
	haveAfterRemove := []proxy.Process{}
	wantOrderStart := []proxy.Process{}

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
				if node.State != "disconnected" {
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

type resources struct {
	nodes   map[string]proxy.NodeResources
	blocked map[string]struct{}
}

func NewResources(nodes map[string]proxy.NodeAbout) *resources {
	r := &resources{
		nodes:   map[string]proxy.NodeResources{},
		blocked: map[string]struct{}{},
	}

	for nodeid, about := range nodes {
		r.nodes[nodeid] = about.Resources
		if about.State != "connected" {
			r.blocked[nodeid] = struct{}{}
		}
	}

	return r
}

// HasNodeEnough returns whether a node has enough resources available for the
// requested cpu and memory consumption.
func (r *resources) HasNodeEnough(nodeid string, cpu float64, mem uint64) bool {
	res, hasNode := r.nodes[nodeid]
	if !hasNode {
		return false
	}

	if _, hasNode := r.blocked[nodeid]; hasNode {
		return false
	}

	if res.CPU+cpu < res.CPULimit && res.Mem+mem < res.MemLimit && !res.IsThrottling {
		return true
	}

	return false
}

// FindBestNodes returns an array of nodeids that can fit the requested cpu and memory requirements. If no
// such node is available, an empty array is returned. The array is sorted by the most suitable node first.
func (r *resources) FindBestNodes(cpu float64, mem uint64) []string {
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
func (r *resources) Add(nodeid string, cpu float64, mem uint64) {
	res, hasRes := r.nodes[nodeid]
	if !hasRes {
		return
	}

	res.CPU += cpu
	res.Mem += mem
	r.nodes[nodeid] = res
}

// Remove subtracts the resources from the node according to the cpu and memory utilization.
func (r *resources) Remove(nodeid string, cpu float64, mem uint64) {
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
func (r *resources) Move(target, source string, cpu float64, mem uint64) {
	r.Add(target, cpu, mem)
	r.Remove(source, cpu, mem)
}

func (r *resources) Map() map[string]proxy.NodeResources {
	return r.nodes
}

type referenceAffinityNodeCount struct {
	nodeid string
	count  uint64
}

type referenceAffinity struct {
	m map[string][]referenceAffinityNodeCount
}

// NewReferenceAffinity returns a referenceAffinity. This is a map of references (per domain) to an array of
// nodes this reference is found on and their count.
func NewReferenceAffinity(processes []proxy.Process) *referenceAffinity {
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

// rebalance returns a list of operations that will move running processes away from nodes that are overloaded.
func rebalance(have []proxy.Process, nodes map[string]proxy.NodeAbout) ([]interface{}, map[string]proxy.NodeResources) {
	resources := NewResources(nodes)

	// Group all running processes by node and sort them by their runtime in ascending order.
	nodeProcessMap := createNodeProcessMap(have)

	// A map from the process reference to the nodes it is running on.
	haveReferenceAffinity := NewReferenceAffinity(have)

	opStack := []interface{}{}

	// Check if any of the nodes is overloaded.
	for id, node := range nodes {
		r := node.Resources

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

					if resources.HasNodeEnough(raNodeid, p.CPU, p.Mem) {
						availableNodeid = raNodeid
						break
					}
				}
			}

			// Find the best node with enough resources available.
			if len(availableNodeid) == 0 {
				nodes := resources.FindBestNodes(p.CPU, p.Mem)
				for _, nodeid := range nodes {
					if nodeid == overloadedNodeid {
						continue
					}

					availableNodeid = nodeid
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
			resources.Move(availableNodeid, overloadedNodeid, p.CPU, p.Mem)

			// Adjust the reference affinity.
			haveReferenceAffinity.Move(p.Config.Reference, p.Config.Domain, overloadedNodeid, availableNodeid)

			// Move only one process at a time.
			break
		}
	}

	return opStack, resources.Map()
}

// relocate returns a list of operations that will move deployed processes to different nodes.
func relocate(have []proxy.Process, nodes map[string]proxy.NodeAbout, relocateMap map[string]string) ([]interface{}, map[string]proxy.NodeResources, []string) {
	resources := NewResources(nodes)

	relocatedProcessIDs := []string{}

	// A map from the process reference to the nodes it is running on.
	haveReferenceAffinity := NewReferenceAffinity(have)

	opStack := []interface{}{}

	// Check for any requested relocations.
	for processid, targetNodeid := range relocateMap {
		process := proxy.Process{}

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

			if !hasNode || !resources.HasNodeEnough(targetNodeid, process.CPU, process.Mem) {
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

					if resources.HasNodeEnough(raNodeid, process.CPU, process.Mem) {
						targetNodeid = raNodeid
						break
					}
				}
			}

			// Find the best node with enough resources available.
			if len(targetNodeid) == 0 {
				nodes := resources.FindBestNodes(process.CPU, process.Mem)
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

// createNodeProcessMap takes a list of processes and groups them by the nodeid they
// are running on. Each group contains only running processes and gets sorted by their
// preference to be moved somewhere else, increasing. From the running processes, the
// ones with the shortest runtime have the highest preference.
func createNodeProcessMap(processes []proxy.Process) map[string][]proxy.Process {
	nodeProcessMap := map[string][]proxy.Process{}

	for _, p := range processes {
		if p.State != "running" {
			continue
		}

		nodeProcessMap[p.NodeID] = append(nodeProcessMap[p.NodeID], p)
	}

	// Sort the processes by their runtime (if they are running) for each node
	for nodeid, processes := range nodeProcessMap {
		sort.SliceStable(processes, func(a, b int) bool {
			return processes[a].Runtime < processes[b].Runtime
		})

		nodeProcessMap[nodeid] = processes
	}

	return nodeProcessMap
}
