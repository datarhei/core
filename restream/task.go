package restream

import (
	"errors"
	"time"

	"github.com/datarhei/core/v16/ffmpeg/parse"
	"github.com/datarhei/core/v16/glob"
	"github.com/datarhei/core/v16/log"
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
	metadata  map[string]interface{}

	lock *xsync.RBMutex
}

func (t *task) IsValid() bool {
	token := t.lock.RLock()
	defer t.lock.RUnlock(token)

	return t.valid
}

func (t *task) Valid(valid bool) {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.valid = valid
}

func (t *task) UsesDisk() bool {
	token := t.lock.RLock()
	defer t.lock.RUnlock(token)

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
	token := t.lock.RLock()
	defer t.lock.RUnlock(token)

	if !t.valid {
		return ErrInvalidProcessConfig
	}

	if t.ffmpeg == nil {
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
	token := t.lock.RLock()
	defer t.lock.RUnlock(token)

	if !t.valid {
		return ErrInvalidProcessConfig
	}

	if t.ffmpeg == nil {
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
	token := t.lock.RLock()
	defer t.lock.RUnlock(token)

	if t.ffmpeg == nil {
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
	token := t.lock.RLock()
	defer t.lock.RUnlock(token)

	if t.ffmpeg == nil {
		return
	}

	t.ffmpeg.Stop(true)
}

func (t *task) Restart() error {
	token := t.lock.RLock()
	defer t.lock.RUnlock(token)

	if !t.valid {
		return ErrInvalidProcessConfig
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
	token := t.lock.RLock()
	defer t.lock.RUnlock(token)

	state := &app.State{}

	if !t.valid {
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
	state.Resources.CPU = status.CPU
	state.Resources.Memory = status.Memory
	state.Duration = status.Duration.Round(10 * time.Millisecond).Seconds()
	state.Reconnect = -1
	state.Command = status.CommandArgs
	state.LastLog = t.parser.LastLogline()

	if status.Reconnect >= time.Duration(0) {
		state.Reconnect = status.Reconnect.Round(10 * time.Millisecond).Seconds()
	}

	progress := t.parser.Progress()
	state.Progress.UnmarshalParser(&progress)

	for i, p := range state.Progress.Input {
		if int(p.Index) >= len(t.process.Config.Input) {
			continue
		}

		state.Progress.Input[i].ID = t.process.Config.Input[p.Index].ID
	}

	for i, p := range state.Progress.Output {
		if int(p.Index) >= len(t.process.Config.Output) {
			continue
		}

		state.Progress.Output[i].ID = t.process.Config.Output[p.Index].ID
	}

	return state, nil
}

func (t *task) Report() (*app.Report, error) {
	token := t.lock.RLock()
	defer t.lock.RUnlock(token)

	report := &app.Report{}

	if !t.valid {
		return report, nil
	}

	current := t.parser.Report()

	report.UnmarshalParser(&current)

	history := t.parser.ReportHistory()

	report.History = make([]app.ReportHistoryEntry, len(history))

	for i, h := range history {
		report.History[i].UnmarshalParser(&h)
		e := &report.History[i]

		for i, p := range e.Progress.Input {
			if int(p.Index) >= len(t.process.Config.Input) {
				continue
			}

			e.Progress.Input[i].ID = t.process.Config.Input[p.Index].ID
		}

		for i, p := range e.Progress.Output {
			if int(p.Index) >= len(t.process.Config.Output) {
				continue
			}

			e.Progress.Output[i].ID = t.process.Config.Output[p.Index].ID
		}
	}

	return report, nil
}

func (t *task) SetReport(report *app.Report) error {
	token := t.lock.RLock()
	defer t.lock.RUnlock(token)

	if !t.valid {
		return nil
	}

	_, history := report.MarshalParser()

	t.parser.ImportReportHistory(history)

	return nil
}

func (t *task) SearchReportHistory(state string, from, to *time.Time) []app.ReportHistorySearchResult {
	token := t.lock.RLock()
	defer t.lock.RUnlock(token)

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
	t.lock.Lock()
	defer t.lock.Unlock()

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

func (t *task) GetMetadata(key string) (interface{}, error) {
	token := t.lock.RLock()
	defer t.lock.RUnlock(token)

	if len(key) == 0 {
		return t.metadata, nil
	}

	data, ok := t.metadata[key]
	if !ok {
		return nil, ErrMetadataKeyNotFound
	}

	return data, nil
}

func (t *task) Limit(cpu, memory bool) bool {
	token := t.lock.RLock()
	defer t.lock.RUnlock(token)

	if !t.valid {
		return false
	}

	if t.ffmpeg == nil {
		return false
	}

	t.ffmpeg.Limit(cpu, memory)

	return true
}

func (t *task) Equal(config *app.Config) bool {
	token := t.lock.RLock()
	defer t.lock.RUnlock(token)

	return t.process.Config.Equal(config)
}

func (t *task) Config() *app.Config {
	token := t.lock.RLock()
	defer t.lock.RUnlock(token)

	return t.config.Clone()
}

func (t *task) Destroy() {
	t.Stop()

	t.lock.Lock()
	defer t.lock.Unlock()

	t.valid = false
	t.process = nil
	t.config = nil
	t.command = nil
	t.ffmpeg = nil
	t.parser = nil
	t.metadata = nil
}

func (t *task) Match(id, reference, owner, domain glob.Glob) bool {
	token := t.lock.RLock()
	defer t.lock.RUnlock(token)

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
	token := t.lock.RLock()
	defer t.lock.RUnlock(token)

	return t.process.Clone()
}

func (t *task) Order() string {
	token := t.lock.RLock()
	defer t.lock.RUnlock(token)

	return t.process.Order.String()
}
