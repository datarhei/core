package fs

import "io"

// Sizer interface can decorate a Reader
type Sizer interface {
	// Size returns the size of the object
	Size() int64
}

type ReadSizer interface {
	io.Reader
	Sizer
}

type readSizer struct {
	r    io.Reader
	size int64
}

func NewReadSizer(r io.Reader, size int64) ReadSizer {
	rs := &readSizer{
		r:    r,
		size: size,
	}

	return rs
}

func (rs *readSizer) Read(p []byte) (int, error) {
	return rs.r.Read(p)
}

func (rs *readSizer) Size() int64 {
	return rs.size
}
