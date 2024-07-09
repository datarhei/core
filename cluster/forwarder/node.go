package forwarder

import (
	apiclient "github.com/datarhei/core/v16/cluster/client"
)

func (f *Forwarder) NodeSetState(origin, nodeid, state string) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.SetNodeStateRequest{
		State: state,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return reconstructError(client.NodeSetState(origin, nodeid, r))
}
