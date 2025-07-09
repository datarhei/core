package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/datarhei/core/v16/cluster/node"
	"github.com/datarhei/core/v16/cluster/store"
	"github.com/datarhei/core/v16/glob"
	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/handler/util"
	"github.com/datarhei/core/v16/restream"
	"github.com/datarhei/core/v16/restream/app"
	"github.com/labstack/echo/v4"
	"github.com/lithammer/shortuuid/v4"
)

// ProcessList returns the list of processes running on all nodes of the cluster
// @Summary List of processes in the cluster
// @Description List of processes in the cluster
// @Tags v16.?.?
// @ID cluster-3-get-all-processes
// @Produce json
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
func (h *ClusterHandler) ProcessList(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	filter := newFilter(util.DefaultQuery(c, "filter", ""))
	reference := util.DefaultQuery(c, "reference", "")
	wantids := strings.FieldsFunc(util.DefaultQuery(c, "id", ""), func(r rune) bool {
		return r == rune(',')
	})
	idpattern := util.DefaultQuery(c, "idpattern", "")
	refpattern := util.DefaultQuery(c, "refpattern", "")
	ownerpattern := util.DefaultQuery(c, "ownerpattern", "")
	domainpattern := util.DefaultQuery(c, "domainpattern", "")

	procs := h.proxy.ProcessList(node.ProcessListOptions{
		ID:            wantids,
		Filter:        filter.Slice(),
		Reference:     reference,
		IDPattern:     idpattern,
		RefPattern:    refpattern,
		OwnerPattern:  ownerpattern,
		DomainPattern: domainpattern,
	})

	pmap := map[app.ProcessID]api.Process{}

	for _, p := range procs {
		if !h.iam.Enforce(ctxuser, p.Domain, "process", p.ID, "read") {
			continue
		}

		pmap[app.NewProcessID(p.ID, p.Domain)] = p
	}

	missing := []api.Process{}

	// Here we have to add those processes that are in the cluster DB and couldn't be deployed
	{
		processes := h.cluster.Store().ProcessList()
		filtered := h.getFilteredStoreProcesses(processes, wantids, reference, idpattern, refpattern, ownerpattern, domainpattern)

		for _, p := range filtered {
			if !h.iam.Enforce(ctxuser, p.Config.Domain, "process", p.Config.ID, "read") {
				continue
			}

			// Check if the process has been deployed
			if len(p.Error) == 0 {
				if _, ok := pmap[p.Config.ProcessID()]; ok {
					continue
				}
			} else {
				delete(pmap, p.Config.ProcessID())
			}

			process := api.Process{}
			process.UnmarshalStore(p, filter.config, filter.state, filter.report, filter.metadata)

			missing = append(missing, process)
		}
	}

	processes := []api.Process{}
	for _, p := range pmap {
		processes = append(processes, p)
	}

	processes = append(processes, missing...)

	return c.JSON(http.StatusOK, processes)
}

