package api

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/handler/util"
	"github.com/datarhei/core/v16/iam"
	"github.com/datarhei/core/v16/restream"
	"github.com/datarhei/core/v16/restream/app"

	"github.com/labstack/echo/v4"
	"github.com/lithammer/shortuuid/v4"
)

// The ProcessHandler type provides functions to interact with a Restreamer instance
type ProcessHandler struct {
	restream restream.Restreamer
	iam      iam.Enforcer
}

// NewProcess return a new Restream type. You have to provide a valid Restreamer instance.
func NewProcess(restream restream.Restreamer, iam iam.IAM) *ProcessHandler {
	return &ProcessHandler{
		restream: restream,
		iam:      iam,
	}
}

// Add adds a new process
// @Summary Add a new process
// @Description Add a new FFmpeg process
// @Tags v16.7.2
// @ID process-3-add
// @Accept json
// @Produce json
// @Param config body api.ProcessConfig true "Process config"
// @Success 200 {object} api.ProcessConfig
// @Failure 400 {object} api.Error
// @Failure 403 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/process [post]
func (h *ProcessHandler) Add(c echo.Context) error {
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

	if !h.iam.Enforce(ctxuser, process.Domain, "process", process.ID, "write") {
		return api.Err(http.StatusForbidden, "", "You are not allowed to write this process, check the domain and process ID")
	}

	if !superuser {
		if !h.iam.Enforce(process.Owner, process.Domain, "process", process.ID, "write") {
			return api.Err(http.StatusForbidden, "", "The owner '%s' is not allowed to write this process", process.Owner)
		}
	}

	if process.Type != "ffmpeg" {
		return api.Err(http.StatusBadRequest, "", "unsupported process type, supported process types are: ffmpeg")
	}

	if len(process.Input) == 0 || len(process.Output) == 0 {
		return api.Err(http.StatusBadRequest, "", "at least one input and one output need to be defined")
	}

	config, metadata := process.Marshal()

	if err := h.restream.AddProcess(config); err != nil {
		return api.Err(http.StatusBadRequest, "", "invalid process config: %s", err.Error())
	}

	tid := app.ProcessID{
		ID:     config.ID,
		Domain: config.Domain,
	}

	for key, data := range metadata {
		h.restream.SetProcessMetadata(tid, key, data)
	}

	p, _ := h.getProcess(tid, newFilter("config"))

	return c.JSON(http.StatusOK, p.Config)
}

// GetAll returns all known processes
// @Summary List all known processes
// @Description List all known processes. Use the query parameter to filter the listed processes.
// @Tags v16.7.2
// @ID process-3-get-all
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
// @Router /api/v3/process [get]
func (h *ProcessHandler) GetAll(c echo.Context) error {
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

	ids := h.restream.GetProcessIDs(idpattern, refpattern, ownerpattern, domainpattern)

	if len(wantids) != 0 {
		filteredIds := []app.ProcessID{}

		for _, id := range ids {
			for _, wantid := range wantids {
				if wantid == id.ID {
					filteredIds = append(filteredIds, id)
					break
				}
			}
		}

		ids = filteredIds
	}

	processes := make([]*api.Process, 0, len(ids))

	hasReference := len(reference) != 0 && len(wantids) == 0

	processesMu := sync.Mutex{}
	idChan := make(chan app.ProcessID, len(ids))

	wg := sync.WaitGroup{}

	for i := 0; i < 8; /*runtime.NumCPU()*/ i++ {
		wg.Add(1)

		go func(idChan <-chan app.ProcessID) {
			defer wg.Done()

			for id := range idChan {
				if !h.iam.Enforce(ctxuser, domain, "process", id.ID, "read") {
					continue
				}

				process, err := h.getProcess(id, filter)
				if err != nil {
					continue
				}

				if hasReference && process.Reference != reference {
					continue
				}

				processesMu.Lock()
				processes = append(processes, &process)
				processesMu.Unlock()
			}
		}(idChan)
	}

	for _, id := range ids {
		idChan <- id
	}

	close(idChan)

	wg.Wait()

	return c.JSON(http.StatusOK, processes)
}

