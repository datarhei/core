package handler

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/textproto"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/fs"
	"github.com/datarhei/core/v16/http/handler/util"

	"github.com/labstack/echo/v4"
)

// The FSHandler type provides handlers for manipulating a filesystem
type FSHandler struct {
	fs fs.FS
}

// NewFS return a new FSHandler type. You have to provide a filesystem to act on.
func NewFS(fs fs.FS) *FSHandler {
	return &FSHandler{
		fs: fs,
	}
}

func (h *FSHandler) GetFile(c echo.Context) error {
	path := util.PathWildcardParam(c)

	mimeType := c.Response().Header().Get(echo.HeaderContentType)
	c.Response().Header().Del(echo.HeaderContentType)

	file := h.fs.Filesystem.Open(path)
	if file == nil {
		return api.Err(http.StatusNotFound, "File not found", path)
	}

	stat, _ := file.Stat()

	if len(h.fs.DefaultFile) != 0 {
		if stat.IsDir() {
			path = filepath.Join(path, h.fs.DefaultFile)

			file.Close()

			file = h.fs.Filesystem.Open(path)
			if file == nil {
				return api.Err(http.StatusNotFound, "File not found", path)
			}

			stat, _ = file.Stat()
		}
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
	c.Response().Header().Set("Accept-Ranges", "bytes")

	if c.Request().Method == "HEAD" {
		c.Response().Header().Set(echo.HeaderContentLength, strconv.FormatInt(stat.Size(), 10))
		return c.Blob(http.StatusOK, "application/data", nil)
	}

	var streamFile io.Reader = file
	status := http.StatusOK

	ifRange := c.Request().Header.Get("If-Range")
	if len(ifRange) != 0 {
		ifTime, err := time.Parse("Mon, 02 Jan 2006 15:04:05 MST", ifRange)
		if err != nil {
			return api.Err(http.StatusBadRequest, "", "%s", err)
		}

		if ifTime.Unix() != stat.ModTime().Unix() {
			c.Request().Header.Del("Range")
		}
	}

	byteRange := c.Request().Header.Get("Range")
	if len(byteRange) != 0 {
		ranges, err := parseRange(byteRange, stat.Size())
		if err != nil {
			return api.Err(http.StatusRequestedRangeNotSatisfiable, "", "%s", err.Error())
		}

		if len(ranges) > 1 {
			return api.Err(http.StatusNotImplemented, "", "multipart range requests are not supported")
		}

		if len(ranges) == 1 {
			_, err := file.Seek(ranges[0].start, io.SeekStart)
			if err != nil {
				return api.Err(http.StatusRequestedRangeNotSatisfiable, "", "%s", err.Error())
			}

			c.Response().Header().Set("Content-Range", ranges[0].contentRange(stat.Size()))
			streamFile = &io.LimitedReader{
				R: streamFile,
				N: ranges[0].length,
			}

			status = http.StatusPartialContent
		}
	}

	return c.Stream(status, "application/data", streamFile)
}

func (h *FSHandler) PutFile(c echo.Context) error {
	path := util.PathWildcardParam(c)

	c.Response().Header().Del(echo.HeaderContentType)

	req := c.Request()

	_, created, err := h.fs.Filesystem.WriteFileReader(path, req.Body)
	if err != nil {
		return api.Err(http.StatusBadRequest, "Bad request", "%s", err)
	}

	if h.fs.Cache != nil {
		h.fs.Cache.Delete(path)

		if len(h.fs.DefaultFile) != 0 {
			if strings.HasSuffix(path, "/"+h.fs.DefaultFile) {
				path := strings.TrimSuffix(path, h.fs.DefaultFile)
				h.fs.Cache.Delete(path)
			}
		}
	}

	c.Response().Header().Set("Content-Location", req.URL.RequestURI())

	if created {
		return c.String(http.StatusCreated, "")
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *FSHandler) DeleteFile(c echo.Context) error {
	path := util.PathWildcardParam(c)

	c.Response().Header().Del(echo.HeaderContentType)

	size := h.fs.Filesystem.Remove(path)

	if h.fs.Cache != nil {
		h.fs.Cache.Delete(path)

		if len(h.fs.DefaultFile) != 0 {
			if strings.HasSuffix(path, "/"+h.fs.DefaultFile) {
				path := strings.TrimSuffix(path, h.fs.DefaultFile)
				h.fs.Cache.Delete(path)
			}
		}
	}

	if size < 0 {
		return api.Err(http.StatusNotFound, "File not found", path)
	}

	return c.String(http.StatusOK, "Deleted: "+path)
}

func (h *FSHandler) ListFiles(c echo.Context) error {
	pattern := util.DefaultQuery(c, "glob", "")
	sortby := util.DefaultQuery(c, "sort", "none")
	order := util.DefaultQuery(c, "order", "asc")

	files := h.fs.Filesystem.List("/", pattern)

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

// From: github.com/golang/go/net/http/fs.go@7dc9fcb

// errNoOverlap is returned by serveContent's parseRange if first-byte-pos of
// all of the byte-range-spec values is greater than the content size.
var errNoOverlap = errors.New("invalid range: failed to overlap")

// httpRange specifies the byte range to be sent to the client.
type httpRange struct {
	start, length int64
}

func (r httpRange) contentRange(size int64) string {
	return fmt.Sprintf("bytes %d-%d/%d", r.start, r.start+r.length-1, size)
}

/*
func (r httpRange) mimeHeader(contentType string, size int64) textproto.MIMEHeader {
	return textproto.MIMEHeader{
		"Content-Range": {r.contentRange(size)},
		"Content-Type":  {contentType},
	}
}
*/

// parseRange parses a Range header string as per RFC 7233.
// errNoOverlap is returned if none of the ranges overlap.
func parseRange(s string, size int64) ([]httpRange, error) {
	if s == "" {
		return nil, nil // header not present
	}
	const b = "bytes="
	if !strings.HasPrefix(s, b) {
		return nil, errors.New("invalid range")
	}
	var ranges []httpRange
	noOverlap := false
	for _, ra := range strings.Split(s[len(b):], ",") {
		ra = textproto.TrimString(ra)
		if ra == "" {
			continue
		}
		start, end, ok := strings.Cut(ra, "-")
		if !ok {
			return nil, errors.New("invalid range")
		}
		start, end = textproto.TrimString(start), textproto.TrimString(end)
		var r httpRange
		if start == "" {
			// If no start is specified, end specifies the
			// range start relative to the end of the file,
			// and we are dealing with <suffix-length>
			// which has to be a non-negative integer as per
			// RFC 7233 Section 2.1 "Byte-Ranges".
			if end == "" || end[0] == '-' {
				return nil, errors.New("invalid range")
			}
			i, err := strconv.ParseInt(end, 10, 64)
			if i < 0 || err != nil {
				return nil, errors.New("invalid range")
			}
			if i > size {
				i = size
			}
			r.start = size - i
			r.length = size - r.start
		} else {
			i, err := strconv.ParseInt(start, 10, 64)
			if err != nil || i < 0 {
				return nil, errors.New("invalid range")
			}
			if i >= size {
				// If the range begins after the size of the content,
				// then it does not overlap.
				noOverlap = true
				continue
			}
			r.start = i
			if end == "" {
				// If no end is specified, range extends to end of the file.
				r.length = size - r.start
			} else {
				i, err := strconv.ParseInt(end, 10, 64)
				if err != nil || r.start > i {
					return nil, errors.New("invalid range")
				}
				if i >= size {
					i = size - 1
				}
				r.length = i - r.start + 1
			}
		}
		ranges = append(ranges, r)
	}
	if noOverlap && len(ranges) == 0 {
		// The specified ranges did not overlap with the content.
		return nil, errNoOverlap
	}
	return ranges, nil
}
