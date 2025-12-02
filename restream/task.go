package restream

import (
	"maps"
	"sync/atomic"
	"time"

	"github.com/datarhei/core/v16/event"
	"github.com/datarhei/core/v16/ffmpeg/parse"
	"github.com/datarhei/core/v16/glob"
	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/process"
	"github.com/datarhei/core/v16/restream/app"
)

type task struct {
	readers   *atomic.Int64          // Number of concurrent readers
	id        string                 // ID of the task/process
	owner     string                 // Owner of the process
	domain    string                 // Domain of the process
	reference string                 // reference of the process
	process   *app.Process           // The process definition
	config    *app.Config            // Process config with replaced static placeholders
	command   []string               // The actual command parameter for ffmpeg
	ffmpeg    process.Process        // The OS process
	parser    parse.Parser           // Parser for the OS process' output
	playout   map[string]int         // Port mapping to access playout API
	logger    log.Logger             // Logger
	usesDisk  bool                   // Whether this task uses the disk
	hwdevice  *atomic.Int32          // Index of the GPU this task uses
	metadata  map[string]interface{} // Metadata of the process
}

func NewTask(process *app.Process, logger log.Logger) *task {
	t := &task{
		readers:   &atomic.Int64{},
		id:        process.ID,
		owner:     process.Owner,
		domain:    process.Domain,
		reference: process.Reference,
		process:   process,
		config:    process.Config.Clone(),
		playout:   map[string]int{},
		logger:    logger,
		hwdevice:  &atomic.Int32{},
		metadata:  nil,
	}

	return t
}

func (t *task) UsesDisk() bool {
	return t.usesDisk
}

func (t *task) ID() app.ProcessID {
	return app.ProcessID{
		ID:     t.id,
		Domain: t.domain,
	}
}

func (t *task) String() string {
	return t.ID().String()
}