// Get returns the process with the given ID
// @Summary List a process by its ID
// @Description List a process by its ID. Use the filter parameter to specifiy the level of detail of the output.
// @Tags v16.7.2
// @ID process-3-get
// @Produce json
// @Param id path string true "Process ID"
// @Param domain query string false "Domain to act on"
// @Param filter query string false "Comma separated list of fields (config, state, report, metadata) to be part of the output. If empty, all fields will be part of the output"
// @Success 200 {object} api.Process
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/process/{id} [get]
func (h *ProcessHandler) Get(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	id := util.PathParam(c, "id")
	filter := util.DefaultQuery(c, "filter", "")
	domain := util.DefaultQuery(c, "domain", "")

	if !h.iam.Enforce(ctxuser, domain, "process", id, "read") {
		return api.Err(http.StatusForbidden, "Forbidden")
	}

	tid := app.ProcessID{
		ID:     id,
		Domain: domain,
	}

	p, err := h.getProcess(tid, newFilter(filter))
	if err != nil {
		return api.Err(http.StatusNotFound, "", "unknown process ID: %s", err.Error())
	}

	return c.JSON(http.StatusOK, p)
}

// Delete deletes the process with the given ID
// @Summary Delete a process by its ID
// @Description Delete a process by its ID
// @Tags v16.7.2
// @ID process-3-delete
// @Produce json
// @Param id path string true "Process ID"
// @Param domain query string false "Domain to act on"
// @Success 200 {string} string
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/process/{id} [delete]
func (h *ProcessHandler) Delete(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	superuser := util.DefaultContext(c, "superuser", false)
	id := util.PathParam(c, "id")
	domain := util.DefaultQuery(c, "domain", "")

	tid := app.ProcessID{
		ID:     id,
		Domain: domain,
	}

	if !superuser {
		if !h.iam.Enforce(ctxuser, domain, "process", id, "write") {
			return api.Err(http.StatusForbidden, "")
		}
	}

	if err := h.restream.StopProcess(tid); err != nil {
		return api.Err(http.StatusNotFound, "", "unknown process ID: %s", err.Error())
	}

	if err := h.restream.DeleteProcess(tid); err != nil {
		return api.Err(http.StatusInternalServerError, "", "process can't be deleted: %s", err.Error())
	}

	return c.JSON(http.StatusOK, "OK")
}

