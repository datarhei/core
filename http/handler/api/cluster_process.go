package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	clientapi "github.com/datarhei/core-client-go/v16/api"
	"github.com/datarhei/core/v16/cluster/proxy"
	"github.com/datarhei/core/v16/cluster/store"
	"github.com/datarhei/core/v16/glob"
	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/handler/util"
	"github.com/datarhei/core/v16/restream"
	"github.com/datarhei/core/v16/restream/app"
	"github.com/labstack/echo/v4"
	"github.com/lithammer/shortuuid/v4"
)

// GetAllProcesses returns the list of processes running on all nodes of the cluster
// @Summary List of processes in the cluster
// @Description List of processes in the cluster
// @Tags v16.?.?
// @ID cluster-3-get-all-processes
// @Produce json
// @Param domain query string false "Domain to act on"
// @Param filter query string false "Comma separated list of fields (config, state, report, metadata) that will be part of the output. If empty, all fields will be part of the output."
// @Param reference query string false "Return only these process that have this reference value. If empty, the reference will be ignored."
// @Param id query string false "Comma separated list of process ids to list. Overrides the reference. If empty all IDs will be returned."
// @Param idpattern query string false "Glob pattern for process IDs. If empty all IDs will be returned. Intersected with results from other pattern matches."
// @Param refpattern query string false "Glob pattern for process references. If empty all IDs will be returned. Intersected with results from other pattern matches."
// @Param ownerpattern query string false "Glob pattern for process owners. If empty all IDs will be returned. Intersected with results from other pattern matches."
// @Param domainpattern query string false "Glob pattern for process domain. If empty all IDs will be returned. Intersected with results from other pattern matches."
// @Success 200 {array} api.Process
// @Security ApiKeyAuth
// @Router /api/v3/cluster/process [get]
func (h *ClusterHandler) GetAllProcesses(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	filter := newFilter(util.DefaultQuery(c, "filter", ""))
	reference := util.DefaultQuery(c, "reference", "")
	wantids := strings.FieldsFunc(util.DefaultQuery(c, "id", ""), func(r rune) bool {
		return r == rune(',')
	})
	domain := util.DefaultQuery(c, "domain", "")
	idpattern := util.DefaultQuery(c, "idpattern", "")
	refpattern := util.DefaultQuery(c, "refpattern", "")
	ownerpattern := util.DefaultQuery(c, "ownerpattern", "")
	domainpattern := util.DefaultQuery(c, "domainpattern", "")

	procs := h.proxy.ListProcesses(proxy.ProcessListOptions{
		ID:            wantids,
		Filter:        filter.Slice(),
		Domain:        domain,
		Reference:     reference,
		IDPattern:     idpattern,
		RefPattern:    refpattern,
		OwnerPattern:  ownerpattern,
		DomainPattern: domainpattern,
	})

	processes := []clientapi.Process{}
	pmap := map[app.ProcessID]struct{}{}

	for _, p := range procs {
		if !h.iam.Enforce(ctxuser, domain, "process:"+p.ID, "read") {
			continue
		}

		processes = append(processes, p)
		pmap[app.NewProcessID(p.ID, p.Domain)] = struct{}{}
	}

	missing := []api.Process{}

	// Here we have to add those processes that are in the cluster DB and couldn't be deployed
	{
		processes := h.cluster.ListProcesses()
		filtered := h.getFilteredStoreProcesses(processes, wantids, domain, reference, idpattern, refpattern, ownerpattern, domainpattern)

		for _, p := range filtered {
			if !h.iam.Enforce(ctxuser, domain, "process:"+p.Config.ID, "read") {
				continue
			}

			// Check if the process has been deployed
			if _, ok := pmap[p.Config.ProcessID()]; ok {
				continue
			}

			process := h.convertStoreProcessToAPIProcess(p, filter)

			missing = append(missing, process)
		}
	}

	// We're doing some byte-wrangling here because the processes from the nodes
	// are of type clientapi.Process, the missing processes are from type api.Process.
	// They are actually the same and converting them is cumbersome. That's why
	// we're doing the JSON marshalling here and appending these two slices is done
	// in JSON representation.

	data, err := json.Marshal(processes)
	if err != nil {
		return api.Err(http.StatusInternalServerError, "", err.Error())
	}

	buf := &bytes.Buffer{}

	if len(missing) != 0 {
		reallyData, err := json.Marshal(missing)
		if err != nil {
			return api.Err(http.StatusInternalServerError, "", err.Error())
		}

		i := bytes.LastIndexByte(data, ']')
		if i == -1 {
			return api.Err(http.StatusInternalServerError, "", "no valid JSON")
		}

		if len(processes) != 0 {
			data[i] = ','
		} else {
			data[i] = ' '
		}
		buf.Write(data)

		i = bytes.IndexByte(reallyData, '[')
		if i == -1 {
			return api.Err(http.StatusInternalServerError, "", "no valid JSON")
		}
		buf.Write(reallyData[i+1:])
	} else {
		buf.Write(data)
	}

	return c.Stream(http.StatusOK, "application/json", buf)
}