func (h *ClusterHandler) getFilteredStoreProcesses(processes []store.Process, wantids []string, reference, idpattern, refpattern, ownerpattern, domainpattern string) []store.Process {
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

// ProcessGet returns the process with the given ID whereever it's running on the cluster
// @Summary List a process by its ID
// @Description List a process by its ID. Use the filter parameter to specifiy the level of detail of the output.
// @Tags v16.?.?
// @ID cluster-3-get-process
// @Produce json
// @Param id path string true "Process ID"
// @Param domain query string false "Process domain"
// @Param filter query string false "Comma separated list of fields (config, state, report, metadata) to be part of the output. If empty, all fields will be part of the output"
// @Success 200 {object} api.Process
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/process/{id} [get]
func (h *ClusterHandler) ProcessGet(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	id := util.PathParam(c, "id")
	filter := newFilter(util.DefaultQuery(c, "filter", ""))
	domain := util.DefaultQuery(c, "domain", "")

	if !h.iam.Enforce(ctxuser, domain, "process", id, "read") {
		return api.Err(http.StatusForbidden, "")
	}

	pid := app.NewProcessID(id, domain)

	// Check the store for the process
	p, nodeid, err := h.cluster.ProcessGet("", pid, false)
	if err != nil {
		return api.Err(http.StatusNotFound, "", "process not found: %s in domain '%s'", pid.ID, pid.Domain)
	}

	process := api.Process{}
	process.UnmarshalStore(p, filter.config, filter.state, filter.report, filter.metadata)

	// Get the actual process data
	if len(nodeid) != 0 {
		process, err = h.proxy.ProcessGet(nodeid, pid, filter.Slice())
		if err != nil {
			return api.Err(http.StatusNotFound, "", "process not found: %s in domain '%s'", pid.ID, pid.Domain)
		}
	}

	return c.JSON(http.StatusOK, process)
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
func (h *ClusterHandler) ProcessAdd(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")

	process := api.ProcessConfig{
		ID:        shortuuid.New(),
		Owner:     ctxuser,
		Type:      "ffmpeg",
		Autostart: true,
	}

	if err := util.ShouldBindJSON(c, &process); err != nil {
		return api.Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	if !h.iam.Enforce(ctxuser, process.Domain, "process", process.ID, "write") {
		return api.Err(http.StatusForbidden, "", "API user %s is not allowed to write this process in domain %s", ctxuser, process.Domain)
	}

	if process.Type != "ffmpeg" {
		return api.Err(http.StatusBadRequest, "", "unsupported process type: supported process types are: ffmpeg")
	}

	if len(process.Input) == 0 || len(process.Output) == 0 {
		return api.Err(http.StatusBadRequest, "", "At least one input and one output need to be defined")
	}

	config, metadata := process.Marshal()

	if err := h.cluster.ProcessAdd("", config); err != nil {
		return api.Err(http.StatusBadRequest, "", "adding process config: %s", err.Error())
	}

	for key, value := range metadata {
		h.cluster.ProcessSetMetadata("", config.ProcessID(), key, value)
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
// @Param domain query string false "Process domain"
// @Param config body api.ProcessConfig true "Process config"
// @Success 200 {object} api.ProcessConfig
// @Failure 400 {object} api.Error
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/process/{id} [put]
func (h *ClusterHandler) ProcessUpdate(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	domain := util.DefaultQuery(c, "domain", "")
	id := util.PathParam(c, "id")

	process := api.ProcessConfig{
		ID:        id,
		Owner:     ctxuser,
		Domain:    domain,
		Type:      "ffmpeg",
		Autostart: true,
	}

	if !h.iam.Enforce(ctxuser, domain, "process", id, "write") {
		return api.Err(http.StatusForbidden, "", "API user %s is not allowed to write the process in domain: %s", ctxuser, domain)
	}

	pid := process.ProcessID()

	current, _, err := h.cluster.ProcessGet("", pid, false)
	if err != nil {
		return api.Err(http.StatusNotFound, "", "process not found: %s in domain '%s'", pid.ID, pid.Domain)
	}

	// Prefill the config with the current values
	process.Unmarshal(current.Config, nil)

	if err := util.ShouldBindJSON(c, &process); err != nil {
		return api.Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	if !h.iam.Enforce(ctxuser, process.Domain, "process", process.ID, "write") {
		return api.Err(http.StatusForbidden, "", "API user %s is not allowed to write this process", ctxuser)
	}

	config, metadata := process.Marshal()

	if err := h.cluster.ProcessUpdate("", pid, config); err != nil {
		if err == restream.ErrUnknownProcess {
			return api.Err(http.StatusNotFound, "", "process not found: %s in domain '%s'", pid.ID, pid.Domain)
		}

		return api.Err(http.StatusBadRequest, "", "process can't be updated: %s", err.Error())
	}

	pid = process.ProcessID()

	for key, value := range metadata {
		h.cluster.ProcessSetMetadata("", pid, key, value)
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
// @Param domain query string false "Process domain"
// @Param command body api.Command true "Process command"
// @Success 200 {string} string
// @Failure 400 {object} api.Error
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/process/{id}/command [put]
func (h *ClusterHandler) ProcessSetCommand(c echo.Context) error {
	id := util.PathParam(c, "id")
	ctxuser := util.DefaultContext(c, "user", "")
	domain := util.DefaultQuery(c, "domain", "")

	if !h.iam.Enforce(ctxuser, domain, "process", id, "write") {
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

	if err := h.cluster.ProcessSetCommand("", pid, command.Command); err != nil {
		if cerr, ok := err.(api.Error); ok {
			return cerr
		}
		return api.Err(http.StatusNotFound, "", "command failed: %s", err.Error())
	}

	return c.JSON(http.StatusOK, "OK")
}

// ProcessSetMetadata stores metadata with a process
// @Summary Add JSON metadata with a process under the given key
// @Description Add arbitrary JSON metadata under the given key. If the key exists, all already stored metadata with this key will be overwritten. If the key doesn't exist, it will be created.
// @Tags v16.?.?
// @ID cluster-3-set-process-metadata
// @Produce json
// @Param id path string true "Process ID"
// @Param key path string true "Key for data store"
// @Param domain query string false "Process domain"
// @Param data body api.Metadata true "Arbitrary JSON data. The null value will remove the key and its contents"
// @Success 200 {object} api.Metadata
// @Failure 400 {object} api.Error
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/process/{id}/metadata/{key} [put]
func (h *ClusterHandler) ProcessSetMetadata(c echo.Context) error {
	id := util.PathParam(c, "id")
	key := util.PathParam(c, "key")
	ctxuser := util.DefaultContext(c, "user", "")
	domain := util.DefaultQuery(c, "domain", "")

	if !h.iam.Enforce(ctxuser, domain, "process", id, "write") {
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

	if err := h.cluster.ProcessSetMetadata("", pid, key, data); err != nil {
		return api.Err(http.StatusNotFound, "", "setting metadata failed: %s", err.Error())
	}

	return c.JSON(http.StatusOK, data)
}

// ProcessGetMetadata returns the metadata stored with a process
// @Summary Retrieve JSON metadata stored with a process under a key
// @Description Retrieve the previously stored JSON metadata under the given key. If the key is empty, all metadata will be returned.
// @Tags v16.?.?
// @ID cluster-3-get-process-metadata
// @Produce json
// @Param id path string true "Process ID"
// @Param key path string true "Key for data store"
// @Param domain query string false "Process domain"
// @Success 200 {object} api.Metadata
// @Failure 400 {object} api.Error
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/process/{id}/metadata/{key} [get]
func (h *ClusterHandler) ProcessGetMetadata(c echo.Context) error {
	id := util.PathParam(c, "id")
	key := util.PathParam(c, "key")
	ctxuser := util.DefaultContext(c, "user", "")
	domain := util.DefaultQuery(c, "domain", "")

	if !h.iam.Enforce(ctxuser, domain, "process", id, "read") {
		return api.Err(http.StatusForbidden, "")
	}

	pid := app.ProcessID{
		ID:     id,
		Domain: domain,
	}

	data, err := h.cluster.ProcessGetMetadata("", pid, key)
	if err != nil {
		return api.Err(http.StatusNotFound, "", "unknown process ID: %s", err.Error())
	}

	return c.JSON(http.StatusOK, data)
}

// Probe probes a process in the cluster
// @Summary Probe a process in the cluster
// @Description Probe an existing process to get a detailed stream information on the inputs. The probe is executed on the same node as the process.
// @Tags v16.?.?
// @ID cluster-3-process-probe
// @Produce json
// @Param id path string true "Process ID"
// @Param domain query string false "Process domain"
// @Success 200 {object} api.Probe
// @Failure 403 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/process/{id}/probe [get]
func (h *ClusterHandler) ProcessProbe(c echo.Context) error {
	id := util.PathParam(c, "id")
	ctxuser := util.DefaultContext(c, "user", "")
	domain := util.DefaultQuery(c, "domain", "")

	if !h.iam.Enforce(ctxuser, domain, "process", id, "write") {
		return api.Err(http.StatusForbidden, "")
	}

	pid := app.ProcessID{
		ID:     id,
		Domain: domain,
	}

	nodeid, err := h.cluster.Store().ProcessGetNode(pid)
	if err != nil {
		return c.JSON(http.StatusOK, api.Probe{
			Log: []string{fmt.Sprintf("the process can't be found: %s", err.Error())},
		})
	}

	probe, _ := h.proxy.ProcessProbe(nodeid, pid)

	return c.JSON(http.StatusOK, probe)
}

// Probe probes a process in the cluster
// @Summary Probe a process in the cluster
// @Description Probe a process config to get a detailed stream information on the inputs.
// @Tags v16.?.?
// @ID cluster-3-probe-process-config
// @Accept json
// @Produce json
// @Param config body api.ProcessConfig true "Process config"
// @Param coreid query string false "Core to execute the probe on"
// @Success 200 {object} api.Probe
// @Failure 400 {object} api.Error
// @Failure 403 {object} api.Error
// @Failure 500 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/process/probe [post]
func (h *ClusterHandler) ProcessProbeConfig(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	coreid := util.DefaultQuery(c, "coreid", "")

	process := api.ProcessConfig{
		ID:        shortuuid.New(),
		Owner:     ctxuser,
		Type:      "ffmpeg",
		Autostart: true,
	}

	if err := util.ShouldBindJSON(c, &process); err != nil {
		return api.Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	if !h.iam.Enforce(ctxuser, process.Domain, "process", process.ID, "write") {
		return api.Err(http.StatusForbidden, "", "API user %s is not allowed to write this process in domain %s", ctxuser, process.Domain)
	}

	if process.Type != "ffmpeg" {
		return api.Err(http.StatusBadRequest, "", "unsupported process type: supported process types are: ffmpeg")
	}

	if len(process.Input) == 0 {
		return api.Err(http.StatusBadRequest, "", "At least one input must be defined")
	}

	if process.Limits.CPU <= 0 || process.Limits.Memory == 0 {
		return api.Err(http.StatusBadRequest, "", "Resource limit must be defined")
	}

	config, _ := process.Marshal()

	coreid = h.proxy.FindNodeForResources(coreid, config.LimitCPU, config.LimitMemory)
	if len(coreid) == 0 {
		return api.Err(http.StatusInternalServerError, "", "Not enough resources available to execute probe")
	}

	probe, _ := h.proxy.ProcessProbeConfig(coreid, config)

	return c.JSON(http.StatusOK, probe)
}

// Delete deletes the process with the given ID from the cluster
// @Summary Delete a process by its ID
// @Description Delete a process by its ID
// @Tags v16.?.?
// @ID cluster-3-delete-process
// @Produce json
// @Param id path string true "Process ID"
// @Param domain query string false "Process domain"
// @Success 200 {string} string
// @Failure 400 {object} api.Error
// @Failure 403 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/process/{id} [delete]
func (h *ClusterHandler) ProcessDelete(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	domain := util.DefaultQuery(c, "domain", "")
	id := util.PathParam(c, "id")

	if !h.iam.Enforce(ctxuser, domain, "process", id, "write") {
		return api.Err(http.StatusForbidden, "", "API user %s is not allowed to write the process in domain: %s", ctxuser, domain)
	}

	pid := app.ProcessID{
		ID:     id,
		Domain: domain,
	}

	if err := h.cluster.ProcessRemove("", pid); err != nil {
		return api.Err(http.StatusBadRequest, "", "%s", err.Error())
	}

	return c.JSON(http.StatusOK, "OK")
}
