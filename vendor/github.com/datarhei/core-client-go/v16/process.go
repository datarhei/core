package coreclient

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/datarhei/core-client-go/v16/api"
)

type ProcessListOptions struct {
	ID         []string
	Filter     []string
	Reference  string
	IDPattern  string
	RefPattern string
}

func (p *ProcessListOptions) Query() string {
	values := url.Values{}
	values.Set("id", strings.Join(p.ID, ","))
	values.Set("filter", strings.Join(p.Filter, ","))
	values.Set("reference", p.Reference)
	values.Set("idpattern", p.IDPattern)
	values.Set("refpattern", p.RefPattern)

	return values.Encode()
}

func (r *restclient) ProcessList(opts ProcessListOptions) ([]api.Process, error) {
	var processes []api.Process

	data, err := r.call("GET", "/v3/process?"+opts.Query(), "", nil)
	if err != nil {
		return processes, err
	}

	err = json.Unmarshal(data, &processes)

	return processes, err
}

func (r *restclient) Process(id string, filter []string) (api.Process, error) {
	var info api.Process

	values := url.Values{}
	values.Set("filter", strings.Join(filter, ","))

	data, err := r.call("GET", "/v3/process/"+url.PathEscape(id)+"?"+values.Encode(), "", nil)
	if err != nil {
		return info, err
	}

	err = json.Unmarshal(data, &info)

	return info, err
}

func (r *restclient) ProcessAdd(p api.ProcessConfig) error {
	var buf bytes.Buffer

	e := json.NewEncoder(&buf)
	e.Encode(p)

	_, err := r.call("POST", "/v3/process", "application/json", &buf)
	if err != nil {
		return err
	}

	return nil
}

func (r *restclient) ProcessUpdate(id string, p api.ProcessConfig) error {
	var buf bytes.Buffer

	e := json.NewEncoder(&buf)
	e.Encode(p)

	_, err := r.call("PUT", "/v3/process/"+url.PathEscape(id)+"", "application/json", &buf)
	if err != nil {
		return err
	}

	return nil
}

func (r *restclient) ProcessDelete(id string) error {
	r.call("DELETE", "/v3/process/"+url.PathEscape(id), "", nil)

	return nil
}

func (r *restclient) ProcessCommand(id, command string) error {
	var buf bytes.Buffer

	e := json.NewEncoder(&buf)
	e.Encode(api.Command{
		Command: command,
	})

	_, err := r.call("PUT", "/v3/process/"+url.PathEscape(id)+"/command", "application/json", &buf)
	if err != nil {
		return err
	}

	return nil
}

func (r *restclient) ProcessProbe(id string) (api.Probe, error) {
	var p api.Probe

	data, err := r.call("GET", "/v3/process/"+url.PathEscape(id)+"/probe", "", nil)
	if err != nil {
		return p, err
	}

	err = json.Unmarshal(data, &p)

	return p, err
}

func (r *restclient) ProcessConfig(id string) (api.ProcessConfig, error) {
	var p api.ProcessConfig

	data, err := r.call("GET", "/v3/process/"+url.PathEscape(id)+"/config", "", nil)
	if err != nil {
		return p, err
	}

	err = json.Unmarshal(data, &p)

	return p, err
}

func (r *restclient) ProcessReport(id string) (api.ProcessReport, error) {
	var p api.ProcessReport

	data, err := r.call("GET", "/v3/process/"+url.PathEscape(id)+"/report", "", nil)
	if err != nil {
		return p, err
	}

	err = json.Unmarshal(data, &p)

	return p, err
}

func (r *restclient) ProcessState(id string) (api.ProcessState, error) {
	var p api.ProcessState

	data, err := r.call("GET", "/v3/process/"+url.PathEscape(id)+"/state", "", nil)
	if err != nil {
		return p, err
	}

	err = json.Unmarshal(data, &p)

	return p, err
}

func (r *restclient) ProcessMetadata(id, key string) (api.Metadata, error) {
	var m api.Metadata

	path := "/v3/process/" + url.PathEscape(id) + "/metadata"
	if len(key) != 0 {
		path += "/" + url.PathEscape(key)
	}

	data, err := r.call("GET", path, "", nil)
	if err != nil {
		return m, err
	}

	err = json.Unmarshal(data, &m)

	return m, err
}

func (r *restclient) ProcessMetadataSet(id, key string, metadata api.Metadata) error {
	var buf bytes.Buffer

	e := json.NewEncoder(&buf)
	e.Encode(metadata)

	_, err := r.call("PUT", "/v3/process/"+url.PathEscape(id)+"/metadata/"+url.PathEscape(key), "application/json", &buf)
	if err != nil {
		return err
	}

	return nil
}
