package gpu

import "errors"

type Process struct {
	PID    int32
	Memory uint64
}

type Stats struct {
	Name         string
	Architecture string

	MemoryTotal uint64
	MemoryUsed  uint64

	Usage        float64
	MemoryUsage  float64
	EncoderUsage float64
	DecoderUsage float64

	Process []Process

	Extension interface{}
}

type GPU interface {
	Count() (int, error)
	Stats() ([]Stats, error)
	Process(pid int32) (Process, error)
}

var ErrProcessNotFound = errors.New("process not found")
