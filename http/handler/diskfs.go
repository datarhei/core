package handler

import (
	"net/http"
	"path/filepath"

	"github.com/datarhei/core/http/api"
	"github.com/datarhei/core/http/cache"
	"github.com/datarhei/core/http/handler/util"
	"github.com/datarhei/core/io/fs"

	"github.com/labstack/echo/v4"
)

// The DiskFSHandler type provides handlers for manipulating a filesystem
type DiskFSHandler struct {
	cache      cache.Cacher
	filesystem fs.Filesystem
}

// NewDiskFS return a new DiskFS type. You have to provide a filesystem to act on and optionally
// a Cacher where files will be purged from if the Cacher is related to the filesystem.
func NewDiskFS(fs fs.Filesystem, cache cache.Cacher) *DiskFSHandler {
	return &DiskFSHandler{
		cache:      cache,
		filesystem: fs,
	}
}

// GetFile returns the file at the given path
// @Summary Fetch a file from the filesystem
// @Description Fetch a file from the filesystem. If the file is a directory, a index.html is returned, if it exists.
// @ID diskfs-get-file
// @Produce application/data
// @Produce json
// @Param path path string true "Path to file"
// @Success 200 {file} byte
// @Success 301 {string} string
// @Failure 404 {object} api.Error
// @Router /{path} [get]
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
		path = filepath.Join(path, "index.html")

		file.Close()

		file = h.filesystem.Open(path)
		if file == nil {
			return api.Err(http.StatusNotFound, "File not found", path)
		}

		stat, _ = file.Stat()
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
