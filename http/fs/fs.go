package fs

import (
	"github.com/darkiris4/sfx-core/http/cache"
	"github.com/darkiris4/sfx-core/io/fs"
)

type FS struct {
	Name       string
	Mountpoint string

	AllowWrite bool

	EnableAuth bool
	Username   string
	Password   string

	DefaultFile        string
	DefaultContentType string
	Gzip               bool

	Filesystem fs.Filesystem

	Cache cache.Cacher
}
