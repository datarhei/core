package api

import (
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/datarhei/core/v16/cluster"
	"github.com/datarhei/core/v16/cluster/node"
	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/handler/util"
	"github.com/labstack/echo/v4"
)

// NodeList returns the list of proxy nodes in the cluster
// @Summary List of proxy nodes in the cluster
// @Description List of proxy nodes in the cluster
// @Tags v16.?.?
// @ID cluster-3-get-nodes
// @Produce json
// @Success 200 {array} api.ClusterNode
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/node [get]
func (h *ClusterHandler) NodeList(c echo.Context) error {
	about, _ := h.cluster.About()

	nodes := h.cluster.Store().NodeList()

	list := []api.ClusterNode{}

	for _, node := range about.Nodes {
		if dbnode, hasNode := nodes[node.ID]; hasNode {
			if dbnode.State == "maintenance" {
				node.State = dbnode.State
			}
		}

		list = append(list, h.marshalClusterNode(node))
	}

	return c.JSON(http.StatusOK, list)
}

// NodeGet returns the proxy node with the given ID
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
func (h *ClusterHandler) NodeGet(c echo.Context) error {
	id := util.PathParam(c, "id")

	about, _ := h.cluster.About()

	nodes := h.cluster.Store().NodeList()

	for _, node := range about.Nodes {
		if node.ID != id {
			continue
		}

		if dbnode, hasNode := nodes[node.ID]; hasNode {
			if dbnode.State == "maintenance" {
				node.State = dbnode.State
			}
		}

		return c.JSON(http.StatusOK, h.marshalClusterNode(node))
	}

	return api.Err(http.StatusNotFound, "", "node not found")
}

// NodeGetVersion returns the proxy node version with the given ID
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
func (h *ClusterHandler) NodeGetVersion(c echo.Context) error {
	id := util.PathParam(c, "id")

	peer, err := h.proxy.NodeGet(id)
	if err != nil {
		return api.Err(http.StatusNotFound, "", "node not found: %s", err.Error())
	}

	v := peer.CoreAbout()

	version := api.AboutVersion{
		Number:   v.Version.Number,
		Commit:   v.Version.Commit,
		Branch:   v.Version.Branch,
		Build:    v.Version.Build.Format(time.RFC3339),
		Arch:     v.Version.Arch,
		Compiler: v.Version.Compiler,
	}

	return c.JSON(http.StatusOK, version)
}

