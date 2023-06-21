package coreclient

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/datarhei/core-client-go/v16/api"
)

type ProcessID struct {
	ID     string
	Domain string
}

func NewProcessID(id, domain string) ProcessID {
	return ProcessID{
		ID:     id,
		Domain: domain,
	}
}

func ParseProcessID(pid string) ProcessID {
	i := strings.LastIndex(pid, "@")
	if i == -1 {
		return NewProcessID(pid, "")
	}

	return NewProcessID(pid[:i], pid[i+1:])
}

func ProcessIDFromProcess(p api.Process) ProcessID {
	return NewProcessID(p.ID, p.Config.Domain)
}

func (p ProcessID) String() string {
	return p.ID + "@" + p.Domain
}

type ProcessListOptions struct {
	ID            []string
	Filter        []string
	Domain        string
	Reference     string
	IDPattern     string
	RefPattern    string
	OwnerPattern  string
	DomainPattern string
}

func (p *ProcessListOptions) Query() *url.Values {
	values := &url.Values{}
	values.Set("id", strings.Join(p.ID, ","))
	values.Set("filter", strings.Join(p.Filter, ","))
	values.Set("domain", p.Domain)
	values.Set("reference", p.Reference)
	values.Set("idpattern", p.IDPattern)
	values.Set("refpattern", p.RefPattern)
	values.Set("ownerpattern", p.OwnerPattern)
	values.Set("domainpattern", p.DomainPattern)

	return values
}

func (r *restclient) ProcessList(opts ProcessListOptions) ([]api.Process, error) {
	var processes []api.Process

	data, err := r.call("GET", "/v3/process", opts.Query(), nil, "", nil)
	if err != nil {
		return processes, err
	}

	err = json.Unmarshal(data, &processes)

	return processes, err
}

func (r *restclient) Process(id ProcessID, filter []string) (api.Process, error) {
	var info api.Process

	values := &url.Values{}
	values.Set("filter", strings.Join(filter, ","))
	values.Set("domain", id.Domain)

	data, err := r.call("GET", "/v3/process/"+url.PathEscape(id.ID), values, nil, "", nil)
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

	_, err := r.call("POST", "/v3/process", nil, nil, "application/json", &buf)
	if err != nil {
		return err
	}

	return nil
}

func (r *restclient) ProcessUpdate(id ProcessID, p api.ProcessConfig) error {
	var buf bytes.Buffer

	e := json.NewEncoder(&buf)
	e.Encode(p)

	query := &url.Values{}
	query.Set("domain", id.Domain)

	_, err := r.call("PUT", "/v3/process/"+url.PathEscape(id.ID), query, nil, "application/json", &buf)
	if err != nil {
		return err
	}

	return nil
}

func (r *restclient) ProcessDelete(id ProcessID) error {
	query := &url.Values{}
	query.Set("domain", id.Domain)

	r.call("DELETE", "/v3/process/"+url.PathEscape(id.ID), query, nil, "", nil)

	return nil
}

func (r *restclient) ProcessCommand(id ProcessID, command string) error {
	var buf bytes.Buffer

	e := json.NewEncoder(&buf)
	e.Encode(api.Command{
		Command: command,
	})

	query := &url.Values{}
	query.Set("domain", id.Domain)

	_, err := r.call("PUT", "/v3/process/"+url.PathEscape(id.ID)+"/command", query, nil, "application/json", &buf)
	if err != nil {
		return err
	}

	return nil
}

func (r *restclient) ProcessProbe(id ProcessID) (api.Probe, error) {
	var p api.Probe

	query := &url.Values{}
	query.Set("domain", id.Domain)

	data, err := r.call("GET", "/v3/process/"+url.PathEscape(id.ID)+"/probe", query, nil, "", nil)
	if err != nil {
		return p, err
	}

	err = json.Unmarshal(data, &p)

	return p, err
}

func (r *restclient) ProcessConfig(id ProcessID) (api.ProcessConfig, error) {
	var p api.ProcessConfig

	query := &url.Values{}
	query.Set("domain", id.Domain)

	data, err := r.call("GET", "/v3/process/"+url.PathEscape(id.ID)+"/config", query, nil, "", nil)
	if err != nil {
		return p, err
	}

	err = json.Unmarshal(data, &p)

	return p, err
}

func (r *restclient) ProcessReport(id ProcessID) (api.ProcessReport, error) {
	var p api.ProcessReport

	query := &url.Values{}
	query.Set("domain", id.Domain)

	data, err := r.call("GET", "/v3/process/"+url.PathEscape(id.ID)+"/report", query, nil, "", nil)
	if err != nil {
		return p, err
	}

	err = json.Unmarshal(data, &p)

	return p, err
}

func (r *restclient) ProcessState(id ProcessID) (api.ProcessState, error) {
	var p api.ProcessState

	query := &url.Values{}
	query.Set("domain", id.Domain)

	data, err := r.call("GET", "/v3/process/"+url.PathEscape(id.ID)+"/state", query, nil, "", nil)
	if err != nil {
		return p, err
	}

	err = json.Unmarshal(data, &p)

	return p, err
}

func (r *restclient) ProcessMetadata(id ProcessID, key string) (api.Metadata, error) {
	var m api.Metadata

	query := &url.Values{}
	query.Set("domain", id.Domain)

	path := "/v3/process/" + url.PathEscape(id.ID) + "/metadata"
	if len(key) != 0 {
		path += "/" + url.PathEscape(key)
	}

	data, err := r.call("GET", path, query, nil, "", nil)
	if err != nil {
		return m, err
	}

	err = json.Unmarshal(data, &m)

	return m, err
}

func (r *restclient) ProcessMetadataSet(id ProcessID, key string, metadata api.Metadata) error {
	var buf bytes.Buffer

	e := json.NewEncoder(&buf)
	e.Encode(metadata)

	query := &url.Values{}
	query.Set("domain", id.Domain)

	_, err := r.call("PUT", "/v3/process/"+url.PathEscape(id.ID)+"/metadata/"+url.PathEscape(key), query, nil, "application/json", &buf)
	if err != nil {
		return err
	}

	return nil
}
