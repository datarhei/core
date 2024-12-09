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
	"strings"
	"sync"
	"time"

	"github.com/datarhei/core/v16/resources/psutil/gpu"
)

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

		s, err := parseQuery(content)
		if err != nil {
			continue
		}

		w.ch <- s
	}

	return n, nil
}

func parseQuery(data []byte) (Stats, error) {
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
	terminator []byte
	matcher    *processMatcher
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

		s, err := w.matcher.Parse(content)
		if err != nil {
			continue
		}

		w.ch <- s
	}

	return n, nil
}

type processMatcher struct {
	re      *regexp.Regexp
	mapping map[string]int
}

func newProcessMatcher() *processMatcher {
	m := &processMatcher{
		re:      regexp.MustCompile(`\s+`),
		mapping: map[string]int{},
	}

	return m
}

func (m *processMatcher) Parse(data []byte) (Process, error) {
	p := Process{}

	if len(data) == 0 {
		return p, fmt.Errorf("empty line")
	}

	line := string(data)

	if strings.HasPrefix(line, "# gpu") {
		m.mapping = map[string]int{}
		columns := m.re.Split(strings.TrimPrefix(line, "# "), -1)
		for i, column := range columns {
			m.mapping[column] = i
		}
	}

	if line[0] == '#' {
		return p, fmt.Errorf("comment")
	}

	columns := m.re.Split(strings.TrimSpace(line), -1)
	if len(columns) == 0 {
		return p, fmt.Errorf("no matches found")
	}

	if columns[0] == line {
		return p, fmt.Errorf("no matches found")
	}

	if index, ok := m.mapping["gpu"]; ok {
		if d, err := strconv.ParseInt(columns[index], 10, 0); err == nil {
			p.Index = int(d)
		}
	}

	if index, ok := m.mapping["pid"]; ok {
		if d, err := strconv.ParseInt(columns[index], 10, 32); err == nil {
			p.PID = int32(d)
		}
	}

	if index, ok := m.mapping["sm"]; ok {
		if columns[index] != "-" {
			if d, err := strconv.ParseFloat(columns[index], 64); err == nil {
				p.Usage = d
			}
		}
	}

	if index, ok := m.mapping["enc"]; ok {
		if columns[index] != "-" {
			if d, err := strconv.ParseFloat(columns[index], 64); err == nil {
				p.Encoder = d
			}
		}
	}

	if index, ok := m.mapping["dec"]; ok {
		if columns[index] != "-" {
			if d, err := strconv.ParseFloat(columns[index], 64); err == nil {
				p.Decoder = d
			}
		}
	}

	if index, ok := m.mapping["fb"]; ok {
		if columns[index] != "-" {
			if d, err := strconv.ParseUint(columns[index], 10, 64); err == nil {
				p.Memory = d * 1024 * 1024
			}
		}
	}

	if p.PID == 0 {
		return p, fmt.Errorf("no process found")
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
		process: map[int32]Process{},
	}

	stats, err := n.runQueryOnce(path)
	if err != nil {
		return &dummy{}
	}

	n.stats = stats

	process, err := n.runProcessOnce(path)
	if err != nil {
		return &dummy{}
	}

	n.process = process

	n.wrQuery = &writerQuery{
		ch:         make(chan Stats, 1),
		terminator: []byte("</nvidia_smi_log>\n"),
	}
	n.wrProcess = &writerProcess{
		ch:         make(chan Process, 32),
		terminator: []byte("\n"),
		matcher:    newProcessMatcher(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	n.cancel = cancel

	go n.reader(ctx)
	go n.runnerQuery(ctx, path)
	go n.runnerProcess(ctx, path)

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

func (n *nvidia) runQueryOnce(path string) (Stats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	data := &bytes.Buffer{}

	cmd := exec.CommandContext(ctx, path, "-q", "-x")
	cmd.Stdout = data
	err := cmd.Start()
	if err != nil {
		return Stats{}, err
	}

	err = cmd.Wait()
	if err != nil {
		return Stats{}, err
	}

	stats, err := parseQuery(data.Bytes())
	if err != nil {
		return Stats{}, err
	}

	return stats, nil
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

func (n *nvidia) runProcessOnce(path string) (map[int32]Process, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	data := &bytes.Buffer{}

	cmd := exec.CommandContext(ctx, path, "pmon", "-s", "um", "-c", "1")
	cmd.Stdout = data
	err := cmd.Start()
	if err != nil {
		return nil, err
	}

	err = cmd.Wait()
	if err != nil {
		return nil, err
	}

	matcher := newProcessMatcher()

	lines := bytes.Split(data.Bytes(), []byte{'\n'})

	process := map[int32]Process{}

	for _, line := range lines {
		p, err := matcher.Parse(line)
		if err != nil {
			continue
		}

		process[p.PID] = p
	}

	return process, nil
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

	for i, nv := range n.stats.GPU {
		s := gpu.Stats{
			Index:        i,
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
