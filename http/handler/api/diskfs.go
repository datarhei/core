package api

import (
	"net/http"
	"path/filepath"
	"sort"

	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/cache"
	"github.com/datarhei/core/v16/http/handler"
	"github.com/datarhei/core/v16/http/handler/util"
	"github.com/datarhei/core/v16/io/fs"

	"github.com/labstack/echo/v4"
)

// The DiskFSHandler type provides handlers for manipulating a filesystem
type DiskFSHandler struct {
	cache      cache.Cacher
	filesystem fs.Filesystem
	handler    *handler.DiskFSHandler
}

// NewDiskFS return a new DiskFS type. You have to provide a filesystem to act on and optionally
// a Cacher where files will be purged from if the Cacher is related to the filesystem.
func NewDiskFS(fs fs.Filesystem, cache cache.Cacher) *DiskFSHandler {
	return &DiskFSHandler{
		cache:      cache,
		filesystem: fs,
		handler:    handler.NewDiskFS(fs, cache),
	}
}

// GetFile returns the file at the given path
// @Summary Fetch a file from the filesystem
// @Description Fetch a file from the filesystem. The contents of that file are returned.
// @Tags v16.7.2
// @ID diskfs-3-get-file
// @Produce application/data
// @Produce json
// @Param path path string true "Path to file"
// @Success 200 {file} byte
// @Success 301 {string} string
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/fs/disk/{path} [get]
func (h *DiskFSHandler) GetFile(c echo.Context) error {
	path := util.PathWildcardParam(c)

	mimeType := c.Response().Header().Get(echo.HeaderContentType)
	c.Response().Header().Del(echo.HeaderContentType)

	file := h.filesystem.Open(path)
	if file == nil {
		return api.Err(http.StatusNotFound, "File not found", path)
	}

	stat, _ := file.Stat()

	if stat.IsDir() {
		return api.Err(http.StatusNotFound, "File not found", path)
	}

	defer file.Close()

	c.Response().Header().Set("Last-Modified", stat.ModTime().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT"))

	if path, ok := stat.IsLink(); ok {
		path = filepath.Clean("/" + path)

		if path[0] == '/' {
			path = path[1:]
		}

		return c.Redirect(http.StatusMovedPermanently, path)
	}

	c.Response().Header().Set(echo.HeaderContentType, mimeType)

	if c.Request().Method == "HEAD" {
		return c.Blob(http.StatusOK, "application/data", nil)
	}

	return c.Stream(http.StatusOK, "application/data", file)
}

// PutFile adds or overwrites a file at the given path
// @Summary Add a file to the filesystem
// @Description Writes or overwrites a file on the filesystem
// @Tags v16.7.2
// @ID diskfs-3-put-file
// @Accept application/data
// @Produce text/plain
// @Produce json
// @Param path path string true "Path to file"
// @Param data body []byte true "File data"
// @Success 201 {string} string
// @Success 204 {string} string
// @Failure 507 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/fs/disk/{path} [put]
func (h *DiskFSHandler) PutFile(c echo.Context) error {
	path := util.PathWildcardParam(c)

	c.Response().Header().Del(echo.HeaderContentType)

	req := c.Request()

	_, created, err := h.filesystem.Store(path, req.Body)
	if err != nil {
		return api.Err(http.StatusBadRequest, "%s", err)
	}

	if h.cache != nil {
		h.cache.Delete(path)
	}

	c.Response().Header().Set("Content-Location", req.URL.RequestURI())

	if created {
		return c.String(http.StatusCreated, path)
	}

	return c.NoContent(http.StatusNoContent)
}

// DeleteFile removes a file from the filesystem
// @Summary Remove a file from the filesystem
// @Description Remove a file from the filesystem
// @Tags v16.7.2
// @ID diskfs-3-delete-file
// @Produce text/plain
// @Param path path string true "Path to file"
// @Success 200 {string} string
// @Failure 404 {object} api.Error
// @Security ApiKeyAuth
// @Router /api/v3/fs/disk/{path} [delete]
func (h *DiskFSHandler) DeleteFile(c echo.Context) error {
	path := util.PathWildcardParam(c)

	c.Response().Header().Del(echo.HeaderContentType)

	size := h.filesystem.Delete(path)

	if size < 0 {
		return api.Err(http.StatusNotFound, "File not found", path)
	}

	if h.cache != nil {
		h.cache.Delete(path)
	}

	return c.String(http.StatusOK, "OK")
}

// ListFiles lists all files on the filesystem
// @Summary List all files on the filesystem
// @Description List all files on the filesystem. The listing can be ordered by name, size, or date of last modification in ascending or descending order.
// @Tags v16.7.2
// @ID diskfs-3-list-files
// @Produce json
// @Param glob query string false "glob pattern for file names"
// @Param sort query string false "none, name, size, lastmod"
// @Param order query string false "asc, desc"
// @Success 200 {array} api.FileInfo
// @Security ApiKeyAuth
// @Router /api/v3/fs/disk [get]
func (h *DiskFSHandler) ListFiles(c echo.Context) error {
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

	fileinfos := []api.FileInfo{}

	for _, f := range files {
		if f.IsDir() {
			continue
		}

		fileinfos = append(fileinfos, api.FileInfo{
			Name:    f.Name(),
			Size:    f.Size(),
			LastMod: f.ModTime().Unix(),
		})
	}

	return c.JSON(http.StatusOK, fileinfos)
}
