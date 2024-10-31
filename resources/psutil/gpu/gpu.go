package gpu

import "errors"

type Process struct {
	PID     int32
	Index   int
	Memory  uint64  // bytes
	Usage   float64 // percent 0-100
	Encoder float64 // percent 0-100
	Decoder float64 // percent 0-100
}

type Stats struct {
	Index        int
	ID           string
	Name         string
	Architecture string

	MemoryTotal uint64 // bytes
	MemoryUsed  uint64 // bytes

	Usage   float64 // percent 0-100
	Encoder float64 // percent 0-100
	Decoder float64 // percent 0-100

	Process []Process

	Extension interface{}
}

type GPU interface {
	// Count returns the number of GPU in the system.
	Count() (int, error)

	// Stats returns current GPU stats.
	Stats() ([]Stats, error)

	// Process returns a Process.
	Process(pid int32) (Process, error)

	// Close stops all GPU collection processes
	Close()
}

var ErrProcessNotFound = errors.New("process not found")

type dummy struct{}

func (d *dummy) Count() (int, error)                { return 0, nil }
func (d *dummy) Stats() ([]Stats, error)            { return nil, nil }
func (d *dummy) Process(pid int32) (Process, error) { return Process{}, ErrProcessNotFound }
func (d *dummy) Close()                             {}

func NewNilGPU() GPU {
	return &dummy{}
}
