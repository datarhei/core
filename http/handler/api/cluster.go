package api

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/datarhei/core/v16/cluster"
	"github.com/datarhei/core/v16/cluster/proxy"
	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/handler/util"
	"github.com/datarhei/core/v16/iam"
	"github.com/datarhei/core/v16/iam/access"
	"github.com/datarhei/core/v16/iam/identity"
	"github.com/datarhei/core/v16/restream"
	"github.com/datarhei/core/v16/restream/app"

	clientapi "github.com/datarhei/core-client-go/v16/api"
	"github.com/labstack/echo/v4"
	"github.com/lithammer/shortuuid/v4"
)

// The ClusterHandler type provides handler functions for manipulating the cluster config.
type ClusterHandler struct {
	cluster cluster.Cluster
	proxy   proxy.ProxyReader
	iam     iam.IAM
}

// NewCluster return a new ClusterHandler type. You have to provide a cluster.
func NewCluster(cluster cluster.Cluster, iam iam.IAM) (*ClusterHandler, error) {
	h := &ClusterHandler{
		cluster: cluster,
		proxy:   cluster.ProxyReader(),
		iam:     iam,
	}

	if h.cluster == nil {
		return nil, fmt.Errorf("no cluster provided")
	}

	if h.iam == nil {
		return nil, fmt.Errorf("no IAM provided")
	}

	return h, nil
}

// GetCluster returns the list of nodes in the cluster
// @Summary List of nodes in the cluster
// @Description List of nodes in the cluster
// @Tags v16.?.?
// @ID cluster-3-get-cluster
// @Produce json
// @Success 200 {object} api.ClusterAbout
// @Security ApiKeyAuth
// @Router /api/v3/cluster [get]
func (h *ClusterHandler) About(c echo.Context) error {
	state, _ := h.cluster.About()

	about := api.ClusterAbout{
		ID:                state.ID,
		Address:           state.Address,
		ClusterAPIAddress: state.ClusterAPIAddress,
		CoreAPIAddress:    state.CoreAPIAddress,
		Raft: api.ClusterRaft{
			Server: []api.ClusterRaftServer{},
			Stats: api.ClusterRaftStats{
				State:       state.Raft.Stats.State,
				LastContact: state.Raft.Stats.LastContact.Seconds() * 1000,
				NumPeers:    state.Raft.Stats.NumPeers,
			},
		},
		Version:  state.Version.String(),
		Degraded: state.Degraded,
	}

	if state.DegradedErr != nil {
		about.DegradedErr = state.DegradedErr.Error()
	}

	for _, n := range state.Raft.Server {
		about.Raft.Server = append(about.Raft.Server, api.ClusterRaftServer{
			ID:      n.ID,
			Address: n.Address,
			Voter:   n.Voter,
			Leader:  n.Leader,
		})
	}

	for _, node := range state.Nodes {
		n := api.ClusterNode{}
		n.Marshal(node)

		about.Nodes = append(about.Nodes, n)
	}

	return c.JSON(http.StatusOK, about)
}

// Leave the cluster gracefully
// @Summary Leave the cluster gracefully
// @Description Leave the cluster gracefully
// @Tags v16.?.?
// @ID cluster-3-leave
// @Produce json
// @Success 200 {string} string
// @Failure 500 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/leave [put]
func (h *ClusterHandler) Leave(c echo.Context) error {
	h.cluster.Leave("", "")
	h.cluster.Shutdown()

	return c.JSON(http.StatusOK, "OK")
}

// ListAllNodesProcesses returns the list of processes running on all nodes of the cluster
// @Summary List of processes in the cluster
// @Description List of processes in the cluster
// @Tags v16.?.?
// @ID cluster-3-list-all-node-processes
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
func (h *ClusterHandler) ListAllNodesProcesses(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	filter := strings.FieldsFunc(util.DefaultQuery(c, "filter", ""), func(r rune) bool {
		return r == rune(',')
	})
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
		Filter:        filter,
		Domain:        domain,
		Reference:     reference,
		IDPattern:     idpattern,
		RefPattern:    refpattern,
		OwnerPattern:  ownerpattern,
		DomainPattern: domainpattern,
	})

	processes := []clientapi.Process{}

	for _, p := range procs {
		if !h.iam.Enforce(ctxuser, domain, "process:"+p.ID, "read") {
			continue
		}

		processes = append(processes, p)
	}

	return c.JSON(http.StatusOK, processes)
}