// Update replaces an existing process
// @Summary Replace an existing process
// @Description Replace an existing process.
// @Tags v16.7.2
// @ID process-3-update
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
// @Router /api/v3/process/{id} [put]
func (h *ProcessHandler) Update(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	superuser := util.DefaultContext(c, "superuser", false)
	domain := util.DefaultQuery(c, "domain", "")
	id := util.PathParam(c, "id")

	process := api.ProcessConfig{
		ID:        id,
		Owner:     ctxuser,
		Type:      "ffmpeg",
		Autostart: true,
	}

	if !h.iam.Enforce(ctxuser, domain, "process", id, "write") {
		return api.Err(http.StatusForbidden, "", "You are not allowed to write this process: %s", id)
	}

	tid := app.ProcessID{
		ID:     id,
		Domain: domain,
	}

	current, err := h.restream.GetProcess(tid)
	if err != nil {
		return api.Err(http.StatusNotFound, "")
	}

	// Prefill the config with the current values
	process.Unmarshal(current.Config, nil)

	if err := util.ShouldBindJSON(c, &process); err != nil {
		return api.Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	if !h.iam.Enforce(ctxuser, process.Domain, "process", process.ID, "write") {
		return api.Err(http.StatusForbidden, "", "You are not allowed to write this process: %s", process.ID)
	}

	if !superuser {
		if !h.iam.Enforce(process.Owner, process.Domain, "process", process.ID, "write") {
			return api.Err(http.StatusForbidden, "", "The owner '%s' is not allowed to write this process: %s", process.Owner, process.ID)
		}
	}

	config, metadata := process.Marshal()

	if err := h.restream.UpdateProcess(tid, config); err != nil {
		if err == restream.ErrUnknownProcess {
			return api.Err(http.StatusNotFound, "", "process not found: %s", id)
		}

		return api.Err(http.StatusBadRequest, "", "process can't be updated: %s", err.Error())
	}

	tid = app.ProcessID{
		ID:     config.ID,
		Domain: config.Domain,
	}

	for key, data := range metadata {
		h.restream.SetProcessMetadata(tid, key, data)
	}

	return c.JSON(http.StatusOK, process)
}

// Command issues a command to a process
// @Summary Issue a command to a process
// @Description Issue a command to a process: start, stop, reload, restart
// @Tags v16.7.2
// @ID process-3-command
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
// @Router /api/v3/process/{id}/command [put]
func (h *ProcessHandler) Command(c echo.Context) error {
	id := util.PathParam(c, "id")
	ctxuser := util.DefaultContext(c, "user", "")
	domain := util.DefaultQuery(c, "domain", "")

	if !h.iam.Enforce(ctxuser, domain, "process", id, "write") {
		return api.Err(http.StatusForbidden, "")
	}

	var command api.Command

	if err := util.ShouldBindJSON(c, &command); err != nil {
		return api.Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	tid := app.ProcessID{
		ID:     id,
		Domain: domain,
	}

	var err error
	if command.Command == "start" {
		err = h.restream.StartProcess(tid)
	} else if command.Command == "stop" {
		err = h.restream.StopProcess(tid)
	} else if command.Command == "restart" {
		err = h.restream.RestartProcess(tid)
	} else if command.Command == "reload" {
		err = h.restream.ReloadProcess(tid)
	} else {
		return api.Err(http.StatusBadRequest, "", "unknown command provided: known commands are: start, stop, reload, restart")
	}

	if err != nil {
		return api.Err(http.StatusBadRequest, "", "command failed: %s", err.Error())
	}

	return c.JSON(http.StatusOK, "OK")
}

// GetConfig returns the configuration of a process
// @Summary Get the configuration of a process
// @Description Get the configuration of a process. This is the configuration as provided by Add or Update.
// @Tags v16.7.2
// @ID process-3-get-config
// @Produce json
// @Param id path string true "Process ID"
// @Param domain query string false "Domain to act on"
// @Success 200 {object} api.ProcessConfig
// @Failure 400 {object} api.Error
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/process/{id}/config [get]
func (h *ProcessHandler) GetConfig(c echo.Context) error {
	id := util.PathParam(c, "id")
	ctxuser := util.DefaultContext(c, "user", "")
	domain := util.DefaultQuery(c, "domain", "")

	if !h.iam.Enforce(ctxuser, domain, "process", id, "read") {
		return api.Err(http.StatusForbidden, "")
	}

	tid := app.ProcessID{
		ID:     id,
		Domain: domain,
	}

	p, err := h.restream.GetProcess(tid)
	if err != nil {
		return api.Err(http.StatusNotFound, "", "unknown process ID: %s", err.Error())
	}

	config := api.ProcessConfig{}
	config.Unmarshal(p.Config, nil)

	return c.JSON(http.StatusOK, config)
}

// GetState returns the current state of a process
// @Summary Get the state of a process
// @Description Get the state and progress data of a process.
// @Tags v16.7.2
// @ID process-3-get-state
// @Produce json
// @Param id path string true "Process ID"
// @Param domain query string false "Domain to act on"
// @Success 200 {object} api.ProcessState
// @Failure 400 {object} api.Error
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/process/{id}/state [get]
func (h *ProcessHandler) GetState(c echo.Context) error {
	id := util.PathParam(c, "id")
	ctxuser := util.DefaultContext(c, "user", "")
	domain := util.DefaultQuery(c, "domain", "")

	if !h.iam.Enforce(ctxuser, domain, "process", id, "read") {
		return api.Err(http.StatusForbidden, "")
	}

	tid := app.ProcessID{
		ID:     id,
		Domain: domain,
	}

	s, err := h.restream.GetProcessState(tid)
	if err != nil {
		return api.Err(http.StatusNotFound, "", "unknown process ID: %s", err.Error())
	}

	state := api.ProcessState{}
	state.Unmarshal(s)

	return c.JSON(http.StatusOK, state)
}

// GetReport return the current log and the log history of a process
// @Summary Get the logs of a process
// @Description Get the logs and the log history of a process.
// @Tags v16.7.2
// @ID process-3-get-report
// @Produce json
// @Param id path string true "Process ID"
// @Param created_at query int64 false "Select only the report with that created_at date. Unix timestamp, leave empty for any. In combination with exited_at it denotes a range or reports."
// @Param exited_at query int64 false "Select only the report with that exited_at date. Unix timestamp, leave empty for any. In combination with created_at it denotes a range or reports."
// @Param domain query string false "Domain to act on"
// @Success 200 {object} api.ProcessReport
// @Failure 400 {object} api.Error
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/process/{id}/report [get]
func (h *ProcessHandler) GetReport(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	domain := util.DefaultQuery(c, "domain", "")
	id := util.PathParam(c, "id")
	createdUnix := util.DefaultQuery(c, "created_at", "")
	exitedUnix := util.DefaultQuery(c, "exited_at", "")

	var createdAt *int64 = nil
	var exitedAt *int64 = nil

	if len(createdUnix) != 0 {
		if x, err := strconv.ParseInt(createdUnix, 10, 64); err != nil {
			return api.Err(http.StatusBadRequest, "", "invalid created_at unix timestamp: %s", err.Error())
		} else {
			createdAt = &x
		}
	}

	if len(exitedUnix) != 0 {
		if x, err := strconv.ParseInt(exitedUnix, 10, 64); err != nil {
			return api.Err(http.StatusBadRequest, "", "invalid exited_at unix timestamp: %s", err.Error())
		} else {
			exitedAt = &x
		}
	}

	if !h.iam.Enforce(ctxuser, domain, "process", id, "read") {
		return api.Err(http.StatusForbidden, "")
	}

	tid := app.ProcessID{
		ID:     id,
		Domain: domain,
	}

	l, err := h.restream.GetProcessReport(tid)
	if err != nil {
		return api.Err(http.StatusNotFound, "", "unknown process ID: %s", err.Error())
	}

	report := api.ProcessReport{}
	report.Unmarshal(l)

	if createdAt == nil && exitedAt == nil {
		return c.JSON(http.StatusOK, report)
	}

	filteredReport := api.ProcessReport{}

	// Add the current report as a fake history entry
	report.History = append(report.History, api.ProcessReportHistoryEntry{
		ProcessReportEntry: api.ProcessReportEntry{
			CreatedAt: report.CreatedAt,
			Prelude:   report.Prelude,
			Log:       report.Log,
		},
	})

	entries := []api.ProcessReportHistoryEntry{}

	for _, r := range report.History {
		if createdAt != nil && exitedAt == nil {
			if r.CreatedAt == *createdAt {
				entries = append(entries, r)
			}
		} else if createdAt == nil && exitedAt != nil {
			if r.ExitedAt == *exitedAt {
				entries = append(entries, r)
			}
		} else {
			if r.CreatedAt >= *createdAt && r.ExitedAt <= *exitedAt {
				entries = append(entries, r)
			}
		}
	}

	if len(entries) == 0 {
		return api.Err(http.StatusNotFound, "", "No matching reports found")
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].CreatedAt > entries[j].CreatedAt
	})

	if entries[0].ExitState == "" {
		// This is a running process
		filteredReport.CreatedAt = entries[0].CreatedAt
		filteredReport.Prelude = entries[0].Prelude
		filteredReport.Log = entries[0].Log

		filteredReport.History = entries[1:]
	} else {
		filteredReport.History = entries
	}

	return c.JSON(http.StatusOK, filteredReport)
}

