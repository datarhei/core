package coreclient

import "github.com/datarhei/core-client-go/v16/api"

func (r *restclient) ClusterProcessList(opts ProcessListOptions) ([]api.Process, error) {
	return r.processList("cluster", opts)
}

func (r *restclient) ClusterProcess(id ProcessID, filter []string) (api.Process, error) {
	return r.process("cluster", id, filter)
}

func (r *restclient) ClusterProcessAdd(p api.ProcessConfig) error {
	return r.processAdd("cluster", p)
}

func (r *restclient) ClusterProcessUpdate(id ProcessID, p api.ProcessConfig) error {
	return r.processUpdate("cluster", id, p)
}

func (r *restclient) ClusterProcessDelete(id ProcessID) error {
	return r.processDelete("cluster", id)
}

func (r *restclient) ClusterProcessCommand(id ProcessID, command string) error {
	return r.processCommand("cluster", id, command)
}

func (r *restclient) ClusterProcessMetadata(id ProcessID, key string) (api.Metadata, error) {
	return r.processMetadata("cluster", id, key)
}

func (r *restclient) ClusterProcessMetadataSet(id ProcessID, key string, metadata api.Metadata) error {
	return r.processMetadataSet("cluster", id, key, metadata)
}

func (r *restclient) ClusterProcessProbe(id ProcessID) (api.Probe, error) {
	return r.processProbe("cluster", id)
}

func (r *restclient) ClusterProcessProbeConfig(config api.ProcessConfig, coreid string) (api.Probe, error) {
	return r.processProbeConfig("cluster", config, coreid)
}
