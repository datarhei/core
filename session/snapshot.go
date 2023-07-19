package session

import (
	"bytes"
	"errors"
	"io"
	"os"

	"github.com/datarhei/core/v16/io/fs"
)

type SnapshotSource interface {
	io.ReadCloser
}

type SnapshotSink interface {
	io.WriteCloser

	Cancel() error
}

type Snapshot interface {
	Persist(sink SnapshotSink) error
	Release()
}

type historySource struct {
	fs   fs.Filesystem
	path string
	data *bytes.Reader
}

// NewHistorySource returns a new SnapshotSource which reads the previously stored
// session history. If there's no data, a nil source with a nil error will be returned.
// If there's data, a non-nil source with a nil error will be returned. Otherwise
// the source will be nil and the error non-nil.
func NewHistorySource(fs fs.Filesystem, path string) (SnapshotSource, error) {
	s := &historySource{
		fs:   fs,
		path: path,
	}

	if _, err := s.fs.Stat(s.path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}

		return nil, err
	}

	data, err := s.fs.ReadFile(s.path)
	if err != nil {
		return nil, err
	}

	s.data = bytes.NewReader(data)

	return s, nil
}

func (s *historySource) Read(p []byte) (int, error) {
	return s.data.Read(p)
}

func (s *historySource) Close() error {
	s.data = nil
	return nil
}

type historySink struct {
	fs   fs.Filesystem
	path string
	data *bytes.Buffer
}

func NewHistorySink(fs fs.Filesystem, path string) (SnapshotSink, error) {
	s := &historySink{
		fs:   fs,
		path: path,
		data: &bytes.Buffer{},
	}

	return s, nil
}

func (s *historySink) Write(p []byte) (int, error) {
	return s.data.Write(p)
}

func (s *historySink) Close() error {
	_, _, err := s.fs.WriteFileSafe(s.path, s.data.Bytes())
	s.data = nil
	return err
}

func (s *historySink) Cancel() error {
	s.data = nil
	return nil
}
