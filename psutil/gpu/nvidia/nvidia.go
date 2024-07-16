package nvidia

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/datarhei/core/v16/psutil/gpu"
)

var Default gpu.GPU

func init() {
	Default = New("")
}

type Megabytes uint64

func (m *Megabytes) UnmarshalText(text []byte) error {
	value := uint64(0)
	_, err := fmt.Sscanf(string(text), "%d MiB", &value)
	if err != nil {
		return err
	}

	*m = Megabytes(value * 1024 * 1024)

	return nil
}

type Utilization float64

func (u *Utilization) UnmarshalText(text []byte) error {
	value := float64(0)
	_, err := fmt.Sscanf(string(text), "%f %%", &value)
	if err != nil {
		return err
	}

	*u = Utilization(value)

	return nil
}

type Process struct {
	PID    int32     `xml:"pid"`
	Memory Megabytes `xml:"used_memory"`
}

type GPUStats struct {
	Name         string `xml:"product_name"`
	Architecture string `xml:"product_architecture"`

	MemoryTotal Megabytes `xml:"fb_memory_usage>total"`
	MemoryUsed  Megabytes `xml:"fb_memory_usage>used"`

	Usage        Utilization `xml:"utilization>gpu_util"`
	MemoryUsage  Utilization `xml:"utilization>memory_util"`
	EncoderUsage Utilization `xml:"utilization>encoder_util"`
	DecoderUsage Utilization `xml:"utilization>decoder_util"`

	Process []Process `xml:"processes>process_info"`
}

type Stats struct {
	GPU []GPUStats `xml:"gpu"`
}

func parse(data []byte) (Stats, error) {
	nv := Stats{}

	err := xml.Unmarshal(data, &nv)
	if err != nil {
		return nv, fmt.Errorf("parsing report: %w", err)
	}

	return nv, nil
}

type nvidia struct {
	cmd *exec.Cmd
	wr  *writer

	lock    sync.RWMutex
	cancel  context.CancelFunc
	stats   Stats
	process map[int32]Process
	err     error
}

type dummy struct{}

func (d *dummy) Count() (int, error)                    { return 0, nil }
func (d *dummy) Stats() ([]gpu.Stats, error)            { return nil, nil }
func (d *dummy) Process(pid int32) (gpu.Process, error) { return gpu.Process{}, gpu.ErrProcessNotFound }

type writer struct {
	buf bytes.Buffer
	ch  chan Stats
}

var terminator = []byte("</nvidia_smi_log>\n")

func (w *writer) Write(data []byte) (int, error) {
	n, err := w.buf.Write(data)
	if err != nil {
		return n, err
	}

	for {
		idx := bytes.Index(w.buf.Bytes(), terminator)
		if idx == -1 {
			break
		}

		content := make([]byte, idx+len(terminator))
		n, err := w.buf.Read(content)
		if err != nil || n != len(content) {
			break
		}

		s, err := parse(content)
		if err != nil {
			continue
		}

		w.ch <- s
	}

	return n, nil
}

func New(path string) gpu.GPU {
	if len(path) == 0 {
		path = "nvidia-smi"
	}

	_, err := exec.LookPath(path)
	if err != nil {
		return &dummy{}
	}

	n := &nvidia{
		wr: &writer{
			ch: make(chan Stats, 1),
		},
		process: map[int32]Process{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	n.cancel = cancel

	go n.runner(ctx, path)
	go n.reader(ctx)

	return n
}

func (n *nvidia) reader(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case stats := <-n.wr.ch:
			n.lock.Lock()
			n.stats = stats
			n.process = map[int32]Process{}
			for _, g := range n.stats.GPU {
				for _, p := range g.Process {
					n.process[p.PID] = p
				}
			}
			n.lock.Unlock()
		}
	}
}

func (n *nvidia) runner(ctx context.Context, path string) {
	for {
		n.cmd = exec.Command(path, "-q", "-x", "-l", "1")
		n.cmd.Stdout = n.wr
		err := n.cmd.Start()
		if err != nil {
			n.lock.Lock()
			n.err = err
			n.lock.Unlock()

			time.Sleep(3 * time.Second)
			continue
		}

		err = n.cmd.Wait()

		n.lock.Lock()
		n.err = err
		n.lock.Unlock()

		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

func (n *nvidia) Count() (int, error) {
	n.lock.RLock()
	defer n.lock.RUnlock()

	if n.err != nil {
		return 0, n.err
	}

	return len(n.stats.GPU), nil
}

func (n *nvidia) Stats() ([]gpu.Stats, error) {
	s := []gpu.Stats{}

	n.lock.RLock()
	defer n.lock.RUnlock()

	if n.err != nil {
		return s, n.err
	}

	for _, nv := range n.stats.GPU {
		stats := gpu.Stats{
			Name:         nv.Name,
			Architecture: nv.Architecture,
			MemoryTotal:  uint64(nv.MemoryTotal),
			MemoryUsed:   uint64(nv.MemoryUsed),
			Usage:        float64(nv.Usage),
			MemoryUsage:  float64(nv.MemoryUsage),
			EncoderUsage: float64(nv.EncoderUsage),
			DecoderUsage: float64(nv.DecoderUsage),
			Process:      []gpu.Process{},
		}

		for _, p := range nv.Process {
			stats.Process = append(stats.Process, gpu.Process{
				PID:    p.PID,
				Memory: uint64(p.Memory),
			})
		}

		s = append(s, stats)
	}

	return s, nil
}

func (n *nvidia) Process(pid int32) (gpu.Process, error) {
	n.lock.RLock()
	defer n.lock.RUnlock()

	p, hasProcess := n.process[pid]
	if !hasProcess {
		return gpu.Process{}, gpu.ErrProcessNotFound
	}

	return gpu.Process{
		PID:    p.PID,
		Memory: uint64(p.Memory),
	}, nil
}

func (n *nvidia) Close() {
	n.lock.Lock()
	defer n.lock.Unlock()

	if n.cancel == nil {
		return
	}

	n.cancel()
	n.cancel = nil

	n.cmd.Process.Kill()
}