func (h *ClusterHandler) getFilteredStoreProcesses(processes []store.Process, wantids []string, domain, reference, idpattern, refpattern, ownerpattern, domainpattern string) []store.Process {
	filtered := []store.Process{}

	count := 0

	var idglob glob.Glob
	var refglob glob.Glob
	var ownerglob glob.Glob
	var domainglob glob.Glob

	if len(idpattern) != 0 {
		count++
		idglob, _ = glob.Compile(idpattern)
	}

	if len(refpattern) != 0 {
		count++
		refglob, _ = glob.Compile(refpattern)
	}

	if len(ownerpattern) != 0 {
		count++
		ownerglob, _ = glob.Compile(ownerpattern)
	}

	if len(domainpattern) != 0 {
		count++
		domainglob, _ = glob.Compile(domainpattern)
	}

	for _, t := range processes {
		matches := 0
		if idglob != nil {
			if match := idglob.Match(t.Config.ID); match {
				matches++
			}
		}

		if refglob != nil {
			if match := refglob.Match(t.Config.Reference); match {
				matches++
			}
		}

		if ownerglob != nil {
			if match := ownerglob.Match(t.Config.Owner); match {
				matches++
			}
		}

		if domainglob != nil {
			if match := domainglob.Match(t.Config.Domain); match {
				matches++
			}
		}

		if count != matches {
			continue
		}

		filtered = append(filtered, t)
	}

	final := []store.Process{}

	if len(wantids) == 0 || len(reference) != 0 {
		for _, p := range filtered {
			if len(reference) != 0 && p.Config.Reference != reference {
				continue
			}

			final = append(final, p)
		}
	} else {
		for _, p := range filtered {
			for _, wantid := range wantids {
				if wantid == p.Config.ID {
					final = append(final, p)
				}
			}
		}
	}

	return final
}

func (h *ClusterHandler) convertStoreProcessToAPIProcess(p store.Process, filter filter) api.Process {
	process := api.Process{
		ID:        p.Config.ID,
		Owner:     p.Config.Owner,
		Domain:    p.Config.Domain,
		Type:      "ffmpeg",
		Reference: p.Config.Reference,
		CreatedAt: p.CreatedAt.Unix(),
		UpdatedAt: p.UpdatedAt.Unix(),
	}

	if filter.metadata {
		process.Metadata = p.Metadata
	}

	if filter.config {
		config := &api.ProcessConfig{}
		config.Unmarshal(p.Config)

		process.Config = config
	}

	if filter.state {
		process.State = &api.ProcessState{
			Order:   p.Order,
			LastLog: p.Error,
		}

		if len(p.Error) != 0 {
			process.State.State = "failed"
		} else {
			process.State.State = "finished"
		}
	}

	if filter.report {
		process.Report = &api.ProcessReport{
			ProcessReportEntry: api.ProcessReportEntry{
				CreatedAt: p.CreatedAt.Unix(),
				Prelude:   []string{},
				Log:       [][2]string{},
				Matches:   []string{},
			},
		}

		if len(p.Error) != 0 {
			process.Report.Prelude = []string{p.Error}
			process.Report.Log = [][2]string{
				{strconv.FormatInt(p.CreatedAt.Unix(), 10), p.Error},
			}
			process.Report.ExitedAt = p.CreatedAt.Unix()
			process.Report.ExitState = "failed"
		}
	}

	return process
}

