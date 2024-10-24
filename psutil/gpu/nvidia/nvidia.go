package nvidia

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
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
	Index  int
	PID    int32
	Memory uint64 // bytes

	Usage   float64 // percent 0-100
	Encoder float64 // percent 0-100
	Decoder float64 // percent 0-100

	lastSeen time.Time
}

type GPUStats struct {
	ID           string `xml:"id,attr"`
	Name         string `xml:"product_name"`
	Architecture string `xml:"product_architecture"`

	MemoryTotal Megabytes `xml:"fb_memory_usage>total"`
	MemoryUsed  Megabytes `xml:"fb_memory_usage>used"`

	Usage        Utilization `xml:"utilization>gpu_util"`
	UsageEncoder Utilization `xml:"utilization>encoder_util"`
	UsageDecoder Utilization `xml:"utilization>decoder_util"`
}

type Stats struct {
	GPU []GPUStats `xml:"gpu"`
}

type nvidia struct {
	wrQuery   *writerQuery
	wrProcess *writerProcess

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
func (d *dummy) Close()                                 {}

type writerQuery struct {
	buf        bytes.Buffer
	ch         chan Stats
	terminator []byte
}

func (w *writerQuery) Write(data []byte) (int, error) {
	n, err := w.buf.Write(data)
	if err != nil {
		return n, err
	}

	for {
		idx := bytes.Index(w.buf.Bytes(), w.terminator)
		if idx == -1 {
			break
		}

		content := make([]byte, idx+len(w.terminator))
		n, err := w.buf.Read(content)
		if err != nil || n != len(content) {
			break
		}

		s, err := w.parse(content)
		if err != nil {
			continue
		}

		w.ch <- s
	}

	return n, nil
}

func (w *writerQuery) parse(data []byte) (Stats, error) {
	nv := Stats{}

	err := xml.Unmarshal(data, &nv)
	if err != nil {
		return nv, fmt.Errorf("parsing report: %w", err)
	}

	return nv, nil
}

type writerProcess struct {
	buf        bytes.Buffer
	ch         chan Process
	re         *regexp.Regexp
	terminator []byte
}

func (w *writerProcess) Write(data []byte) (int, error) {
	n, err := w.buf.Write(data)
	if err != nil {
		return n, err
	}

	for {
		idx := bytes.Index(w.buf.Bytes(), w.terminator)
		if idx == -1 {
			break
		}

		content := make([]byte, idx+len(w.terminator))
		n, err := w.buf.Read(content)
		if err != nil || n != len(content) {
			break
		}

		s, err := w.parse(content)
		if err != nil {
			continue
		}

		w.ch <- s
	}

	return n, nil
}

func (w *writerProcess) parse(data []byte) (Process, error) {
	p := Process{}

	if len(data) == 0 {
		return p, fmt.Errorf("empty line")
	}

	if data[0] == '#' {
		return p, fmt.Errorf("comment")
	}

	matches := w.re.FindStringSubmatch(string(data))
	if matches == nil {
		return p, fmt.Errorf("no matches found")
	}

	if len(matches) != 7 {
		return p, fmt.Errorf("not the expected number of matches found")
	}

	if d, err := strconv.ParseInt(matches[1], 10, 0); err == nil {
		p.Index = int(d)
	}

	if d, err := strconv.ParseInt(matches[2], 10, 32); err == nil {
		p.PID = int32(d)
	}

	if matches[3][0] != '-' {
		if d, err := strconv.ParseFloat(matches[3], 64); err == nil {
			p.Usage = d
		}
	}

	if matches[4][0] != '-' {
		if d, err := strconv.ParseFloat(matches[4], 64); err == nil {
			p.Encoder = d
		}
	}

	if matches[5][0] != '-' {
		if d, err := strconv.ParseFloat(matches[5], 64); err == nil {
			p.Decoder = d
		}
	}

	if d, err := strconv.ParseUint(matches[6], 10, 64); err == nil {
		p.Memory = d * 1024 * 1024
	}

	return p, nil
}

func New(path string) gpu.GPU {
	if len(path) == 0 {
		path = "nvidia-smi"
	}

	path, err := exec.LookPath(path)
	if err != nil {
		return &dummy{}
	}

	n := &nvidia{
		wrQuery: &writerQuery{
			ch:         make(chan Stats, 1),
			terminator: []byte("</nvidia_smi_log>\n"),
		},
		wrProcess: &writerProcess{
			ch: make(chan Process, 32),
			// # gpu        pid  type    sm   mem   enc   dec    fb   command
			// # Idx          #   C/G     %     %     %     %    MB   name
			//     0       7372     C     2     0     2     -   136   ffmpeg
			//     0      12176     C     5     2     3     7   782   ffmpeg
			//     0      20035     C     8     2     4     1  1145   ffmpeg
			//     0      20141     C     2     1     1     3   429   ffmpeg
			//     0      29591     C     2     1     -     2   435   ffmpeg
			re:         regexp.MustCompile(`^\s*([0-9]+)\s+([0-9]+)\s+[A-Z]\s+([0-9-]+)\s+[0-9-]+\s+([0-9-]+)\s+([0-9-]+)\s+([0-9]+).*`),
			terminator: []byte("\n"),
		},
		process: map[int32]Process{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	n.cancel = cancel

	go n.runnerQuery(ctx, path)
	go n.runnerProcess(ctx, path)
	go n.reader(ctx)

	return n
}

func (n *nvidia) reader(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case stats := <-n.wrQuery.ch:
			n.lock.Lock()
			n.stats = stats
			n.lock.Unlock()
		case process := <-n.wrProcess.ch:
			process.lastSeen = time.Now()
			n.lock.Lock()
			n.process[process.PID] = process

			for pid, p := range n.process {
				if time.Since(p.lastSeen) > 11*time.Second {
					delete(n.process, pid)
				}
			}
			n.lock.Unlock()
		}
	}
}

func (n *nvidia) runnerQuery(ctx context.Context, path string) {
	for {
		cmd := exec.CommandContext(ctx, path, "-q", "-x", "-l", "1")
		cmd.Stdout = n.wrQuery
		err := cmd.Start()
		if err != nil {
			n.lock.Lock()
			n.err = err
			n.lock.Unlock()

			time.Sleep(3 * time.Second)
			continue
		}

		err = cmd.Wait()

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

func (n *nvidia) runnerProcess(ctx context.Context, path string) {
	for {
		cmd := exec.CommandContext(ctx, path, "pmon", "-s", "um", "-d", "5")
		cmd.Stdout = n.wrProcess
		err := cmd.Start()
		if err != nil {
			n.lock.Lock()
			n.err = err
			n.lock.Unlock()

			time.Sleep(3 * time.Second)
			continue
		}

		err = cmd.Wait()

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
	stats := []gpu.Stats{}

	n.lock.RLock()
	defer n.lock.RUnlock()

	if n.err != nil {
		return stats, n.err
	}

	for _, nv := range n.stats.GPU {
		s := gpu.Stats{
			ID:           nv.ID,
			Name:         nv.Name,
			Architecture: nv.Architecture,
			MemoryTotal:  uint64(nv.MemoryTotal),
			MemoryUsed:   uint64(nv.MemoryUsed),
			Usage:        float64(nv.Usage),
			Encoder:      float64(nv.UsageEncoder),
			Decoder:      float64(nv.UsageDecoder),
			Process:      []gpu.Process{},
		}

		stats = append(stats, s)
	}

	for _, p := range n.process {
		if p.Index >= len(stats) {
			continue
		}

		stats[p.Index].Process = append(stats[p.Index].Process, gpu.Process{
			PID:     p.PID,
			Index:   p.Index,
			Memory:  p.Memory,
			Usage:   p.Usage,
			Encoder: p.Encoder,
			Decoder: p.Decoder,
		})
	}

	for i := range stats {
		p := stats[i].Process
		slices.SortFunc(p, func(a, b gpu.Process) int {
			return int(a.PID - b.PID)
		})
		stats[i].Process = p
	}

	return stats, nil
}

func (n *nvidia) Process(pid int32) (gpu.Process, error) {
	n.lock.RLock()
	defer n.lock.RUnlock()

	p, hasProcess := n.process[pid]
	if hasProcess {
		return gpu.Process{
			PID:     p.PID,
			Index:   p.Index,
			Memory:  p.Memory,
			Usage:   p.Usage,
			Encoder: p.Encoder,
			Decoder: p.Decoder,
		}, nil
	}

	return gpu.Process{Index: -1}, gpu.ErrProcessNotFound
}

func (n *nvidia) Close() {
	n.lock.Lock()
	defer n.lock.Unlock()

	if n.cancel == nil {
		return
	}

	n.cancel()
	n.cancel = nil
}
