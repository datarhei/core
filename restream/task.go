package restream

import (
	"errors"
	"maps"
	"time"

	"github.com/datarhei/core/v16/ffmpeg/parse"
	"github.com/datarhei/core/v16/glob"
	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/math/rand"
	"github.com/datarhei/core/v16/process"
	"github.com/datarhei/core/v16/restream/app"

	"github.com/puzpuzpuz/xsync/v3"
)

var ErrInvalidProcessConfig = errors.New("invalid process config")
var ErrMetadataKeyNotFound = errors.New("unknown metadata key")
var ErrMetadataKeyRequired = errors.New("a key for storing metadata is required")

type task struct {
	valid     bool
	id        string // ID of the task/process
	owner     string
	domain    string
	reference string
	process   *app.Process
	config    *app.Config // Process config with replaced static placeholders
	command   []string    // The actual command parameter for ffmpeg
	ffmpeg    process.Process
	parser    parse.Parser
	playout   map[string]int
	logger    log.Logger
	usesDisk  bool // Whether this task uses the disk
	hwdevice  int  // Index of the GPU this task uses
	metadata  map[string]interface{}

	lock   *xsync.RBMutex
	tokens *xsync.MapOf[string, *xsync.RToken]
}

func NewTask(process *app.Process, logger log.Logger) *task {
	t := &task{
		id:        process.ID,
		owner:     process.Owner,
		domain:    process.Domain,
		reference: process.Reference,
		process:   process,
		config:    process.Config.Clone(),
		playout:   map[string]int{},
		logger:    logger,
		metadata:  nil,
		lock:      xsync.NewRBMutex(),
		tokens:    xsync.NewMapOf[string, *xsync.RToken](),
	}

	return t
}

func (t *task) Lock() {
	t.lock.Lock()
}

func (t *task) Unlock() {
	t.lock.Unlock()
}

func (t *task) RLock() string {
	token := ""
	for {
		token = rand.String(16)
		rtoken := t.lock.RLock()

		_, loaded := t.tokens.LoadOrStore(token, rtoken)
		if !loaded {
			break
		}

		t.lock.RUnlock(rtoken)
	}

	return token
}

func (t *task) Release(token string) {
	rtoken, ok := t.tokens.LoadAndDelete(token)
	if !ok {
		return
	}

	t.lock.RUnlock(rtoken)
}

func (t *task) IsValid() bool {
	return t.valid
}

func (t *task) SetValid(valid bool) {
	t.valid = valid
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
	if !t.valid {
		return ErrInvalidProcessConfig
	}

	if t.ffmpeg == nil {
		return ErrInvalidProcessConfig
	}

	if t.process == nil {
		return ErrInvalidProcessConfig
	}

	if t.process.Order.String() == "start" {
		err := t.ffmpeg.Start()
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *task) Start() error {
	if !t.valid {
		return ErrInvalidProcessConfig
	}

	if t.ffmpeg == nil {
		return nil
	}

	if t.process == nil {
		return nil
	}

	status := t.ffmpeg.Status()

	if t.process.Order.String() == "start" && status.Order == "start" {
		return nil
	}

	t.process.Order.Set("start")

	t.ffmpeg.Start()

	return nil
}

func (t *task) Stop() error {
	if t.ffmpeg == nil {
		return nil
	}

	if t.process == nil {
		return nil
	}

	status := t.ffmpeg.Status()

	if t.process.Order.String() == "stop" && status.Order == "stop" {
		return nil
	}

	t.process.Order.Set("stop")

	t.ffmpeg.Stop(true)

	return nil
}

// Kill stops a process without changing the tasks order
func (t *task) Kill() {
	if t.ffmpeg == nil {
		return
	}

	t.ffmpeg.Stop(true)
}

func (t *task) Restart() error {
	if !t.valid {
		return ErrInvalidProcessConfig
	}

	if t.process == nil {
		return nil
	}

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

	if !t.valid {
		return state, nil
	}

	if t.ffmpeg == nil {
		return state, nil
	}

	if t.parser == nil {
		return state, nil
	}

	if t.process == nil {
		return state, nil
	}

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

	if !t.valid {
		return report, nil
	}

	if t.parser == nil {
		return report, nil
	}

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
	if !t.valid {
		return nil
	}

	if t.parser == nil {
		return nil
	}

	_, history := report.MarshalParser()

	t.parser.ImportReportHistory(history)

	return nil
}

func (t *task) SearchReportHistory(state string, from, to *time.Time) []app.ReportHistorySearchResult {
	if t.parser == nil {
		return []app.ReportHistorySearchResult{}
	}

	result := []app.ReportHistorySearchResult{}

	presult := t.parser.SearchReportHistory(state, from, to)

	for _, f := range presult {
		result = append(result, app.ReportHistorySearchResult{
			ProcessID: t.id,
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
	if t.ffmpeg == nil {
		return false
	}

	t.ffmpeg.Limit(cpu, memory, gpu)

	return true
}

func (t *task) SetHWDevice(index int) {
	t.hwdevice = index
}

func (t *task) GetHWDevice() int {
	return t.hwdevice
}

func (t *task) Equal(config *app.Config) bool {
	if t.process == nil {
		return false
	}

	return t.process.Config.Equal(config)
}

func (t *task) ResolvedConfig() *app.Config {
	if t.config == nil {
		return nil
	}

	return t.config.Clone()
}

func (t *task) Config() *app.Config {
	if t.process == nil {
		return nil
	}

	return t.process.Config.Clone()
}

func (t *task) Destroy() {
	t.Stop()

	t.valid = false
	t.process = nil
	t.config = nil
	t.command = nil
	t.ffmpeg = nil
	t.parser = nil
	t.metadata = map[string]interface{}{}
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
	if t.process == nil {
		return nil
	}

	return t.process.Clone()
}

func (t *task) Order() string {
	if t.process == nil {
		return ""
	}

	return t.process.Order.String()
}

func (t *task) ExportParserReportHistory() []parse.ReportHistoryEntry {
	if t.parser == nil {
		return nil
	}

	return t.parser.ReportHistory()
}

func (t *task) ImportParserReportHistory(report []parse.ReportHistoryEntry) {
	if t.parser == nil {
		return
	}

	t.parser.ImportReportHistory(report)
}
