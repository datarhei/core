package cluster

import (
	"context"
	"time"

	"github.com/datarhei/core/v16/cluster/raft"
)

// followerLoop is run by every follower node in the cluster.
func (c *cluster) followerLoop(stopCh chan struct{}) {
	establishedFollower := false

	if !establishedFollower {
		c.establishFollowership(context.TODO())
		establishedFollower = true
		defer c.revokeFollowership()
	}

	for {
		select {
		case <-stopCh:
			return
		case <-c.shutdownCh:
			return
		}
	}
}

func (c *cluster) establishFollowership(ctx context.Context) {
	c.logger.Info().Log("Establishing followership")

	ctx, cancel := context.WithCancel(ctx)
	c.cancelFollowerShip = cancel

	go c.recoverCluster(ctx, c.syncInterval)
}

func (c *cluster) revokeFollowership() {
	c.logger.Info().Log("Revoking followership")

	if c.cancelFollowerShip != nil {
		c.cancelFollowerShip()
		c.cancelFollowerShip = nil
	}
}

func (c *cluster) recoverCluster(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.leaderLock.Lock()
			hasLeader := c.hasRaftLeader
			lastLeaderChange := c.lastLeaderChange
			c.leaderLock.Unlock()

			uptime := c.raft.Stats().Uptime
			if uptime < 2*c.recoverTimeout {
				continue
			}

			if !hasLeader && c.recoverTimeout > 0 && time.Since(lastLeaderChange) > c.recoverTimeout {
				peers := []raft.Peer{}

				// find living peers and recover
				servers, err := c.raft.Servers()
				if err != nil {
					break
				}

				nodes := c.manager.NodeList()
				for _, node := range nodes {
					if _, err := node.Status(); err != nil {
						continue
					}

					id := node.About().ID

					for _, server := range servers {
						if server.ID == id && server.ID != c.nodeID {
							peers = append(peers, raft.Peer{
								ID:      id,
								Address: server.Address,
							})
						}
					}
				}

				c.logger.Warn().WithField("peers", peers).Log("Recovering raft")

				// recover raft with new set of peers
				err = c.raft.Recover(peers, 2*interval)
				if err != nil {
					c.logger.Error().WithError(err).Log("Recovering raft failed, shutting down")
					c.Shutdown()
					return
				}
			}
		}
	}
}
