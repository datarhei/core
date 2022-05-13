package api

import (
	"io/ioutil"
	"net/http"
	"net/url"
	"sort"

	"github.com/datarhei/core/http/api"
	"github.com/datarhei/core/http/handler"
	"github.com/datarhei/core/http/handler/util"
	"github.com/datarhei/core/io/fs"

	"github.com/labstack/echo/v4"
)

// The MemFSHandler type provides handlers for manipulating a filesystem
type MemFSHandler struct {
	filesystem fs.Filesystem
	handler    *handler.MemFSHandler
}

// NewMemFS return a new MemFS type. You have to provide a filesystem to act on.
func NewMemFS(fs fs.Filesystem) *MemFSHandler {
	return &MemFSHandler{
		filesystem: fs,
		handler:    handler.NewMemFS(fs),
	}
}

// GetFileAPI returns the file at the given path
// @Summary Fetch a file from the memory filesystem
// @Description Fetch a file from the memory filesystem
// @ID memfs-3-get-file-api
// @Produce application/data
// @Produce json
// @Param path path string true "Path to file"
// @Success 200 {file} byte
// @Success 301 {string} string
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/fs/mem/{path} [get]
func (h *MemFSHandler) GetFile(c echo.Context) error {
	return h.handler.GetFile(c)
}

// PutFileAPI adds or overwrites a file at the given path
// @Summary Add a file to the memory filesystem
// @Description Writes or overwrites a file on the memory filesystem
// @ID memfs-3-put-file-api
// @Accept application/data
// @Produce text/plain
// @Produce json
// @Param path path string true "Path to file"
// @Param data body []byte true "File data"
// @Success 201 {string} string
// @Success 204 {string} string
// @Failure 507 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/fs/mem/{path} [put]
func (h *MemFSHandler) PutFile(c echo.Context) error {
	return h.handler.PutFile(c)
}

// DeleteFileAPI removes a file from the filesystem
// @Summary Remove a file from the memory filesystem
// @Description Remove a file from the memory filesystem
// @ID memfs-delete-file-api
// @Produce text/plain
// @Param path path string true "Path to file"
// @Success 200 {string} string
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/fs/mem/{path} [delete]
func (h *MemFSHandler) DeleteFile(c echo.Context) error {
	return h.handler.DeleteFile(c)
}

// PatchFile creates a symbolic link to a file in the filesystem
// @Summary Create a link to a file in the memory filesystem
// @Description Create a link to a file in the memory filesystem. The file linked to has to exist.
// @ID memfs-3-patch
// @Accept application/data
// @Produce text/plain
// @Produce json
// @Param path path string true "Path to file"
// @Param url body string true "Path to the file to link to"
// @Success 201 {string} string
// @Failure 400 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/fs/mem/{path} [patch]
func (h *MemFSHandler) PatchFile(c echo.Context) error {
	path := util.PathWildcardParam(c)

	c.Response().Header().Del(echo.HeaderContentType)

	req := c.Request()

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return api.Err(http.StatusBadRequest, "Failed reading request body", "%s", err)
	}

	u, err := url.Parse(string(body))
	if err != nil {
		return api.Err(http.StatusBadRequest, "Body doesn't contain a valid path", "%s", err)
	}

	if err := h.filesystem.Symlink(u.Path, path); err != nil {
		return api.Err(http.StatusBadRequest, "Failed to create symlink", "%s", err)
	}

	c.Response().Header().Set("Content-Location", req.URL.RequestURI())

	return c.String(http.StatusCreated, "")
}

// ListFiles lists all files on the filesystem
// @Summary List all files on the memory filesystem
// @Description List all files on the memory filesystem. The listing can be ordered by name, size, or date of last modification in ascending or descending order.
// @ID memfs-3-list-files
// @Produce json
// @Param glob query string false "glob pattern for file names"
// @Param sort query string false "none, name, size, lastmod"
// @Param order query string false "asc, desc"
// @Success 200 {array} api.FileInfo
// @Security ApiKeyAuth
// @Router /api/v3/fs/mem/ [get]
func (h *MemFSHandler) ListFiles(c echo.Context) error {
	pattern := util.DefaultQuery(c, "glob", "")
	sortby := util.DefaultQuery(c, "sort", "none")
	order := util.DefaultQuery(c, "order", "asc")

	files := h.filesystem.List(pattern)

	var sortFunc func(i, j int) bool

	switch sortby {
	case "name":
		if order == "desc" {
			sortFunc = func(i, j int) bool { return files[i].Name() > files[j].Name() }
		} else {
			sortFunc = func(i, j int) bool { return files[i].Name() < files[j].Name() }
		}
	case "size":
		if order == "desc" {
			sortFunc = func(i, j int) bool { return files[i].Size() > files[j].Size() }
		} else {
			sortFunc = func(i, j int) bool { return files[i].Size() < files[j].Size() }
		}
	default:
		if order == "asc" {
			sortFunc = func(i, j int) bool { return files[i].ModTime().Before(files[j].ModTime()) }
		} else {
			sortFunc = func(i, j int) bool { return files[i].ModTime().After(files[j].ModTime()) }
		}
	}

	sort.Slice(files, sortFunc)

	var fileinfos []api.FileInfo = make([]api.FileInfo, len(files))

	for i, f := range files {
		fileinfos[i] = api.FileInfo{
			Name:    f.Name(),
			Size:    f.Size(),
			LastMod: f.ModTime().Unix(),
		}
	}

	return c.JSON(http.StatusOK, fileinfos)
}
