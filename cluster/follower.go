package cluster

// followerLoop is run by every follower node in the cluster.
func (c *cluster) followerLoop(stopCh chan struct{}) {
	// Periodically reconcile as long as we are the leader
	for {
		select {
		case <-stopCh:
			return
		case <-c.shutdownCh:
			return
		}
	}
}
