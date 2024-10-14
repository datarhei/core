package client

import (
	"net/url"
	"strings"

	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/mem"
	"github.com/datarhei/core/v16/restream/app"
)

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

func (r *restclient) Process(id app.ProcessID, filter []string) (api.Process, error) {
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

func (r *restclient) ProcessAdd(p *app.Config, metadata map[string]interface{}) error {
	buf := mem.Get()
	defer mem.Put(buf)

	config := api.ProcessConfig{}
	config.Unmarshal(p, metadata)

	e := json.NewEncoder(buf)
	e.Encode(config)

	_, err := r.call("POST", "/v3/process", nil, nil, "application/json", buf.Reader())
	if err != nil {
		return err
	}

	return nil
}

func (r *restclient) ProcessUpdate(id app.ProcessID, p *app.Config, metadata map[string]interface{}) error {
	buf := mem.Get()
	defer mem.Put(buf)

	config := api.ProcessConfig{}
	config.Unmarshal(p, metadata)

	e := json.NewEncoder(buf)
	e.Encode(config)

	query := &url.Values{}
	query.Set("domain", id.Domain)

	_, err := r.call("PUT", "/v3/process/"+url.PathEscape(id.ID), query, nil, "application/json", buf.Reader())
	if err != nil {
		return err
	}

	return nil
}

func (r *restclient) ProcessReportSet(id app.ProcessID, report *app.Report) error {
	buf := mem.Get()
	defer mem.Put(buf)

	data := api.ProcessReport{}
	data.Unmarshal(report)

	e := json.NewEncoder(buf)
	e.Encode(data)

	query := &url.Values{}
	query.Set("domain", id.Domain)

	_, err := r.call("PUT", "/v3/process/"+url.PathEscape(id.ID)+"/report", query, nil, "application/json", buf.Reader())
	if err != nil {
		return err
	}

	return nil
}

func (r *restclient) ProcessDelete(id app.ProcessID) error {
	query := &url.Values{}
	query.Set("domain", id.Domain)

	_, err := r.call("DELETE", "/v3/process/"+url.PathEscape(id.ID), query, nil, "", nil)

	return err
}

func (r *restclient) ProcessCommand(id app.ProcessID, command string) error {
	buf := mem.Get()
	defer mem.Put(buf)

	e := json.NewEncoder(buf)
	e.Encode(api.Command{
		Command: command,
	})

	query := &url.Values{}
	query.Set("domain", id.Domain)

	_, err := r.call("PUT", "/v3/process/"+url.PathEscape(id.ID)+"/command", query, nil, "application/json", buf.Reader())

	return err
}

func (r *restclient) ProcessMetadata(id app.ProcessID, key string) (api.Metadata, error) {
	var m api.Metadata

	path := "/v3/process/" + url.PathEscape(id.ID) + "/metadata"

	if len(key) != 0 {
		path += "/" + url.PathEscape(key)
	}

	query := &url.Values{}
	query.Set("domain", id.Domain)

	data, err := r.call("GET", path, query, nil, "", nil)
	if err != nil {
		return m, err
	}

	err = json.Unmarshal(data, &m)

	return m, err
}

func (r *restclient) ProcessMetadataSet(id app.ProcessID, key string, metadata api.Metadata) error {
	buf := mem.Get()
	defer mem.Put(buf)

	e := json.NewEncoder(buf)
	e.Encode(metadata)

	query := &url.Values{}
	query.Set("domain", id.Domain)

	_, err := r.call("PUT", "/v3/process/"+url.PathEscape(id.ID)+"/metadata/"+url.PathEscape(key), query, nil, "application/json", buf.Reader())

	return err
}

func (r *restclient) ProcessProbe(id app.ProcessID) (api.Probe, error) {
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

func (r *restclient) ProcessProbeConfig(p *app.Config) (api.Probe, error) {
	var probe api.Probe

	buf := mem.Get()
	defer mem.Put(buf)

	config := api.ProcessConfig{}
	config.Unmarshal(p, nil)

	e := json.NewEncoder(buf)
	e.Encode(config)

	data, err := r.call("POST", "/v3/process/probe", nil, nil, "application/json", buf.Reader())
	if err != nil {
		return probe, err
	}

	err = json.Unmarshal(data, &p)

	return probe, err
}

func (r *restclient) ProcessConfig(id app.ProcessID) (api.ProcessConfig, error) {
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

func (r *restclient) ProcessReport(id app.ProcessID) (api.ProcessReport, error) {
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

func (r *restclient) ProcessState(id app.ProcessID) (api.ProcessState, error) {
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
