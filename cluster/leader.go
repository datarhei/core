package cluster

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/datarhei/core/v16/log"
)

// monitorLeadership listens to the raf notify channel in order to find
// out if we got the leadership or lost it.
// https://github.com/hashicorp/consul/blob/44b39240a86bc94ddc67bc105286ab450bd869a9/agent/consul/leader.go#L71
func (c *cluster) monitorLeadership() {
	// We use the notify channel we configured Raft with, NOT Raft's
	// leaderCh, which is only notified best-effort. Doing this ensures
	// that we get all notifications in order, which is required for
	// cleanup and to ensure we never run multiple leader loops.
	raftNotifyCh := c.raftNotifyCh

	var weAreLeaderCh chan struct{}
	var leaderLoop sync.WaitGroup
	for {
		select {
		case isLeader := <-raftNotifyCh:
			switch {
			case isLeader:
				if weAreLeaderCh != nil {
					c.logger.Error().Log("attempted to start the leader loop while running")
					continue
				}

				weAreLeaderCh = make(chan struct{})
				leaderLoop.Add(1)
				go func(ch chan struct{}) {
					defer leaderLoop.Done()
					c.leaderLoop(ch)
				}(weAreLeaderCh)
				c.logger.Info().Log("cluster leadership acquired")

			default:
				if weAreLeaderCh == nil {
					c.logger.Error().Log("attempted to stop the leader loop while not running")
					continue
				}

				c.logger.Debug().Log("shutting down leader loop")
				close(weAreLeaderCh)
				leaderLoop.Wait()
				weAreLeaderCh = nil
				c.logger.Info().Log("cluster leadership lost")
			}
		case <-c.shutdownCh:
			return
		}
	}
}

// leadershipTransfer tries to transfer the leadership to another node e.g. in order
// to do a graceful shutdown.
// https://github.com/hashicorp/consul/blob/44b39240a86bc94ddc67bc105286ab450bd869a9/agent/consul/leader.go#L122
func (c *cluster) leadershipTransfer() error {
	retryCount := 3
	for i := 0; i < retryCount; i++ {
		future := c.raft.LeadershipTransfer()
		if err := future.Error(); err != nil {
			c.logger.Error().WithError(err).WithFields(log.Fields{
				"attempt":     i,
				"retry_limit": retryCount,
			}).Log("failed to transfer leadership attempt, will retry")
		} else {
			c.logger.Info().WithFields(log.Fields{
				"attempt":     i,
				"retry_limit": retryCount,
			}).Log("successfully transferred leadership")
			return nil
		}

	}
	return fmt.Errorf("failed to transfer leadership in %d attempts", retryCount)
}

// leaderLoop runs as long as we are the leader to run various maintenance activities
// https://github.com/hashicorp/consul/blob/44b39240a86bc94ddc67bc105286ab450bd869a9/agent/consul/leader.go#L146
func (c *cluster) leaderLoop(stopCh chan struct{}) {
	establishedLeader := false
RECONCILE:
	// Setup a reconciliation timer
	interval := time.After(s.config.ReconcileInterval)

	// Apply a raft barrier to ensure our FSM is caught up
	barrier := c.raft.Barrier(time.Minute)
	if err := barrier.Error(); err != nil {
		c.logger.Error().WithError(err).Log("failed to wait for barrier")
		goto WAIT
	}

	// Check if we need to handle initial leadership actions
	if !establishedLeader {
		if err := c.establishLeadership(stopCtx); err != nil {
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

	// Periodically reconcile as long as we are the leader,
	// or when Serf events arrive
	for {
		select {
		case <-stopCh:
			return
		case <-c.shutdownCh:
			return
		case <-interval:
			goto RECONCILE
		case errCh := <-c.reassertLeaderCh:
			// we can get into this state when the initial
			// establishLeadership has failed as well as the follow
			// up leadershipTransfer. Afterwards we will be waiting
			// for the interval to trigger a reconciliation and can
			// potentially end up here. There is no point to
			// reassert because this agent was never leader in the
			// first place.
			if !establishedLeader {
				errCh <- fmt.Errorf("leadership has not been established")
				continue
			}

			// continue to reassert only if we previously were the
			// leader, which means revokeLeadership followed by an
			// establishLeadership().
			c.revokeLeadership()
			err := c.establishLeadership(stopCtx)
			errCh <- err

			// in case establishLeadership failed, we will try to
			// transfer leadership. At this time raft thinks we are
			// the leader, but we disagree.
			if err != nil {
				if err := c.leadershipTransfer(); err != nil {
					// establishedLeader was true before,
					// but it no longer is since it revoked
					// leadership and Leadership transfer
					// also failed. Which is why it stays
					// in the leaderLoop, but now
					// establishedLeader needs to be set to
					// false.
					establishedLeader = false
					interval = time.After(5 * time.Second)
					goto WAIT
				}

				// leadershipTransfer was successful and it is
				// time to leave the leaderLoop.
				return
			}

		}
	}
}

func (c *cluster) establishLeadership(ctx context.Context) error {
	return nil
}

func (c *cluster) revokeLeadership() {

}
