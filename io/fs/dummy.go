package fs

import (
	"io"
	"time"
)

type dummyFileInfo struct{}

func (d *dummyFileInfo) Name() string           { return "" }
func (d *dummyFileInfo) Size() int64            { return 0 }
func (d *dummyFileInfo) ModTime() time.Time     { return time.Date(2000, 1, 1, 0, 0, 0, 0, nil) }
func (d *dummyFileInfo) IsLink() (string, bool) { return "", false }
func (d *dummyFileInfo) IsDir() bool            { return false }

type dummyFile struct{}

func (d *dummyFile) Read(p []byte) (int, error) { return 0, io.EOF }
func (d *dummyFile) Close() error               { return nil }
func (d *dummyFile) Name() string               { return "" }
func (d *dummyFile) Stat() (FileInfo, error)    { return &dummyFileInfo{}, nil }

type dummyFilesystem struct {
	name string
	typ  string
}

func (d *dummyFilesystem) Name() string                                 { return d.name }
func (d *dummyFilesystem) Base() string                                 { return "/" }
func (d *dummyFilesystem) Rebase(string) error                          { return nil }
func (d *dummyFilesystem) Type() string                                 { return d.typ }
func (d *dummyFilesystem) Size() (int64, int64)                         { return 0, -1 }
func (d *dummyFilesystem) Resize(int64)                                 {}
func (d *dummyFilesystem) Files() int64                                 { return 0 }
func (d *dummyFilesystem) Symlink(string, string) error                 { return nil }
func (d *dummyFilesystem) Open(string) File                             { return &dummyFile{} }
func (d *dummyFilesystem) Store(string, io.Reader) (int64, bool, error) { return 0, true, nil }
func (d *dummyFilesystem) Delete(string) int64                          { return 0 }
func (d *dummyFilesystem) DeleteAll() int64                             { return 0 }
func (d *dummyFilesystem) List(string) []FileInfo                       { return []FileInfo{} }

// NewDummyFilesystem return a dummy filesystem
func NewDummyFilesystem(name, typ string) Filesystem {
	return &dummyFilesystem{
		name: name,
		typ:  typ,
	}
}