// NodeGetMedia returns the resources from the proxy node with the given ID
// @Summary List the resources of a proxy node by its ID
// @Description List the resources of a proxy node by its ID
// @Tags v16.?.?
// @ID cluster-3-get-node-files
// @Produce json
// @Param id path string true "Node ID"
// @Success 200 {object} api.ClusterNodeFiles
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/node/{id}/files [get]
func (h *ClusterHandler) NodeGetMedia(c echo.Context) error {
	id := util.PathParam(c, "id")

	peer, err := h.proxy.NodeGet(id)
	if err != nil {
		return api.Err(http.StatusNotFound, "", "node not found: %s", err.Error())
	}

	files := api.ClusterNodeFiles{
		Files: make(map[string][]string),
	}

	peerFiles := peer.Core().MediaList()

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

// NodeFSListFiles lists all files on a filesystem on a node
// @Summary List all files on a filesystem on a node
// @Description List all files on a filesystem on a node. The listing can be ordered by name, size, or date of last modification in ascending or descending order.
// @Tags v16.?.?
// @ID cluster-3-node-fs-list-files
// @Produce json
// @Param id path string true "Node ID"
// @Param storage path string true "Name of the filesystem"
// @Param glob query string false "glob pattern for file names"
// @Param sort query string false "none, name, size, lastmod"
// @Param order query string false "asc, desc"
// @Success 200 {array} api.FileInfo
// @Success 500 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/node/{id}/fs/{storage} [get]
func (h *ClusterHandler) NodeFSListFiles(c echo.Context) error {
	id := util.PathParam(c, "id")
	name := util.PathParam(c, "storage")
	pattern := util.DefaultQuery(c, "glob", "")
	sortby := util.DefaultQuery(c, "sort", "none")
	order := util.DefaultQuery(c, "order", "asc")

	peer, err := h.proxy.NodeGet(id)
	if err != nil {
		return api.Err(http.StatusNotFound, "", "node not found: %s", err.Error())
	}

	files, err := peer.Core().FilesystemList(name, pattern)
	if err != nil {
		return api.Err(http.StatusInternalServerError, "", "retrieving file list: %s", err.Error())
	}

	var sortFunc func(i, j int) bool

	switch sortby {
	case "name":
		if order == "desc" {
			sortFunc = func(i, j int) bool { return files[i].Name > files[j].Name }
		} else {
			sortFunc = func(i, j int) bool { return files[i].Name < files[j].Name }
		}
	case "size":
		if order == "desc" {
			sortFunc = func(i, j int) bool { return files[i].Size > files[j].Size }
		} else {
			sortFunc = func(i, j int) bool { return files[i].Size < files[j].Size }
		}
	default:
		if order == "asc" {
			sortFunc = func(i, j int) bool { return files[i].LastMod < files[j].LastMod }
		} else {
			sortFunc = func(i, j int) bool { return files[i].LastMod > files[j].LastMod }
		}
	}

	sort.Slice(files, sortFunc)

	return c.JSON(http.StatusOK, files)
}

// NodeFSGetFile returns the file at the given path on a node
// @Summary Fetch a file from a filesystem on a node
// @Description Fetch a file from a filesystem on a node
// @Tags v16.?.?
// @ID cluster-3-node-fs-get-file
// @Produce application/data
// @Produce json
// @Param id path string true "Node ID"
// @Param storage path string true "Name of the filesystem"
// @Param filepath path string true "Path to file"
// @Success 200 {file} byte
// @Success 301 {string} string
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/node/{id}/fs/{storage}/{filepath} [get]
func (h *ClusterHandler) NodeFSGetFile(c echo.Context) error {
	id := util.PathParam(c, "id")
	storage := util.PathParam(c, "storage")
	path := util.PathWildcardParam(c)

	peer, err := h.proxy.NodeGet(id)
	if err != nil {
		return api.Err(http.StatusNotFound, "", "node not found: %s", err.Error())
	}

	file, err := peer.Core().FilesystemGetFile(storage, path, 0)
	if err != nil {
		return api.Err(http.StatusNotFound, "", "%s", err.Error())
	}

	defer file.Close()

	return c.Stream(http.StatusOK, "application/data", file)
}

// NodeFSPutFile adds or overwrites a file at the given path on a node
// @Summary Add a file to a filesystem on a node
// @Description Writes or overwrites a file on a filesystem on a node
// @Tags v16.?.?
// @ID cluster-3-node-fs-put-file
// @Accept application/data
// @Produce text/plain
// @Produce json
// @Param id path string true "Node ID"
// @Param storage path string true "Name of the filesystem"
// @Param filepath path string true "Path to file"
// @Param data body []byte true "File data"
// @Success 201 {string} string
// @Failure 400 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/node/{id}/fs/{storage}/{filepath} [put]
func (h *ClusterHandler) NodeFSPutFile(c echo.Context) error {
	id := util.PathParam(c, "id")
	storage := util.PathParam(c, "storage")
	path := util.PathWildcardParam(c)

	peer, err := h.proxy.NodeGet(id)
	if err != nil {
		return api.Err(http.StatusNotFound, "", "node not found: %s", err.Error())
	}

	req := c.Request()

	err = peer.Core().FilesystemPutFile(storage, path, req.Body)
	if err != nil {
		return api.Err(http.StatusBadRequest, "", "%s", err.Error())
	}

	return c.JSON(http.StatusCreated, nil)
}

// NodeFSDeleteFile removes a file from a filesystem on a node
// @Summary Remove a file from a filesystem on a node
// @Description Remove a file from a filesystem on a node
// @Tags v16.?.?
// @ID cluster-3-node-fs-delete-file
// @Produce text/plain
// @Param id path string true "Node ID"
// @Param storage path string true "Name of the filesystem"
// @Param filepath path string true "Path to file"
// @Success 200 {string} string
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/node/{id}/fs/{storage}/{filepath} [delete]
func (h *ClusterHandler) NodeFSDeleteFile(c echo.Context) error {
	id := util.PathParam(c, "id")
	storage := util.PathParam(c, "storage")
	path := util.PathWildcardParam(c)

	peer, err := h.proxy.NodeGet(id)
	if err != nil {
		return api.Err(http.StatusNotFound, "", "node not found: %s", err.Error())
	}

	err = peer.Core().FilesystemDeleteFile(storage, path)
	if err != nil {
		return api.Err(http.StatusNotFound, "", "%s", err.Error())
	}

	return c.JSON(http.StatusOK, nil)
}

// NodeListProcesses returns the list of processes running on a node of the cluster
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
func (h *ClusterHandler) NodeListProcesses(c echo.Context) error {
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

	peer, err := h.proxy.NodeGet(id)
	if err != nil {
		return api.Err(http.StatusNotFound, "", "node not found: %s", err.Error())
	}

	procs, err := peer.Core().ProcessList(node.ProcessListOptions{
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
		return api.Err(http.StatusInternalServerError, "", "node not available: %s", err.Error())
	}

	processes := []api.Process{}

	for _, p := range procs {
		if !h.iam.Enforce(ctxuser, domain, "process", p.Config.ID, "read") {
			continue
		}

		processes = append(processes, p)
	}

	return c.JSON(http.StatusOK, processes)
}

// NodeGetState returns the state of a node with the given ID
// @Summary Get the state of a node with the given ID
// @Description Get the state of a node with the given ID
// @Tags v16.?.?
// @ID cluster-3-get-node-state
// @Produce json
// @Param id path string true "Node ID"
// @Success 200 {object} api.ClusterNodeState
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/node/{id}/state [get]
func (h *ClusterHandler) NodeGetState(c echo.Context) error {
	id := util.PathParam(c, "id")

	about, _ := h.cluster.About()

	state := ""
	for _, node := range about.Nodes {
		if node.ID != id {
			continue
		}

		state = node.State
		break
	}

	if len(state) == 0 {
		return api.Err(http.StatusNotFound, "", "node not found")
	}

	nodes := h.cluster.Store().NodeList()
	if node, hasNode := nodes[id]; hasNode {
		if node.State == "maintenance" {
			state = node.State
		}
	}

	return c.JSON(http.StatusOK, api.ClusterNodeState{
		State: state,
	})
}

// NodeSetState sets the state of a node with the given ID
// @Summary Set the state of a node with the given ID
// @Description Set the state of a node with the given ID
// @Tags v16.?.?
// @ID cluster-3-set-node-state
// @Produce json
// @Param id path string true "Node ID"
// @Param config body api.ClusterNodeState true "State"
// @Success 200 {string} string
// @Failure 400 {object} api.Error
// @Failure 404 {object} api.Error
// @Failure 500 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/cluster/node/{id}/state [put]
func (h *ClusterHandler) NodeSetState(c echo.Context) error {
	id := util.PathParam(c, "id")

	about, _ := h.cluster.About()

	found := false
	for _, node := range about.Nodes {
		if node.ID != id {
			continue
		}

		found = true
		break
	}

	if !found {
		return api.Err(http.StatusNotFound, "", "node not found")
	}

	state := api.ClusterNodeState{}

	if err := util.ShouldBindJSON(c, &state); err != nil {
		return api.Err(http.StatusBadRequest, "", "invalid JSON: %s", err.Error())
	}

	if state.State == "leave" {
		err := h.cluster.Leave("", id)
		if err != nil {
			if errors.Is(err, cluster.ErrUnknownNode) {
				return api.Err(http.StatusNotFound, "", "node not found")
			}

			return api.Err(http.StatusInternalServerError, "", "%s", err.Error())
		}

		return c.JSON(http.StatusOK, "OK")
	}

	err := h.cluster.NodeSetState("", id, state.State)
	if err != nil {
		if errors.Is(err, cluster.ErrUnsupportedNodeState) {
			return api.Err(http.StatusBadRequest, "", "%s", err.Error())
		}
		return api.Err(http.StatusInternalServerError, "", "%s", err.Error())
	}

	return c.JSON(http.StatusOK, "OK")
}