// GetAllNodesProcess returns the process with the given ID whereever it's running on the cluster
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
func (h *ClusterHandler) GetAllNodesProcess(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	id := util.PathParam(c, "id")
	filter := strings.FieldsFunc(util.DefaultQuery(c, "filter", ""), func(r rune) bool {
		return r == rune(',')
	})
	domain := util.DefaultQuery(c, "domain", "")

	if !h.iam.Enforce(ctxuser, domain, "process:"+id, "read") {
		return api.Err(http.StatusForbidden, "Forbidden")
	}

	procs := h.proxy.ListProcesses(proxy.ProcessListOptions{
		ID:     []string{id},
		Filter: filter,
		Domain: domain,
	})

	if len(procs) == 0 {
		return api.Err(http.StatusNotFound, "", "Unknown process ID: %s", id)
	}

	return c.JSON(http.StatusOK, procs[0])
}

// GetNodes returns the list of proxy nodes in the cluster
// @Summary List of proxy nodes in the cluster
// @Description List of proxy nodes in the cluster
// @Tags v16.?.?
// @ID cluster-3-get-nodes
// @Produce json
// @Success 200 {array} api.ClusterNode
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/node [get]
func (h *ClusterHandler) GetNodes(c echo.Context) error {
	nodes := h.proxy.ListNodes()

	list := []api.ClusterNode{}

	for _, node := range nodes {
		about := node.About()
		n := api.ClusterNode{}
		n.Marshal(about)

		list = append(list, n)
	}

	return c.JSON(http.StatusOK, list)
}

// GetNode returns the proxy node with the given ID
// @Summary List a proxy node by its ID
// @Description List a proxy node by its ID
// @Tags v16.?.?
// @ID cluster-3-get-node
// @Produce json
// @Param id path string true "Node ID"
// @Success 200 {object} api.ClusterNode
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/node/{id} [get]
func (h *ClusterHandler) GetNode(c echo.Context) error {
	id := util.PathParam(c, "id")

	peer, err := h.proxy.GetNode(id)
	if err != nil {
		return api.Err(http.StatusNotFound, "Node not found", "%s", err)
	}

	about := peer.About()

	node := api.ClusterNode{
		ID:          about.ID,
		Name:        about.Name,
		Address:     about.Address,
		CreatedAt:   about.CreatedAt.Format(time.RFC3339),
		Uptime:      int64(about.Uptime.Seconds()),
		LastContact: about.LastContact.Unix(),
		Latency:     about.Latency.Seconds() * 1000,
		State:       about.State,
		Resources:   api.ClusterNodeResources(about.Resources),
	}

	return c.JSON(http.StatusOK, node)
}

// GetNodeVersion returns the proxy node version with the given ID
// @Summary List a proxy node by its ID
// @Description List a proxy node by its ID
// @Tags v16.?.?
// @ID cluster-3-get-node-version
// @Produce json
// @Param id path string true "Node ID"
// @Success 200 {object} api.Version
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/node/{id}/version [get]
func (h *ClusterHandler) GetNodeVersion(c echo.Context) error {
	id := util.PathParam(c, "id")

	peer, err := h.proxy.GetNode(id)
	if err != nil {
		return api.Err(http.StatusNotFound, "Node not found", "%s", err)
	}

	v := peer.Version()

	version := api.Version{
		Number:   v.Number,
		Commit:   v.Commit,
		Branch:   v.Branch,
		Build:    v.Build.Format(time.RFC3339),
		Arch:     v.Arch,
		Compiler: v.Compiler,
	}

	return c.JSON(http.StatusOK, version)
}