// GetProcess returns the process with the given ID whereever it's running on the cluster
// @Summary List a process by its ID
// @Description List a process by its ID. Use the filter parameter to specifiy the level of detail of the output.
// @Tags v16.?.?
// @ID cluster-3-get-process
// @Produce json
// @Param id path string true "Process ID"
// @Param domain query string false "Domain to act on"
// @Param filter query string false "Comma separated list of fields (config, state, report, metadata) to be part of the output. If empty, all fields will be part of the output"
// @Success 200 {object} api.Process
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/process/{id} [get]
func (h *ClusterHandler) GetProcess(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	id := util.PathParam(c, "id")
	filter := newFilter(util.DefaultQuery(c, "filter", ""))
	domain := util.DefaultQuery(c, "domain", "")

	if !h.iam.Enforce(ctxuser, domain, "process:"+id, "read") {
		return api.Err(http.StatusForbidden, "")
	}

	procs := h.proxy.ListProcesses(proxy.ProcessListOptions{
		ID:     []string{id},
		Filter: filter.Slice(),
		Domain: domain,
	})

	if len(procs) == 0 {
		// Check the store in the store for an undeployed process
		p, err := h.cluster.GetProcess(app.NewProcessID(id, domain))
		if err != nil {
			return api.Err(http.StatusNotFound, "", "Unknown process ID: %s", id)
		}

		process := h.convertStoreProcessToAPIProcess(p, filter)

		return c.JSON(http.StatusOK, process)
	}

	if procs[0].Domain != domain {
		return api.Err(http.StatusNotFound, "", "Unknown process ID: %s", id)
	}

	return c.JSON(http.StatusOK, procs[0])
}