// SetReport sets the report history of a process
// @Summary Set the report history a process
// @Description Set the report history a process
// @Tags v16.?.?
// @ID process-3-set-report
// @Accept json
// @Produce json
// @Param id path string true "Process ID"
// @Param domain query string false "Domain to act on"
// @Param report body api.ProcessReport true "Process report"
// @Success 200 {string} string
// @Failure 400 {object} api.Error
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/process/{id}/report [put]
func (h *ProcessHandler) SetReport(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	domain := util.DefaultQuery(c, "domain", "")
	id := util.PathParam(c, "id")

	fmt.Printf("entering SetReport handler\n")

	if !h.iam.Enforce(ctxuser, domain, "process", id, "write") {
		return api.Err(http.StatusForbidden, "", "You are not allowed to write this process: %s", id)
	}

	tid := app.ProcessID{
		ID:     id,
		Domain: domain,
	}

	report := api.ProcessReport{}

	if err := util.ShouldBindJSON(c, &report); err != nil {
		return api.Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	appreport := report.Marshal()

	if err := h.restream.SetProcessReport(tid, &appreport); err != nil {
		return api.Err(http.StatusNotFound, "", "unknown process ID: %s", err.Error())
	}

	return c.JSON(http.StatusOK, "OK")
}

// SearchReportHistory returns a list of matching report references
// @Summary Search log history of all processes
// @Description Search log history of all processes by providing patterns for process IDs and references, a state and a time range. All are optional.
// @Tags v16.?.?
// @ID process-3-search-report-history
// @Produce json
// @Param idpattern query string false "Glob pattern for process IDs. If empty all IDs will be returned. Intersected with results from refpattern."
// @Param refpattern query string false "Glob pattern for process references. If empty all IDs will be returned. Intersected with results from idpattern."
// @Param state query string false "State of a process, leave empty for any"
// @Param from query int64 false "Search range of when the report has been exited, older than this value. Unix timestamp, leave empty for any"
// @Param to query int64 false "Search range of when the report has been exited, younger than this value. Unix timestamp, leave empty for any"
// @Success 200 {array} api.ProcessReportSearchResult
// @Failure 400 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/report/process [get]
func (h *ProcessHandler) SearchReportHistory(c echo.Context) error {
	idpattern := util.DefaultQuery(c, "idpattern", "")
	refpattern := util.DefaultQuery(c, "refpattern", "")
	state := util.DefaultQuery(c, "state", "")
	fromUnix := util.DefaultQuery(c, "from", "")
	toUnix := util.DefaultQuery(c, "to", "")

	var from, to *time.Time = nil, nil

	if len(fromUnix) != 0 {
		if x, err := strconv.ParseInt(fromUnix, 10, 64); err != nil {
			return api.Err(http.StatusBadRequest, "", "invalid search range: %s", err.Error())
		} else {
			t := time.Unix(x, 0)
			from = &t
		}
	}

	if len(toUnix) != 0 {
		if x, err := strconv.ParseInt(toUnix, 10, 64); err != nil {
			return api.Err(http.StatusBadRequest, "", "invalid search range: %s", err.Error())
		} else {
			t := time.Unix(x, 0)
			to = &t
		}
	}

	result := h.restream.SearchProcessLogHistory(idpattern, refpattern, state, from, to)

	response := make([]api.ProcessReportSearchResult, len(result))
	for i, b := range result {
		response[i].ProcessID = b.ProcessID
		response[i].Reference = b.Reference
		response[i].ExitState = b.ExitState
		response[i].CreatedAt = b.CreatedAt.Unix()
		response[i].ExitedAt = b.ExitedAt.Unix()
	}

	return c.JSON(http.StatusOK, response)
}

// Probe probes a known process
// @Summary Probe a known process
// @Description Probe an existing process to get a detailed stream information on the inputs.
// @Tags v16.7.2
// @ID process-3-probe
// @Produce json
// @Param id path string true "Process ID"
// @Param domain query string false "Domain to act on"
// @Success 200 {object} api.Probe
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/process/{id}/probe [get]
func (h *ProcessHandler) Probe(c echo.Context) error {
	id := util.PathParam(c, "id")
	ctxuser := util.DefaultContext(c, "user", "")
	domain := util.DefaultQuery(c, "domain", "")

	if !h.iam.Enforce(ctxuser, domain, "process", id, "write") {
		return api.Err(http.StatusForbidden, "")
	}

	tid := app.ProcessID{
		ID:     id,
		Domain: domain,
	}

	process, err := h.restream.GetProcess(tid)
	if err != nil {
		return api.Err(http.StatusNotFound, "")
	}

	probe := h.restream.Probe(process.Config, 20*time.Second)

	apiprobe := api.Probe{}
	apiprobe.Unmarshal(&probe)

	return c.JSON(http.StatusOK, apiprobe)
}

// ProbeConfig probes a process
// @Summary Add a new process
// @Description Probe a process to get a detailed stream information on the inputs.
// @Tags v16.?.?
// @ID process-3-probe-config
// @Accept json
// @Produce json
// @Param config body api.ProcessConfig true "Process config"
// @Success 200 {object} api.Probe
// @Failure 400 {object} api.Error
// @Failure 403 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/process/probe [post]
func (h *ProcessHandler) ProbeConfig(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")

	process := api.ProcessConfig{
		Owner: ctxuser,
		Type:  "ffmpeg",
	}

	if err := util.ShouldBindJSON(c, &process); err != nil {
		return api.Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	if !h.iam.Enforce(ctxuser, process.Domain, "process", process.ID, "write") {
		return api.Err(http.StatusForbidden, "", "You are not allowed to probe this process, check the domain and process ID")
	}

	if process.Type != "ffmpeg" {
		return api.Err(http.StatusBadRequest, "", "unsupported process type, supported process types are: ffmpeg")
	}

	if len(process.Input) == 0 {
		return api.Err(http.StatusBadRequest, "", "At least one input must be defined")
	}

	config, _ := process.Marshal()

	probe := h.restream.Probe(config, 20*time.Second)

	apiprobe := api.Probe{}
	apiprobe.Unmarshal(&probe)

	return c.JSON(http.StatusOK, apiprobe)
}

// Skills returns the detected FFmpeg capabilities
// @Summary FFmpeg capabilities
// @Description List all detected FFmpeg capabilities.
// @Tags v16.7.2
// @ID skills-3
// @Produce json
// @Success 200 {object} api.Skills
// @Security ApiKeyAuth
// @Router /api/v3/skills [get]
func (h *ProcessHandler) Skills(c echo.Context) error {
	skills := h.restream.Skills()

	apiskills := api.Skills{}
	apiskills.Unmarshal(skills)

	return c.JSON(http.StatusOK, apiskills)
}

// ReloadSkills will refresh the FFmpeg capabilities
// @Summary Refresh FFmpeg capabilities
// @Description Refresh the available FFmpeg capabilities.
// @Tags v16.7.2
// @ID skills-3-reload
// @Produce json
// @Success 200 {object} api.Skills
// @Security ApiKeyAuth
// @Router /api/v3/skills/reload [get]
func (h *ProcessHandler) ReloadSkills(c echo.Context) error {
	h.restream.ReloadSkills()
	skills := h.restream.Skills()

	apiskills := api.Skills{}
	apiskills.Unmarshal(skills)

	return c.JSON(http.StatusOK, apiskills)
}

// GetProcessMetadata returns the metadata stored with a process
// @Summary Retrieve JSON metadata stored with a process under a key
// @Description Retrieve the previously stored JSON metadata under the given key. If the key is empty, all metadata will be returned.
// @Tags v16.7.2
// @ID process-3-get-process-metadata
// @Produce json
// @Param id path string true "Process ID"
// @Param key path string true "Key for data store"
// @Param domain query string false "Domain to act on"
// @Success 200 {object} api.Metadata
// @Failure 400 {object} api.Error
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/process/{id}/metadata/{key} [get]
func (h *ProcessHandler) GetProcessMetadata(c echo.Context) error {
	id := util.PathParam(c, "id")
	key := util.PathParam(c, "key")
	ctxuser := util.DefaultContext(c, "user", "")
	domain := util.DefaultQuery(c, "domain", "")

	if !h.iam.Enforce(ctxuser, domain, "process", id, "read") {
		return api.Err(http.StatusForbidden, "")
	}

	tid := app.ProcessID{
		ID:     id,
		Domain: domain,
	}

	data, err := h.restream.GetProcessMetadata(tid, key)
	if err != nil {
		if err == restream.ErrMetadataKeyNotFound {
			return api.Err(http.StatusNotFound, "", "unknown key: %s", err.Error())
		}

		return api.Err(http.StatusNotFound, "", "unknown process ID: %s", err.Error())
	}

	return c.JSON(http.StatusOK, data)
}

// SetProcessMetadata stores metadata with a process
// @Summary Add JSON metadata with a process under the given key
// @Description Add arbitrary JSON metadata under the given key. If the key exists, all already stored metadata with this key will be overwritten. If the key doesn't exist, it will be created.
// @Tags v16.7.2
// @ID process-3-set-process-metadata
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
// @Router /api/v3/process/{id}/metadata/{key} [put]
func (h *ProcessHandler) SetProcessMetadata(c echo.Context) error {
	id := util.PathParam(c, "id")
	key := util.PathParam(c, "key")
	ctxuser := util.DefaultContext(c, "user", "")
	domain := util.DefaultQuery(c, "domain", "")

	if !h.iam.Enforce(ctxuser, domain, "process", id, "write") {
		return api.Err(http.StatusForbidden, "")
	}

	if len(key) == 0 {
		return api.Err(http.StatusBadRequest, "", "invalid key: the key must not be of length 0")
	}

	var data api.Metadata

	if err := util.ShouldBindJSONValidation(c, &data, false); err != nil {
		return api.Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	tid := app.ProcessID{
		ID:     id,
		Domain: domain,
	}

	if err := h.restream.SetProcessMetadata(tid, key, data); err != nil {
		return api.Err(http.StatusNotFound, "", "unknown process ID: %s", err.Error())
	}

	return c.JSON(http.StatusOK, data)
}

// GetMetadata returns the metadata stored with the Restreamer
// @Summary Retrieve JSON metadata from a key
// @Description Retrieve the previously stored JSON metadata under the given key. If the key is empty, all metadata will be returned.
// @Tags v16.7.2
// @ID metadata-3-get
// @Produce json
// @Param key path string true "Key for data store"
// @Success 200 {object} api.Metadata
// @Failure 404 {object} api.Error
// @Failure 400 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/metadata/{key} [get]
func (h *ProcessHandler) GetMetadata(c echo.Context) error {
	key := util.PathParam(c, "key")

	data, err := h.restream.GetMetadata(key)
	if err != nil {
		return api.Err(http.StatusNotFound, "", "metadata not found: %s", err.Error())
	}

	return c.JSON(http.StatusOK, data)
}

// SetMetadata stores metadata with the Restreamer
// @Summary Add JSON metadata under the given key
// @Description Add arbitrary JSON metadata under the given key. If the key exists, all already stored metadata with this key will be overwritten. If the key doesn't exist, it will be created.
// @Tags v16.7.2
// @ID metadata-3-set
// @Produce json
// @Param key path string true "Key for data store"
// @Param data body api.Metadata true "Arbitrary JSON data"
// @Success 200 {object} api.Metadata
// @Failure 400 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/metadata/{key} [put]
func (h *ProcessHandler) SetMetadata(c echo.Context) error {
	key := util.PathParam(c, "key")

	if len(key) == 0 {
		return api.Err(http.StatusBadRequest, "", "invalid key: the key must not be of length 0")
	}

	var data api.Metadata

	if err := util.ShouldBindJSONValidation(c, &data, false); err != nil {
		return api.Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	if err := h.restream.SetMetadata(key, data); err != nil {
		return api.Err(http.StatusBadRequest, "", "invalid metadata: %s", err.Error())
	}

	return c.JSON(http.StatusOK, data)
}

type filter struct {
	config   bool
	state    bool
	report   bool
	metadata bool
}

func newFilter(filterString string) filter {
	filters := strings.FieldsFunc(filterString, func(r rune) bool {
		return r == rune(',')
	})

	wants := map[string]bool{
		"config":   true,
		"state":    true,
		"report":   true,
		"metadata": true,
	}

	if len(filters) != 0 {
		for k := range wants {
			wants[k] = false
		}

		for _, f := range filters {
			if _, ok := wants[f]; ok {
				wants[f] = true
			}
		}
	}

	f := filter{
		config:   wants["config"],
		state:    wants["state"],
		report:   wants["report"],
		metadata: wants["metadata"],
	}

	return f
}

func (f filter) Slice() []string {
	what := []string{}

	if f.config {
		what = append(what, "config")
	}

	if f.state {
		what = append(what, "state")
	}

	if f.report {
		what = append(what, "report")
	}

	if f.metadata {
		what = append(what, "metadata")
	}

	return what
}

func (f filter) String() string {
	return strings.Join(f.Slice(), ",")
}

func (h *ProcessHandler) getProcess(id app.ProcessID, filter filter) (api.Process, error) {
	process, err := h.restream.GetProcess(id)
	if err != nil {
		return api.Process{}, err
	}

	info := api.Process{
		ID:        process.ID,
		Owner:     process.Owner,
		Domain:    process.Domain,
		Reference: process.Reference,
		Type:      "ffmpeg",
		CoreID:    h.restream.ID(),
		CreatedAt: process.CreatedAt,
		UpdatedAt: process.UpdatedAt,
	}

	if filter.config {
		info.Config = &api.ProcessConfig{}
		info.Config.Unmarshal(process.Config, nil)
	}

	if filter.state {
		state, err := h.restream.GetProcessState(id)
		if err != nil {
			return api.Process{}, err
		}

		info.State = &api.ProcessState{}
		info.State.Unmarshal(state)
	}

	if filter.report {
		log, err := h.restream.GetProcessReport(id)
		if err != nil {
			return api.Process{}, err
		}

		info.Report = &api.ProcessReport{}
		info.Report.Unmarshal(log)
	}

	if filter.metadata {
		data, err := h.restream.GetProcessMetadata(id, "")
		if err != nil {
			return api.Process{}, err
		}

		info.Metadata = api.NewMetadata(data)
	}

	return info, nil
}
