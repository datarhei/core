package forwarder

import (
	apiclient "github.com/datarhei/core/v16/cluster/client"
	"github.com/datarhei/core/v16/cluster/store"
	"github.com/datarhei/core/v16/restream/app"
)

func (f *Forwarder) ProcessAdd(origin string, config *app.Config) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.AddProcessRequest{
		Config: *config,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return reconstructError(client.ProcessAdd(origin, r))
}

func (f *Forwarder) ProcessGet(origin string, id app.ProcessID) (store.Process, string, error) {
	if origin == "" {
		origin = f.ID
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	process, nodeid, err := client.ProcessGet(origin, id)

	return process, nodeid, reconstructError(err)
}

func (f *Forwarder) ProcessUpdate(origin string, id app.ProcessID, config *app.Config) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.UpdateProcessRequest{
		Config: *config,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return reconstructError(client.ProcessUpdate(origin, id, r))
}

func (f *Forwarder) ProcessSetCommand(origin string, id app.ProcessID, command string) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.SetProcessCommandRequest{
		Command: command,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return reconstructError(client.ProcessSetCommand(origin, id, r))
}

func (f *Forwarder) ProcessSetMetadata(origin string, id app.ProcessID, key string, data interface{}) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.SetProcessMetadataRequest{
		Metadata: data,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return reconstructError(client.ProcessSetMetadata(origin, id, key, r))
}

func (f *Forwarder) ProcessRemove(origin string, id app.ProcessID) error {
	if origin == "" {
		origin = f.ID
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return reconstructError(client.ProcessRemove(origin, id))
}

func (f *Forwarder) ProcessesRelocate(origin string, relocations map[app.ProcessID]string) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.RelocateProcessesRequest{
		Map: relocations,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return reconstructError(client.ProcessesRelocate(origin, r))
}
