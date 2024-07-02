package forwarder

import (
	apiclient "github.com/datarhei/core/v16/cluster/client"
	"github.com/datarhei/core/v16/restream/app"
)

func (f *Forwarder) AddProcess(origin string, config *app.Config) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.AddProcessRequest{
		Config: *config,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.AddProcess(origin, r)
}

func (f *Forwarder) UpdateProcess(origin string, id app.ProcessID, config *app.Config) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.UpdateProcessRequest{
		Config: *config,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.UpdateProcess(origin, id, r)
}

func (f *Forwarder) SetProcessCommand(origin string, id app.ProcessID, command string) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.SetProcessCommandRequest{
		Command: command,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.SetProcessCommand(origin, id, r)
}

func (f *Forwarder) SetProcessMetadata(origin string, id app.ProcessID, key string, data interface{}) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.SetProcessMetadataRequest{
		Metadata: data,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.SetProcessMetadata(origin, id, key, r)
}

func (f *Forwarder) RemoveProcess(origin string, id app.ProcessID) error {
	if origin == "" {
		origin = f.ID
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.RemoveProcess(origin, id)
}

func (f *Forwarder) RelocateProcesses(origin string, relocations map[app.ProcessID]string) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.RelocateProcessesRequest{
		Map: relocations,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.RelocateProcesses(origin, r)
}
