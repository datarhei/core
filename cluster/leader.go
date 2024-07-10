package cluster

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/datarhei/core/v16/cluster/node"
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
			locks := c.store.LockList()
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

	logger := c.logger.WithFields(log.Fields{
		"term":    term,
		"logname": "opstack",
	})

	errChan := make(chan processOpError, len(stack))

	wgReader := sync.WaitGroup{}
	wgReader.Add(1)
	go func(errChan <-chan processOpError) {
		for opErr := range errChan {
			errors = append(errors, opErr)
		}

		wgReader.Done()
	}(errChan)

	wg := sync.WaitGroup{}
	for _, op := range stack {
		wg.Add(1)

		go func(errChan chan<- processOpError, op interface{}, logger log.Logger) {
			opErr := c.applyOp(op, logger)
			if opErr.err != nil {
				errChan <- opErr
			}
			wg.Done()
		}(errChan, op, logger)
	}

	wg.Wait()

	close(errChan)

	wgReader.Wait()

	return errors
}

func (c *cluster) applyOp(op interface{}, logger log.Logger) processOpError {
	opErr := processOpError{}

	switch v := op.(type) {
	case processOpAdd:
		err := c.manager.ProcessAdd(v.nodeid, v.config, v.metadata)
		if err != nil {
			opErr = processOpError{
				processid: v.config.ProcessID(),
				err:       err,
			}
			logger.Info().WithError(err).WithFields(log.Fields{
				"processid": v.config.ProcessID(),
				"nodeid":    v.nodeid,
			}).Log("Adding process")
			break
		}

		if v.order == "start" {
			err = c.manager.ProcessCommand(v.nodeid, v.config.ProcessID(), "start")
			if err != nil {
				opErr = processOpError{
					processid: v.config.ProcessID(),
					err:       err,
				}
				logger.Info().WithError(err).WithFields(log.Fields{
					"processid": v.config.ProcessID(),
					"nodeid":    v.nodeid,
				}).Log("Starting process")
				break
			}
		}

		opErr = processOpError{
			processid: v.config.ProcessID(),
			err:       nil,
		}

		logger.Info().WithFields(log.Fields{
			"processid": v.config.ProcessID(),
			"nodeid":    v.nodeid,
		}).Log("Adding process")
	case processOpUpdate:
		err := c.manager.ProcessUpdate(v.nodeid, v.processid, v.config, v.metadata)
		if err != nil {
			opErr = processOpError{
				processid: v.processid,
				err:       err,
			}
			logger.Info().WithError(err).WithFields(log.Fields{
				"processid": v.processid,
				"nodeid":    v.nodeid,
			}).Log("Updating process")
			break
		}

		opErr = processOpError{
			processid: v.processid,
			err:       nil,
		}

		logger.Info().WithFields(log.Fields{
			"processid": v.config.ProcessID(),
			"nodeid":    v.nodeid,
		}).Log("Updating process")
	case processOpDelete:
		err := c.manager.ProcessDelete(v.nodeid, v.processid)
		if err != nil {
			opErr = processOpError{
				processid: v.processid,
				err:       err,
			}
			logger.Info().WithError(err).WithFields(log.Fields{
				"processid": v.processid,
				"nodeid":    v.nodeid,
			}).Log("Removing process")
			break
		}

		opErr = processOpError{
			processid: v.processid,
			err:       nil,
		}

		logger.Info().WithFields(log.Fields{
			"processid": v.processid,
			"nodeid":    v.nodeid,
		}).Log("Removing process")
	case processOpMove:
		err := c.manager.ProcessAdd(v.toNodeid, v.config, v.metadata)
		if err != nil {
			opErr = processOpError{
				processid: v.config.ProcessID(),
				err:       err,
			}
			logger.Info().WithError(err).WithFields(log.Fields{
				"processid":  v.config.ProcessID(),
				"fromnodeid": v.fromNodeid,
				"tonodeid":   v.toNodeid,
			}).Log("Moving process, adding process")
			break
		}

		// Transfer report with best effort, it's ok if it fails.
		err = c.manager.ProcessCommand(v.fromNodeid, v.config.ProcessID(), "stop")
		if err == nil {
			process, err := c.manager.ProcessGet(v.fromNodeid, v.config.ProcessID(), []string{"report"})
			if err != nil {
				logger.Info().WithError(err).WithFields(log.Fields{
					"processid":  v.config.ProcessID(),
					"fromnodeid": v.fromNodeid,
					"tonodeid":   v.toNodeid,
				}).Log("Moving process, get process report")
			}
			if process.Report != nil && err == nil {
				report := process.Report.Marshal()
				err = c.manager.ProcessReportSet(v.toNodeid, v.config.ProcessID(), &report)
				if err != nil {
					logger.Info().WithError(err).WithFields(log.Fields{
						"processid":  v.config.ProcessID(),
						"fromnodeid": v.fromNodeid,
						"tonodeid":   v.toNodeid,
					}).Log("Moving process, set process report")
				}
			}
		} else {
			logger.Info().WithError(err).WithFields(log.Fields{
				"processid":  v.config.ProcessID(),
				"fromnodeid": v.fromNodeid,
				"tonodeid":   v.toNodeid,
			}).Log("Moving process, stopping process")
		}

		err = c.manager.ProcessDelete(v.fromNodeid, v.config.ProcessID())
		if err != nil {
			opErr = processOpError{
				processid: v.config.ProcessID(),
				err:       err,
			}
			logger.Info().WithError(err).WithFields(log.Fields{
				"processid":  v.config.ProcessID(),
				"fromnodeid": v.fromNodeid,
				"tonodeid":   v.toNodeid,
			}).Log("Moving process, removing process")
			break
		}

		if v.order == "start" {
			err = c.manager.ProcessCommand(v.toNodeid, v.config.ProcessID(), "start")
			if err != nil {
				opErr = processOpError{
					processid: v.config.ProcessID(),
					err:       err,
				}
				logger.Info().WithError(err).WithFields(log.Fields{
					"processid":  v.config.ProcessID(),
					"fromnodeid": v.fromNodeid,
					"tonodeid":   v.toNodeid,
				}).Log("Moving process, starting process")
				break
			}
		}

		opErr = processOpError{
			processid: v.config.ProcessID(),
			err:       nil,
		}

		logger.Info().WithFields(log.Fields{
			"processid":  v.config.ProcessID(),
			"fromnodeid": v.fromNodeid,
			"tonodeid":   v.toNodeid,
		}).Log("Moving process")
	case processOpStart:
		err := c.manager.ProcessCommand(v.nodeid, v.processid, "start")
		if err != nil {
			opErr = processOpError{
				processid: v.processid,
				err:       err,
			}
			logger.Info().WithError(err).WithFields(log.Fields{
				"processid": v.processid,
				"nodeid":    v.nodeid,
			}).Log("Starting process")
			break
		}

		opErr = processOpError{
			processid: v.processid,
			err:       nil,
		}

		logger.Info().WithFields(log.Fields{
			"processid": v.processid,
			"nodeid":    v.nodeid,
		}).Log("Starting process")
	case processOpStop:
		err := c.manager.ProcessCommand(v.nodeid, v.processid, "stop")
		if err != nil {
			opErr = processOpError{
				processid: v.processid,
				err:       err,
			}
			logger.Info().WithError(err).WithFields(log.Fields{
				"processid": v.processid,
				"nodeid":    v.nodeid,
			}).Log("Stopping process")
			break
		}

		opErr = processOpError{
			processid: v.processid,
			err:       nil,
		}

		logger.Info().WithFields(log.Fields{
			"processid": v.processid,
			"nodeid":    v.nodeid,
		}).Log("Stopping process")
	case processOpReject:
		opErr = processOpError(v)
		logger.Warn().WithError(v.err).WithField("processid", v.processid).Log("Process rejected")
	case processOpSkip:
		opErr = processOpError{
			processid: v.processid,
			err:       v.err,
		}
		logger.Warn().WithError(v.err).WithFields(log.Fields{
			"nodeid":    v.nodeid,
			"processid": v.processid,
		}).Log("Process skipped")
	case processOpError:
		opErr = v
	default:
		logger.Warn().Log("Unknown operation on stack: %+v", v)
	}

	return opErr
}

// createNodeProcessMap takes a list of processes and groups them by the nodeid they
// are running on. Each group contains only running processes and gets sorted by their
// preference to be moved somewhere else, increasing. From the running processes, the
// ones with the shortest runtime have the highest preference.
func createNodeProcessMap(processes []node.Process) map[string][]node.Process {
	nodeProcessMap := map[string][]node.Process{}

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