// GetNodeFiles returns the files from the proxy node with the given ID
// @Summary List the files of a proxy node by its ID
// @Description List the files of a proxy node by its ID
// @Tags v16.?.?
// @ID cluster-3-get-node-files
// @Produce json
// @Param id path string true "Node ID"
// @Success 200 {object} api.ClusterNodeFiles
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/node/{id}/files [get]
func (h *ClusterHandler) GetNodeFiles(c echo.Context) error {
	id := util.PathParam(c, "id")

	peer, err := h.proxy.GetNode(id)
	if err != nil {
		return api.Err(http.StatusNotFound, "Node not found", "%s", err)
	}

	files := api.ClusterNodeFiles{
		Files: make(map[string][]string),
	}

	peerFiles := peer.Files()

	files.LastUpdate = peerFiles.LastUpdate.Unix()

	sort.Strings(peerFiles.Files)

	for _, path := range peerFiles.Files {
		prefix, path, found := strings.Cut(path, ":")
		if !found {
			continue
		}

		files.Files[prefix] = append(files.Files[prefix], path)
	}

	return c.JSON(http.StatusOK, files)
}

// ListNodeProcesses returns the list of processes running on a node of the cluster
// @Summary List of processes in the cluster on a node
// @Description List of processes in the cluster on a node
// @Tags v16.?.?
// @ID cluster-3-list-node-processes
// @Produce json
// @Param id path string true "Node ID"
// @Param domain query string false "Domain to act on"
// @Param filter query string false "Comma separated list of fields (config, state, report, metadata) that will be part of the output. If empty, all fields will be part of the output."
// @Param reference query string false "Return only these process that have this reference value. If empty, the reference will be ignored."
// @Param id query string false "Comma separated list of process ids to list. Overrides the reference. If empty all IDs will be returned."
// @Param idpattern query string false "Glob pattern for process IDs. If empty all IDs will be returned. Intersected with results from other pattern matches."
// @Param refpattern query string false "Glob pattern for process references. If empty all IDs will be returned. Intersected with results from other pattern matches."
// @Param ownerpattern query string false "Glob pattern for process owners. If empty all IDs will be returned. Intersected with results from other pattern matches."
// @Param domainpattern query string false "Glob pattern for process domain. If empty all IDs will be returned. Intersected with results from other pattern matches."
// @Success 200 {array} api.Process
// @Failure 404 {object} api.Error
// @Failure 500 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/node/{id}/process [get]
func (h *ClusterHandler) ListNodeProcesses(c echo.Context) error {
	id := util.PathParam(c, "id")
	ctxuser := util.DefaultContext(c, "user", "")
	filter := strings.FieldsFunc(util.DefaultQuery(c, "filter", ""), func(r rune) bool {
		return r == rune(',')
	})
	reference := util.DefaultQuery(c, "reference", "")
	wantids := strings.FieldsFunc(util.DefaultQuery(c, "id", ""), func(r rune) bool {
		return r == rune(',')
	})
	domain := util.DefaultQuery(c, "domain", "")
	idpattern := util.DefaultQuery(c, "idpattern", "")
	refpattern := util.DefaultQuery(c, "refpattern", "")
	ownerpattern := util.DefaultQuery(c, "ownerpattern", "")
	domainpattern := util.DefaultQuery(c, "domainpattern", "")

	peer, err := h.proxy.GetNode(id)
	if err != nil {
		return api.Err(http.StatusNotFound, "Node not found", "%s", err)
	}

	procs, err := peer.ProcessList(proxy.ProcessListOptions{
		ID:            wantids,
		Filter:        filter,
		Domain:        domain,
		Reference:     reference,
		IDPattern:     idpattern,
		RefPattern:    refpattern,
		OwnerPattern:  ownerpattern,
		DomainPattern: domainpattern,
	})
	if err != nil {
		return api.Err(http.StatusInternalServerError, "", "Node not available: %s", err)
	}

	processes := []clientapi.Process{}

	for _, p := range procs {
		if !h.iam.Enforce(ctxuser, domain, "process:"+p.Config.ID, "read") {
			continue
		}

		processes = append(processes, p)
	}

	return c.JSON(http.StatusOK, processes)
}