// Restore restores the task's order
func (t *task) Restore() error {
	if t.process.Order.String() == "start" {
		err := t.ffmpeg.Start()
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *task) Start() error {
	t.process.Order.Set("start")

	t.ffmpeg.Start()

	return nil
}

func (t *task) Stop() error {
	t.process.Order.Set("stop")

	t.ffmpeg.Stop(true)

	return nil
}

// Kill stops a process without changing the tasks order
func (t *task) Kill() {
	t.ffmpeg.Stop(true)
}

func (t *task) Restart() error {
	if t.process.Order.String() == "stop" {
		return nil
	}

	if t.ffmpeg != nil {
		t.ffmpeg.Stop(true)
		t.ffmpeg.Start()
	}

	return nil
}

func (t *task) State() (*app.State, error) {
	state := &app.State{}

	status := t.ffmpeg.Status()

	state.Order = t.process.Order.String()
	state.State = status.State
	state.States.Marshal(status.States)
	state.Time = status.Time.Unix()
	state.Memory = status.Memory.Current
	state.CPU = status.CPU.Current / status.CPU.NCPU
	state.LimitMode = status.LimitMode
	state.Resources.CPU = app.ProcessUsageCPU{
		NCPU:         status.CPU.NCPU,
		Current:      status.CPU.Current,
		Average:      status.CPU.Average,
		Max:          status.CPU.Max,
		Limit:        status.CPU.Limit,
		IsThrottling: status.CPU.IsThrottling,
	}
	state.Resources.Memory = app.ProcessUsageMemory{
		Current: status.Memory.Current,
		Average: status.Memory.Average,
		Max:     status.Memory.Max,
		Limit:   status.Memory.Limit,
	}
	state.Resources.GPU = app.ProcessUsageGPU{
		Index: status.GPU.Index,
		Usage: app.ProcessUsageGPUUsage{
			Current: status.GPU.Usage.Current,
			Average: status.GPU.Usage.Average,
			Max:     status.GPU.Usage.Max,
			Limit:   status.GPU.Usage.Limit,
		},
		Encoder: app.ProcessUsageGPUUsage{
			Current: status.GPU.Encoder.Current,
			Average: status.GPU.Encoder.Average,
			Max:     status.GPU.Encoder.Max,
			Limit:   status.GPU.Encoder.Limit,
		},
		Decoder: app.ProcessUsageGPUUsage{
			Current: status.GPU.Decoder.Current,
			Average: status.GPU.Decoder.Average,
			Max:     status.GPU.Decoder.Max,
			Limit:   status.GPU.Decoder.Limit,
		},
		Memory: app.ProcessUsageGPUMemory{
			Current: status.GPU.Memory.Current,
			Average: status.GPU.Memory.Average,
			Max:     status.GPU.Memory.Max,
			Limit:   status.GPU.Memory.Limit,
		},
	}
	state.Duration = status.Duration.Round(10 * time.Millisecond).Seconds()
	state.Reconnect = -1
	state.Command = status.CommandArgs
	state.LastLog = t.parser.LastLogline()

	if status.Reconnect >= time.Duration(0) {
		state.Reconnect = status.Reconnect.Round(10 * time.Millisecond).Seconds()
	}

	progress := t.parser.Progress()
	state.Progress.UnmarshalParser(&progress)

	state.Progress.Input = assignConfigID(state.Progress.Input, t.config.Input)
	state.Progress.Output = assignConfigID(state.Progress.Output, t.config.Output)

	state.PID = status.PID

	return state, nil
}

func assignConfigID(progress []app.ProgressIO, config []app.ConfigIO) []app.ProgressIO {
	for i, p := range progress {
		for _, c := range config {
			if c.Address != p.URL {
				continue
			}

			progress[i].ID = c.ID

			break
		}
	}

	return progress
}

func (t *task) Report() (*app.Report, error) {
	report := &app.Report{}

	current := t.parser.Report()

	report.UnmarshalParser(&current)

	history := t.parser.ReportHistory()

	report.History = make([]app.ReportHistoryEntry, len(history))

	for i, h := range history {
		report.History[i].UnmarshalParser(&h)
		e := &report.History[i]

		e.Progress.Input = assignConfigID(e.Progress.Input, t.config.Input)
		e.Progress.Output = assignConfigID(e.Progress.Output, t.config.Input)
	}

	return report, nil
}

func (t *task) SetReport(report *app.Report) error {
	_, history := report.MarshalParser()

	t.parser.ImportReportHistory(history)

	return nil
}

func (t *task) SearchReportHistory(state string, from, to *time.Time) []app.ReportHistorySearchResult {
	result := []app.ReportHistorySearchResult{}

	presult := t.parser.SearchReportHistory(state, from, to)

	for _, f := range presult {
		result = append(result, app.ReportHistorySearchResult{
			ProcessID: t.id,
			Domain:    t.domain,
			Reference: t.reference,
			ExitState: f.ExitState,
			CreatedAt: f.CreatedAt,
			ExitedAt:  f.ExitedAt,
		})
	}

	return result
}

func (t *task) SetMetadata(key string, data interface{}) error {
	if len(key) == 0 {
		return ErrMetadataKeyRequired
	}

	if t.metadata == nil {
		t.metadata = make(map[string]interface{})
	}

	if data == nil {
		delete(t.metadata, key)
	} else {
		t.metadata[key] = data
	}

	if len(t.metadata) == 0 {
		t.metadata = nil
	}

	return nil
}

func (t *task) ImportMetadata(m map[string]interface{}) {
	t.metadata = m
}

func (t *task) GetMetadata(key string) (interface{}, error) {
	if len(key) == 0 {
		if t.metadata == nil {
			return nil, nil
		}

		return maps.Clone(t.metadata), nil
	}

	if t.metadata == nil {
		return nil, ErrMetadataKeyNotFound
	}

	data, ok := t.metadata[key]
	if !ok {
		return nil, ErrMetadataKeyNotFound
	}

	return data, nil
}

func (t *task) ExportMetadata() map[string]interface{} {
	return t.metadata
}

func (t *task) Limit(cpu, memory, gpu bool) bool {
	t.ffmpeg.Limit(cpu, memory, gpu)

	return true
}

func (t *task) SetHWDevice(index int) {
	t.hwdevice.Store(int32(index))
}

func (t *task) GetHWDevice() int {
	return int(t.hwdevice.Load())
}

func (t *task) Equal(config *app.Config) bool {
	return t.process.Config.Equal(config)
}

func (t *task) ResolvedConfig() *app.Config {
	return t.config.Clone()
}

func (t *task) Config() *app.Config {
	return t.process.Config.Clone()
}

func (t *task) Destroy() {
	t.Stop()

	t.parser.Destroy()
}

func (t *task) Match(id, reference, owner, domain glob.Glob) bool {
	count := 0
	matches := 0

	if id != nil {
		count++
		if match := id.Match(t.id); match {
			matches++
		}
	}

	if reference != nil {
		count++
		if match := reference.Match(t.reference); match {
			matches++
		}
	}

	if owner != nil {
		count++
		if match := owner.Match(t.owner); match {
			matches++
		}
	}

	if domain != nil {
		count++
		if match := domain.Match(t.domain); match {
			matches++
		}
	}

	return count == matches
}

func (t *task) Process() *app.Process {
	return t.process.Clone()
}

func (t *task) Order() string {
	return t.process.Order.String()
}

func (t *task) ExportParserReportHistory() []parse.ReportHistoryEntry {
	return t.parser.ReportHistory()
}

func (t *task) ImportParserReportHistory(report []parse.ReportHistoryEntry) {
	t.parser.ImportReportHistory(report)
}

func (t *task) Events() (<-chan event.Event, event.CancelFunc, error) {
	return t.parser.Events()
}