// Add adds a new process to the cluster
// @Summary Add a new process
// @Description Add a new FFmpeg process
// @Tags v16.?.?
// @ID cluster-3-add-process
// @Accept json
// @Produce json
// @Param config body api.ProcessConfig true "Process config"
// @Success 200 {object} api.ProcessConfig
// @Failure 400 {object} api.Error
// @Failure 403 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/process [post]
func (h *ClusterHandler) AddProcess(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	superuser := util.DefaultContext(c, "superuser", false)

	process := api.ProcessConfig{
		ID:        shortuuid.New(),
		Owner:     ctxuser,
		Type:      "ffmpeg",
		Autostart: true,
	}

	if err := util.ShouldBindJSON(c, &process); err != nil {
		return api.Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	if !h.iam.Enforce(ctxuser, process.Domain, "process:"+process.ID, "write") {
		return api.Err(http.StatusForbidden, "", "API user %s is not allowed to write this process in domain %s", ctxuser, process.Domain)
	}

	if !superuser {
		if !h.iam.Enforce(process.Owner, process.Domain, "process:"+process.ID, "write") {
			return api.Err(http.StatusForbidden, "", "user %s is not allowed to write this process in domain %s", process.Owner, process.Domain)
		}
	}

	if process.Type != "ffmpeg" {
		return api.Err(http.StatusBadRequest, "", "unsupported process type: supported process types are: ffmpeg")
	}

	if len(process.Input) == 0 || len(process.Output) == 0 {
		return api.Err(http.StatusBadRequest, "", "At least one input and one output need to be defined")
	}

	config, metadata := process.Marshal()

	if err := h.cluster.AddProcess("", config); err != nil {
		return api.Err(http.StatusBadRequest, "", "adding process config: %s", err.Error())
	}

	for key, value := range metadata {
		h.cluster.SetProcessMetadata("", config.ProcessID(), key, value)
	}

	return c.JSON(http.StatusOK, process)
}

// Update replaces an existing process
// @Summary Replace an existing process
// @Description Replace an existing process.
// @Tags v16.?.?
// @ID cluster-3-update-process
// @Accept json
// @Produce json
// @Param id path string true "Process ID"
// @Param domain query string false "Domain to act on"
// @Param config body api.ProcessConfig true "Process config"
// @Success 200 {object} api.ProcessConfig
// @Failure 400 {object} api.Error
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/process/{id} [put]
func (h *ClusterHandler) UpdateProcess(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	superuser := util.DefaultContext(c, "superuser", false)
	domain := util.DefaultQuery(c, "domain", "")
	id := util.PathParam(c, "id")

	process := api.ProcessConfig{
		ID:        id,
		Owner:     ctxuser,
		Domain:    domain,
		Type:      "ffmpeg",
		Autostart: true,
	}

	if !h.iam.Enforce(ctxuser, domain, "process:"+id, "write") {
		return api.Err(http.StatusForbidden, "", "API user %s is not allowed to write the process in domain: %s", ctxuser, domain)
	}

	pid := process.ProcessID()

	current, err := h.cluster.GetProcess(pid)
	if err != nil {
		return api.Err(http.StatusNotFound, "", "process not found: %s in domain '%s'", pid.ID, pid.Domain)
	}

	// Prefill the config with the current values
	process.Unmarshal(current.Config)

	if err := util.ShouldBindJSON(c, &process); err != nil {
		return api.Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	if !h.iam.Enforce(ctxuser, process.Domain, "process:"+process.ID, "write") {
		return api.Err(http.StatusForbidden, "", "API user %s is not allowed to write this process", ctxuser)
	}

	if !superuser {
		if !h.iam.Enforce(process.Owner, process.Domain, "process:"+process.ID, "write") {
			return api.Err(http.StatusForbidden, "", "user %s is not allowed to write this process", process.Owner)
		}
	}

	config, metadata := process.Marshal()

	if err := h.cluster.UpdateProcess("", pid, config); err != nil {
		if err == restream.ErrUnknownProcess {
			return api.Err(http.StatusNotFound, "", "process not found: %s in domain '%s'", pid.ID, pid.Domain)
		}

		return api.Err(http.StatusBadRequest, "", "process can't be updated: %s", err.Error())
	}

	pid = process.ProcessID()

	for key, value := range metadata {
		h.cluster.SetProcessMetadata("", pid, key, value)
	}

	return c.JSON(http.StatusOK, process)
}

// Command issues a command to a process in the cluster
// @Summary Issue a command to a process in the cluster
// @Description Issue a command to a process: start, stop, reload, restart
// @Tags v16.?.?
// @ID cluster-3-set-process-command
// @Accept json
// @Produce json
// @Param id path string true "Process ID"
// @Param domain query string false "Domain to act on"
// @Param command body api.Command true "Process command"
// @Success 200 {string} string
// @Failure 400 {object} api.Error
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/process/{id}/command [put]
func (h *ClusterHandler) SetProcessCommand(c echo.Context) error {
	id := util.PathParam(c, "id")
	ctxuser := util.DefaultContext(c, "user", "")
	domain := util.DefaultQuery(c, "domain", "")

	if !h.iam.Enforce(ctxuser, domain, "process:"+id, "write") {
		return api.Err(http.StatusForbidden, "", "API user %s is not allowed to write the process in domain: %s", ctxuser, domain)
	}

	var command api.Command

	if err := util.ShouldBindJSON(c, &command); err != nil {
		return api.Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	pid := app.ProcessID{
		ID:     id,
		Domain: domain,
	}

	switch command.Command {
	case "start":
	case "stop":
	case "restart":
	case "reload":
	default:
		return api.Err(http.StatusBadRequest, "", "unknown command provided. known commands are: start, stop, reload, restart")
	}

	if err := h.cluster.SetProcessCommand("", pid, command.Command); err != nil {
		return api.Err(http.StatusNotFound, "", "command failed: %s", err.Error())
	}

	return c.JSON(http.StatusOK, "OK")
}

// SetProcessMetadata stores metadata with a process
// @Summary Add JSON metadata with a process under the given key
// @Description Add arbitrary JSON metadata under the given key. If the key exists, all already stored metadata with this key will be overwritten. If the key doesn't exist, it will be created.
// @Tags v16.?.?
// @ID cluster-3-set-process-metadata
// @Produce json
// @Param id path string true "Process ID"
// @Param key path string true "Key for data store"
// @Param domain query string false "Domain to act on"
// @Param data body api.Metadata true "Arbitrary JSON data. The null value will remove the key and its contents"
// @Success 200 {object} api.Metadata
// @Failure 400 {object} api.Error
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/process/{id}/metadata/{key} [put]
func (h *ClusterHandler) SetProcessMetadata(c echo.Context) error {
	id := util.PathParam(c, "id")
	key := util.PathParam(c, "key")
	ctxuser := util.DefaultContext(c, "user", "")
	domain := util.DefaultQuery(c, "domain", "")

	if !h.iam.Enforce(ctxuser, domain, "process:"+id, "write") {
		return api.Err(http.StatusForbidden, "", "API user %s is not allowed to write the process in domain: %s", ctxuser, domain)
	}

	if len(key) == 0 {
		return api.Err(http.StatusBadRequest, "", "invalid key: the key must not be of length 0")
	}

	var data api.Metadata

	if err := util.ShouldBindJSONValidation(c, &data, false); err != nil {
		return api.Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	pid := app.ProcessID{
		ID:     id,
		Domain: domain,
	}

	if err := h.cluster.SetProcessMetadata("", pid, key, data); err != nil {
		return api.Err(http.StatusNotFound, "", "setting metadata failed: %s", err.Error())
	}

	return c.JSON(http.StatusOK, data)
}

// GetProcessMetadata returns the metadata stored with a process
// @Summary Retrieve JSON metadata stored with a process under a key
// @Description Retrieve the previously stored JSON metadata under the given key. If the key is empty, all metadata will be returned.
// @Tags v16.?.?
// @ID cluster-3-get-process-metadata
// @Produce json
// @Param id path string true "Process ID"
// @Param key path string true "Key for data store"
// @Param domain query string false "Domain to act on"
// @Success 200 {object} api.Metadata
// @Failure 400 {object} api.Error
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/process/{id}/metadata/{key} [get]
func (h *ClusterHandler) GetProcessMetadata(c echo.Context) error {
	id := util.PathParam(c, "id")
	key := util.PathParam(c, "key")
	ctxuser := util.DefaultContext(c, "user", "")
	domain := util.DefaultQuery(c, "domain", "")

	if !h.iam.Enforce(ctxuser, domain, "process:"+id, "read") {
		return api.Err(http.StatusForbidden, "")
	}

	pid := app.ProcessID{
		ID:     id,
		Domain: domain,
	}

	data, err := h.cluster.GetProcessMetadata("", pid, key)
	if err != nil {
		return api.Err(http.StatusNotFound, "", "unknown process ID: %s", err.Error())
	}

	return c.JSON(http.StatusOK, data)
}

// Probe probes a process in the cluster
// @Summary Probe a process in the cluster
// @Description Probe an existing process to get a detailed stream information on the inputs.
// @Tags v16.?.?
// @ID cluster-3-process-probe
// @Produce json
// @Param id path string true "Process ID"
// @Param domain query string false "Domain to act on"
// @Success 200 {object} api.Probe
// @Failure 403 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/process/{id}/probe [get]
func (h *ClusterHandler) ProbeProcess(c echo.Context) error {
	id := util.PathParam(c, "id")
	ctxuser := util.DefaultContext(c, "user", "")
	domain := util.DefaultQuery(c, "domain", "")

	if !h.iam.Enforce(ctxuser, domain, "process:"+id, "write") {
		return api.Err(http.StatusForbidden, "")
	}

	pid := app.ProcessID{
		ID:     id,
		Domain: domain,
	}

	nodeid, err := h.proxy.FindNodeFromProcess(pid)
	if err != nil {
		return c.JSON(http.StatusOK, api.Probe{
			Log: []string{fmt.Sprintf("the process can't be found: %s", err.Error())},
		})
	}

	probe, _ := h.proxy.ProbeProcess(nodeid, pid)

	return c.JSON(http.StatusOK, probe)
}

// Delete deletes the process with the given ID from the cluster
// @Summary Delete a process by its ID
// @Description Delete a process by its ID
// @Tags v16.?.?
// @ID cluster-3-delete-process
// @Produce json
// @Param id path string true "Process ID"
// @Success 200 {string} string
// @Failure 400 {object} api.Error
// @Failure 403 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/process/{id} [delete]
func (h *ClusterHandler) DeleteProcess(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	domain := util.DefaultQuery(c, "domain", "")
	id := util.PathParam(c, "id")

	if !h.iam.Enforce(ctxuser, domain, "process:"+id, "write") {
		return api.Err(http.StatusForbidden, "", "API user %s is not allowed to write the process in domain: %s", ctxuser, domain)
	}

	pid := app.ProcessID{
		ID:     id,
		Domain: domain,
	}

	if err := h.cluster.RemoveProcess("", pid); err != nil {
		return api.Err(http.StatusBadRequest, "", "%s", err.Error())
	}

	return c.JSON(http.StatusOK, "OK")
}