// ListStoreProcesses returns the list of processes stored in the DB of the cluster
// @Summary List of processes in the cluster
// @Description List of processes in the cluster
// @Tags v16.?.?
// @ID cluster-3-db-list-processes
// @Produce json
// @Success 200 {array} api.Process
// @Security ApiKeyAuth
// @Router /api/v3/cluster/db/process [get]
func (h *ClusterHandler) ListStoreProcesses(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	domain := util.DefaultQuery(c, "domain", "")

	procs := h.cluster.ListProcesses()

	processes := []api.Process{}

	for _, p := range procs {
		if !h.iam.Enforce(ctxuser, domain, "process:"+p.Config.ID, "read") {
			continue
		}

		process := api.Process{
			ID:        p.Config.ID,
			Owner:     p.Config.Owner,
			Domain:    p.Config.Domain,
			Type:      "ffmpeg",
			Reference: p.Config.Reference,
			CreatedAt: p.CreatedAt.Unix(),
			UpdatedAt: p.UpdatedAt.Unix(),
			Metadata:  p.Metadata,
		}

		config := &api.ProcessConfig{}
		config.Unmarshal(p.Config)

		process.Config = config

		processes = append(processes, process)
	}

	return c.JSON(http.StatusOK, processes)
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
		return api.Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
	}

	if !h.iam.Enforce(ctxuser, process.Domain, "process:"+process.ID, "write") {
		return api.Err(http.StatusForbidden, "Forbidden")
	}

	if !superuser {
		if !h.iam.Enforce(process.Owner, process.Domain, "process:"+process.ID, "write") {
			return api.Err(http.StatusForbidden, "Forbidden")
		}
	}

	if process.Type != "ffmpeg" {
		return api.Err(http.StatusBadRequest, "Unsupported process type", "Supported process types are: ffmpeg")
	}

	if len(process.Input) == 0 || len(process.Output) == 0 {
		return api.Err(http.StatusBadRequest, "At least one input and one output need to be defined")
	}

	config, metadata := process.Marshal()

	if err := h.cluster.AddProcess("", config); err != nil {
		return api.Err(http.StatusBadRequest, "Invalid process config", "%s", err.Error())
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
		return api.Err(http.StatusForbidden, "Forbidden")
	}

	pid := process.ProcessID()

	current, err := h.cluster.GetProcess(pid)
	if err != nil {
		return api.Err(http.StatusNotFound, "Process not found", "%s", id)
	}

	// Prefill the config with the current values
	process.Unmarshal(current.Config)

	if err := util.ShouldBindJSON(c, &process); err != nil {
		return api.Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
	}

	if !h.iam.Enforce(ctxuser, process.Domain, "process:"+process.ID, "write") {
		return api.Err(http.StatusForbidden, "Forbidden")
	}

	if !superuser {
		if !h.iam.Enforce(process.Owner, process.Domain, "process:"+process.ID, "write") {
			return api.Err(http.StatusForbidden, "Forbidden")
		}
	}

	config, metadata := process.Marshal()

	if err := h.cluster.UpdateProcess("", pid, config); err != nil {
		if err == restream.ErrUnknownProcess {
			return api.Err(http.StatusNotFound, "Process not found", "%s", id)
		}

		return api.Err(http.StatusBadRequest, "Process can't be updated", "%s", err)
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
		return api.Err(http.StatusForbidden, "Forbidden")
	}

	var command api.Command

	if err := util.ShouldBindJSON(c, &command); err != nil {
		return api.Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
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
		return api.Err(http.StatusNotFound, "", "command failed: %s", err)
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
		return api.Err(http.StatusForbidden, "")
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
		return api.Err(http.StatusForbidden, "", "Not allowed to delete this process")
	}

	pid := app.ProcessID{
		ID:     id,
		Domain: domain,
	}

	if err := h.cluster.RemoveProcess("", pid); err != nil {
		return api.Err(http.StatusBadRequest, "", "%s", err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// Add adds a new identity to the cluster
// @Summary Add a new identiy
// @Description Add a new identity
// @Tags v16.?.?
// @ID cluster-3-add-identity
// @Accept json
// @Produce json
// @Param config body api.IAMUser true "Identity"
// @Success 200 {object} api.IAMUser
// @Failure 400 {object} api.Error
// @Failure 403 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/iam/user [post]
func (h *ClusterHandler) AddIdentity(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	superuser := util.DefaultContext(c, "superuser", false)
	domain := util.DefaultQuery(c, "domain", "$none")

	user := api.IAMUser{}

	if err := util.ShouldBindJSON(c, &user); err != nil {
		return api.Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
	}

	iamuser, iampolicies := user.Unmarshal()

	if !h.iam.Enforce(ctxuser, domain, "iam:"+iamuser.Name, "write") {
		return api.Err(http.StatusForbidden, "Forbidden", "Not allowed to create user '%s'", iamuser.Name)
	}

	for _, p := range iampolicies {
		if !h.iam.Enforce(ctxuser, p.Domain, "iam:"+iamuser.Name, "write") {
			return api.Err(http.StatusForbidden, "Forbidden", "Not allowed to write policy: %v", p)
		}
	}

	if !superuser && iamuser.Superuser {
		return api.Err(http.StatusForbidden, "Forbidden", "Only superusers can add superusers")
	}

	if err := h.cluster.AddIdentity("", iamuser); err != nil {
		return api.Err(http.StatusBadRequest, "Invalid identity", "%s", err.Error())
	}

	if err := h.cluster.SetPolicies("", iamuser.Name, iampolicies); err != nil {
		return api.Err(http.StatusBadRequest, "Invalid policies", "%s", err.Error())
	}

	return c.JSON(http.StatusOK, user)
}

// UpdateIdentity replaces an existing user
// @Summary Replace an existing user
// @Description Replace an existing user.
// @Tags v16.?.?
// @ID cluster-3-update-identity
// @Accept json
// @Produce json
// @Param name path string true "Username"
// @Param domain query string false "Domain of the acting user"
// @Param user body api.IAMUser true "User definition"
// @Success 200 {object} api.IAMUser
// @Failure 400 {object} api.Error
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Failure 500 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/iam/user/{name} [put]
func (h *ClusterHandler) UpdateIdentity(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	superuser := util.DefaultContext(c, "superuser", false)
	domain := util.DefaultQuery(c, "domain", "$none")
	name := util.PathParam(c, "name")

	if !h.iam.Enforce(ctxuser, domain, "iam:"+name, "write") {
		return api.Err(http.StatusForbidden, "Forbidden", "Not allowed to modify this user")
	}

	var iamuser identity.User
	var err error

	if name != "$anon" {
		iamuser, err = h.iam.GetIdentity(name)
		if err != nil {
			return api.Err(http.StatusNotFound, "Not found", "%s", err)
		}
	} else {
		iamuser = identity.User{
			Name: "$anon",
		}
	}

	iampolicies := h.iam.ListPolicies(name, "", "", nil)

	user := api.IAMUser{}
	user.Marshal(iamuser, iampolicies)

	if err := util.ShouldBindJSON(c, &user); err != nil {
		return api.Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
	}

	iamuser, iampolicies = user.Unmarshal()

	if !h.iam.Enforce(ctxuser, domain, "iam:"+iamuser.Name, "write") {
		return api.Err(http.StatusForbidden, "Forbidden", "Not allowed to create user '%s'", iamuser.Name)
	}

	for _, p := range iampolicies {
		if !h.iam.Enforce(ctxuser, p.Domain, "iam:"+iamuser.Name, "write") {
			return api.Err(http.StatusForbidden, "Forbidden", "Not allowed to write policy: %v", p)
		}
	}

	if !superuser && iamuser.Superuser {
		return api.Err(http.StatusForbidden, "Forbidden", "Only superusers can modify superusers")
	}

	if name != "$anon" {
		err = h.cluster.UpdateIdentity("", name, iamuser)
		if err != nil {
			return api.Err(http.StatusBadRequest, "Bad request", "%s", err)
		}
	}

	err = h.cluster.SetPolicies("", name, iampolicies)
	if err != nil {
		return api.Err(http.StatusInternalServerError, "", "set policies: %w", err)
	}

	return c.JSON(http.StatusOK, user)
}

// UpdateIdentityPolicies replaces existing user policies
// @Summary Replace policies of an user
// @Description Replace policies of an user
// @Tags v16.?.?
// @ID cluster-3-update-user-policies
// @Accept json
// @Produce json
// @Param name path string true "Username"
// @Param domain query string false "Domain of the acting user"
// @Param user body []api.IAMPolicy true "Policy definitions"
// @Success 200 {array} api.IAMPolicy
// @Failure 400 {object} api.Error
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Failure 500 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/iam/user/{name}/policy [put]
func (h *ClusterHandler) UpdateIdentityPolicies(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	superuser := util.DefaultContext(c, "superuser", false)
	domain := util.DefaultQuery(c, "domain", "$none")
	name := util.PathParam(c, "name")

	if !h.iam.Enforce(ctxuser, domain, "iam:"+name, "write") {
		return api.Err(http.StatusForbidden, "Forbidden", "Not allowed to modify this user")
	}

	var iamuser identity.User
	var err error

	if name != "$anon" {
		iamuser, err = h.iam.GetIdentity(name)
		if err != nil {
			return api.Err(http.StatusNotFound, "Not found", "%s", err)
		}
	} else {
		iamuser = identity.User{
			Name: "$anon",
		}
	}

	policies := []api.IAMPolicy{}

	if err := util.ShouldBindJSONValidation(c, &policies, false); err != nil {
		return api.Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
	}

	for _, p := range policies {
		err := c.Validate(p)
		if err != nil {
			return api.Err(http.StatusBadRequest, "Invalid JSON", "%s", err)
		}
	}

	accessPolicies := []access.Policy{}

	for _, p := range policies {
		if !h.iam.Enforce(ctxuser, p.Domain, "iam:"+iamuser.Name, "write") {
			return api.Err(http.StatusForbidden, "Forbidden", "Not allowed to write policy: %v", p)
		}

		accessPolicies = append(accessPolicies, access.Policy{
			Name:     name,
			Domain:   p.Domain,
			Resource: p.Resource,
			Actions:  p.Actions,
		})
	}

	if !superuser && iamuser.Superuser {
		return api.Err(http.StatusForbidden, "Forbidden", "Only superusers can modify superusers")
	}

	err = h.cluster.SetPolicies("", name, accessPolicies)
	if err != nil {
		return api.Err(http.StatusInternalServerError, "", "set policies: %s", err.Error())
	}

	return c.JSON(http.StatusOK, policies)
}

// ListStoreIdentities returns the list of identities stored in the DB of the cluster
// @Summary List of identities in the cluster
// @Description List of identities in the cluster
// @Tags v16.?.?
// @ID cluster-3-db-list-identities
// @Produce json
// @Success 200 {array} api.IAMUser
// @Security ApiKeyAuth
// @Router /api/v3/cluster/db/user [get]
func (h *ClusterHandler) ListStoreIdentities(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	domain := util.DefaultQuery(c, "domain", "$none")

	updatedAt, identities := h.cluster.ListIdentities()

	users := make([]api.IAMUser, len(identities))

	for i, iamuser := range identities {
		if !h.iam.Enforce(ctxuser, domain, "iam:"+iamuser.Name, "read") {
			continue
		}

		if !h.iam.Enforce(ctxuser, domain, "iam:"+iamuser.Name, "write") {
			iamuser = identity.User{
				Name: iamuser.Name,
			}
		}

		_, policies := h.cluster.ListUserPolicies(iamuser.Name)
		users[i].Marshal(iamuser, policies)
	}

	c.Response().Header().Set("Last-Modified", updatedAt.UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT"))

	return c.JSON(http.StatusOK, users)
}

// ListStoreIdentity returns the list of identities stored in the DB of the cluster
// @Summary List of identities in the cluster
// @Description List of identities in the cluster
// @Tags v16.?.?
// @ID cluster-3-db-list-identity
// @Produce json
// @Success 200 {object} api.IAMUser
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/db/user/{name} [get]
func (h *ClusterHandler) ListStoreIdentity(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	domain := util.DefaultQuery(c, "domain", "$none")
	name := util.PathParam(c, "name")

	if !h.iam.Enforce(ctxuser, domain, "iam:"+name, "read") {
		return api.Err(http.StatusForbidden, "Forbidden", "Not allowed to access this user")
	}

	var updatedAt time.Time
	var iamuser identity.User
	var err error

	if name != "$anon" {
		updatedAt, iamuser, err = h.cluster.ListIdentity(name)
		if err != nil {
			return api.Err(http.StatusNotFound, "", "%s", err)
		}

		if ctxuser != iamuser.Name {
			if !h.iam.Enforce(ctxuser, domain, "iam:"+name, "write") {
				iamuser = identity.User{
					Name: iamuser.Name,
				}
			}
		}
	} else {
		iamuser = identity.User{
			Name: "$anon",
		}
	}

	policiesUpdatedAt, policies := h.cluster.ListUserPolicies(name)
	if updatedAt.IsZero() {
		updatedAt = policiesUpdatedAt
	}

	user := api.IAMUser{}
	user.Marshal(iamuser, policies)

	c.Response().Header().Set("Last-Modified", updatedAt.UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT"))

	return c.JSON(http.StatusOK, user)
}

// ReloadIAM reloads the identities and policies from the cluster store to IAM
// @Summary Reload identities and policies
// @Description Reload identities and policies
// @Tags v16.?.?
// @ID cluster-3-iam-reload
// @Produce json
// @Success 200 {string} string
// @Success 500 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/iam/reload [get]
func (h *ClusterHandler) ReloadIAM(c echo.Context) error {
	err := h.iam.ReloadIndentities()
	if err != nil {
		return api.Err(http.StatusInternalServerError, "", "reload identities: %w", err)
	}

	err = h.iam.ReloadPolicies()
	if err != nil {
		return api.Err(http.StatusInternalServerError, "", "reload policies: %w", err)
	}

	return c.JSON(http.StatusOK, "OK")
}

// ListIdentities returns the list of identities stored in IAM
// @Summary List of identities in IAM
// @Description List of identities in IAM
// @Tags v16.?.?
// @ID cluster-3-iam-list-identities
// @Produce json
// @Success 200 {array} api.IAMUser
// @Security ApiKeyAuth
// @Router /api/v3/cluster/iam/user [get]
func (h *ClusterHandler) ListIdentities(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	domain := util.DefaultQuery(c, "domain", "$none")

	identities := h.iam.ListIdentities()

	users := make([]api.IAMUser, len(identities)+1)

	for i, iamuser := range identities {
		if !h.iam.Enforce(ctxuser, domain, "iam:"+iamuser.Name, "read") {
			continue
		}

		if !h.iam.Enforce(ctxuser, domain, "iam:"+iamuser.Name, "write") {
			iamuser = identity.User{
				Name: iamuser.Name,
			}
		}

		policies := h.iam.ListPolicies(iamuser.Name, "", "", nil)

		users[i].Marshal(iamuser, policies)
	}

	anon := identity.User{
		Name: "$anon",
	}

	policies := h.iam.ListPolicies("$anon", "", "", nil)

	users[len(users)-1].Marshal(anon, policies)

	return c.JSON(http.StatusOK, users)
}

// ListIdentity returns the identity stored in IAM
// @Summary Identity in IAM
// @Description Identity in IAM
// @Tags v16.?.?
// @ID cluster-3-iam-list-identity
// @Produce json
// @Success 200 {object} api.IAMUser
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/iam/user/{name} [get]
func (h *ClusterHandler) ListIdentity(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	domain := util.DefaultQuery(c, "domain", "$none")
	name := util.PathParam(c, "name")

	if !h.iam.Enforce(ctxuser, domain, "iam:"+name, "read") {
		return api.Err(http.StatusForbidden, "", "Not allowed to access this user")
	}

	var iamuser identity.User
	var err error

	if name != "$anon" {
		iamuser, err = h.iam.GetIdentity(name)
		if err != nil {
			return api.Err(http.StatusNotFound, "", "%s", err)
		}

		if ctxuser != iamuser.Name {
			if !h.iam.Enforce(ctxuser, domain, "iam:"+name, "write") {
				iamuser = identity.User{
					Name: iamuser.Name,
				}
			}
		}
	} else {
		iamuser = identity.User{
			Name: "$anon",
		}
	}

	iampolicies := h.iam.ListPolicies(name, "", "", nil)

	user := api.IAMUser{}
	user.Marshal(iamuser, iampolicies)

	return c.JSON(http.StatusOK, user)
}

// ListStorePolicies returns the list of policies stored in the DB of the cluster
// @Summary List of policies in the cluster
// @Description List of policies in the cluster
// @Tags v16.?.?
// @ID cluster-3-db-list-policies
// @Produce json
// @Success 200 {array} api.IAMPolicy
// @Security ApiKeyAuth
// @Router /api/v3/cluster/db/policies [get]
func (h *ClusterHandler) ListStorePolicies(c echo.Context) error {
	updatedAt, clusterpolicies := h.cluster.ListPolicies()

	policies := []api.IAMPolicy{}

	for _, pol := range clusterpolicies {
		policies = append(policies, api.IAMPolicy{
			Name:     pol.Name,
			Domain:   pol.Domain,
			Resource: pol.Resource,
			Actions:  pol.Actions,
		})
	}

	c.Response().Header().Set("Last-Modified", updatedAt.UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT"))

	return c.JSON(http.StatusOK, policies)
}

// ListPolicies returns the list of policies stored in IAM
// @Summary List of policies in IAM
// @Description List of policies IAM
// @Tags v16.?.?
// @ID cluster-3-iam-list-policies
// @Produce json
// @Success 200 {array} api.IAMPolicy
// @Security ApiKeyAuth
// @Router /api/v3/cluster/iam/policies [get]
func (h *ClusterHandler) ListPolicies(c echo.Context) error {
	iampolicies := h.iam.ListPolicies("", "", "", nil)

	policies := []api.IAMPolicy{}

	for _, pol := range iampolicies {
		policies = append(policies, api.IAMPolicy{
			Name:     pol.Name,
			Domain:   pol.Domain,
			Resource: pol.Resource,
			Actions:  pol.Actions,
		})
	}

	return c.JSON(http.StatusOK, policies)
}

// Delete deletes the identity with the given name from the cluster
// @Summary Delete an identity by its name
// @Description Delete an identity by its name
// @Tags v16.?.?
// @ID cluster-3-delete-identity
// @Produce json
// @Param name path string true "Identity name"
// @Success 200 {string} string
// @Failure 403 {object} api.Error
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/iam/user/{name} [delete]
func (h *ClusterHandler) RemoveIdentity(c echo.Context) error {
	ctxuser := util.DefaultContext(c, "user", "")
	superuser := util.DefaultContext(c, "superuser", false)
	domain := util.DefaultQuery(c, "domain", "$none")
	name := util.PathParam(c, "name")

	if !h.iam.Enforce(ctxuser, domain, "iam:"+name, "write") {
		return api.Err(http.StatusForbidden, "Forbidden", "Not allowed to delete this user")
	}

	iamuser, err := h.iam.GetIdentity(name)
	if err != nil {
		return api.Err(http.StatusNotFound, "Not found", "%s", err)
	}

	if !superuser && iamuser.Superuser {
		return api.Err(http.StatusForbidden, "Forbidden", "Only superusers can remove superusers")
	}

	if err := h.cluster.RemoveIdentity("", name); err != nil {
		return api.Err(http.StatusBadRequest, "Invalid identity", "%s", err.Error())
	}

	return c.JSON(http.StatusOK, "OK")
}

// ListStoreLocks returns the list of currently stored locks
// @Summary List locks in the cluster DB
// @Description List of locks in the cluster DB
// @Tags v16.?.?
// @ID cluster-3-db-list-locks
// @Produce json
// @Success 200 {array} api.ClusterLock
// @Security ApiKeyAuth
// @Router /api/v3/cluster/db/locks [get]
func (h *ClusterHandler) ListStoreLocks(c echo.Context) error {
	clusterlocks := h.cluster.ListLocks()

	locks := []api.ClusterLock{}

	for name, validUntil := range clusterlocks {
		locks = append(locks, api.ClusterLock{
			Name:       name,
			ValidUntil: validUntil,
		})
	}

	return c.JSON(http.StatusOK, locks)
}
