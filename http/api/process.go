package api

import (
	"strconv"

	"github.com/datarhei/core/v16/cluster/store"
	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/restream/app"

	"github.com/lithammer/shortuuid/v4"
)

type ProcessID struct {
	ID     string `json:"id"`
	Domain string `json:"domain"`
}

// Process represents all information on a process
type Process struct {
	ID        string         `json:"id" jsonschema:"minLength=1"`
	Owner     string         `json:"owner"`
	Domain    string         `json:"domain"`
	Type      string         `json:"type" jsonschema:"enum=ffmpeg"`
	Reference string         `json:"reference"`
	CoreID    string         `json:"core_id"`
	CreatedAt int64          `json:"created_at" format:"int64"`
	UpdatedAt int64          `json:"updated_at" format:"int64"`
	Config    *ProcessConfig `json:"config,omitempty"`
	State     *ProcessState  `json:"state,omitempty"`
	Report    *ProcessReport `json:"report,omitempty"`
	Metadata  Metadata       `json:"metadata,omitempty"`
}

func (p *Process) Unmarshal(ap *app.Process, ac *app.Config, as *app.State, ar *app.Report, am interface{}) {
	p.ID = ap.ID
	p.Owner = ap.Owner
	p.Domain = ap.Domain
	p.Reference = ap.Reference
	p.Type = "ffmpeg"
	p.CreatedAt = ap.CreatedAt
	p.UpdatedAt = ap.UpdatedAt

	p.Config = nil
	if ac != nil {
		p.Config = &ProcessConfig{}
		p.Config.Unmarshal(ap.Config, nil)
	}

	p.State = nil
	if as != nil {
		p.State = &ProcessState{}
		p.State.Unmarshal(as)
	}

	p.Report = nil
	if ar != nil {
		p.Report = &ProcessReport{}
		p.Report.Unmarshal(ar)
	}

	p.Metadata = nil
	if am != nil {
		p.Metadata = NewMetadata(am)
	}
}

func (p *Process) UnmarshalStore(s store.Process, config, state, report, metadata bool) {
	p.ID = s.Config.ID
	p.Owner = s.Config.Owner
	p.Domain = s.Config.Domain
	p.Type = "ffmpeg"
	p.Reference = s.Config.Reference
	p.CreatedAt = s.CreatedAt.Unix()
	p.UpdatedAt = s.UpdatedAt.Unix()

	p.Metadata = nil
	if metadata {
		p.Metadata = s.Metadata
	}

	p.Config = nil
	if config {
		config := &ProcessConfig{}
		config.Unmarshal(s.Config, s.Metadata)

		p.Config = config
	}

	p.State = nil
	if state {
		p.State = &ProcessState{
			Order:   s.Order,
			LastLog: s.Error,
			Resources: ProcessUsage{
				CPU: ProcessUsageCPU{
					NCPU:  json.ToNumber(1),
					Limit: json.ToNumber(s.Config.LimitCPU),
				},
				Memory: ProcessUsageMemory{
					Limit: s.Config.LimitMemory,
				},
			},
			Command: []string{},
			Progress: &Progress{
				Input:  []ProgressIO{},
				Output: []ProgressIO{},
				Mapping: StreamMapping{
					Graphs:  []GraphElement{},
					Mapping: []GraphMapping{},
				},
			},
		}

		if len(s.Error) != 0 {
			p.State.State = "failed"
		} else {
			p.State.State = "deploying"
		}
	}

	if report {
		p.Report = &ProcessReport{
			ProcessReportEntry: ProcessReportEntry{
				CreatedAt: s.CreatedAt.Unix(),
				Prelude:   []string{},
				Log:       [][2]string{},
				Matches:   []string{},
			},
		}

		if len(s.Error) != 0 {
			p.Report.Prelude = []string{s.Error}
			p.Report.Log = [][2]string{
				{strconv.FormatInt(s.CreatedAt.Unix(), 10), s.Error},
			}
			//process.Report.ExitedAt = p.CreatedAt.Unix()
			//process.Report.ExitState = "failed"
		}
	}
}

// ProcessConfigIO represents an input or output of an ffmpeg process config
type ProcessConfigIO struct {
	ID      string                   `json:"id"`
	Address string                   `json:"address" validate:"required" jsonschema:"minLength=1"`
	Options []string                 `json:"options"`
	Cleanup []ProcessConfigIOCleanup `json:"cleanup,omitempty"`
}

type ProcessConfigIOCleanup struct {
	Pattern       string `json:"pattern" validate:"required"`
	MaxFiles      uint   `json:"max_files" format:"uint"`
	MaxFileAge    uint   `json:"max_file_age_seconds" format:"uint"`
	PurgeOnDelete bool   `json:"purge_on_delete"`
}

type ProcessConfigLimits struct {
	CPU        float64 `json:"cpu_usage" jsonschema:"minimum=0"`                         // percent 0-100*ncpu
	Memory     uint64  `json:"memory_mbytes" jsonschema:"minimum=0" format:"uint64"`     // megabytes
	GPUUsage   float64 `json:"gpu_usage" jsonschema:"minimum=0"`                         // percent 0-100
	GPUEncoder float64 `json:"gpu_encoder" jsonschema:"minimum=0"`                       // percent 0-100
	GPUDecoder float64 `json:"gpu_decoder" jsonschema:"minimum=0"`                       // percent 0-100
	GPUMemory  uint64  `json:"gpu_memory_mbytes" jsonschema:"minimum=0" format:"uint64"` // megabytes
	WaitFor    uint64  `json:"waitfor_seconds" jsonschema:"minimum=0" format:"uint64"`   // seconds
}

// ProcessConfig represents the configuration of an ffmpeg process
type ProcessConfig struct {
	ID             string                 `json:"id"`
	Owner          string                 `json:"owner"`
	Domain         string                 `json:"domain"`
	Type           string                 `json:"type" validate:"oneof='ffmpeg' ''" jsonschema:"enum=ffmpeg,enum="`
	Reference      string                 `json:"reference"`
	Input          []ProcessConfigIO      `json:"input" validate:"required"`
	Output         []ProcessConfigIO      `json:"output" validate:"required"`
	Options        []string               `json:"options"`
	Reconnect      bool                   `json:"reconnect"`
	ReconnectDelay uint64                 `json:"reconnect_delay_seconds" format:"uint64"`
	Autostart      bool                   `json:"autostart"`
	StaleTimeout   uint64                 `json:"stale_timeout_seconds" format:"uint64"`
	Timeout        uint64                 `json:"runtime_duration_seconds" format:"uint64"`
	Scheduler      string                 `json:"scheduler"`
	LogPatterns    []string               `json:"log_patterns"`
	Limits         ProcessConfigLimits    `json:"limits"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// Marshal converts a process config in API representation to a core process config and metadata
func (cfg *ProcessConfig) Marshal() (*app.Config, map[string]interface{}) {
	p := &app.Config{
		ID:             cfg.ID,
		Owner:          cfg.Owner,
		Domain:         cfg.Domain,
		Reference:      cfg.Reference,
		Options:        cfg.Options,
		Reconnect:      cfg.Reconnect,
		ReconnectDelay: cfg.ReconnectDelay,
		Autostart:      cfg.Autostart,
		StaleTimeout:   cfg.StaleTimeout,
		Timeout:        cfg.Timeout,
		Scheduler:      cfg.Scheduler,
		LimitCPU:       cfg.Limits.CPU,
		LimitMemory:    cfg.Limits.Memory * 1024 * 1024,
		LimitGPU: app.ConfigLimitGPU{
			Usage:   cfg.Limits.GPUUsage,
			Encoder: cfg.Limits.GPUEncoder,
			Decoder: cfg.Limits.GPUDecoder,
			Memory:  cfg.Limits.GPUMemory * 1024 * 1024,
		},
		LimitWaitFor: cfg.Limits.WaitFor,
	}

	cfg.generateInputOutputIDs(cfg.Input)

	for _, x := range cfg.Input {
		p.Input = append(p.Input, app.ConfigIO{
			ID:      x.ID,
			Address: x.Address,
			Options: x.Options,
		})
	}

	cfg.generateInputOutputIDs(cfg.Output)

	for _, x := range cfg.Output {
		output := app.ConfigIO{
			ID:      x.ID,
			Address: x.Address,
			Options: x.Options,
		}

		for _, c := range x.Cleanup {
			output.Cleanup = append(output.Cleanup, app.ConfigIOCleanup{
				Pattern:       c.Pattern,
				MaxFiles:      c.MaxFiles,
				MaxFileAge:    c.MaxFileAge,
				PurgeOnDelete: c.PurgeOnDelete,
			})
		}

		p.Output = append(p.Output, output)

	}

	p.LogPatterns = make([]string, len(cfg.LogPatterns))
	copy(p.LogPatterns, cfg.LogPatterns)

	return p, cfg.Metadata
}

func (cfg *ProcessConfig) generateInputOutputIDs(ioconfig []ProcessConfigIO) {
	ids := map[string]struct{}{}

	for _, io := range ioconfig {
		if len(io.ID) == 0 {
			continue
		}

		ids[io.ID] = struct{}{}
	}

	for i, io := range ioconfig {
		if len(io.ID) != 0 {
			continue
		}

		for {
			id := shortuuid.New()
			if _, ok := ids[id]; !ok {
				ioconfig[i].ID = id
				break
			}
		}
	}
}

// Unmarshal converts a core process config to a process config in API representation
func (cfg *ProcessConfig) Unmarshal(c *app.Config, metadata map[string]interface{}) {
	if c == nil {
		return
	}

	cfg.ID = c.ID
	cfg.Owner = c.Owner
	cfg.Domain = c.Domain
	cfg.Reference = c.Reference
	cfg.Type = "ffmpeg"
	cfg.Reconnect = c.Reconnect
	cfg.ReconnectDelay = c.ReconnectDelay
	cfg.Autostart = c.Autostart
	cfg.StaleTimeout = c.StaleTimeout
	cfg.Timeout = c.Timeout
	cfg.Scheduler = c.Scheduler
	cfg.Limits.CPU = c.LimitCPU
	cfg.Limits.Memory = c.LimitMemory / 1024 / 1024
	cfg.Limits.GPUUsage = c.LimitGPU.Usage
	cfg.Limits.GPUEncoder = c.LimitGPU.Encoder
	cfg.Limits.GPUDecoder = c.LimitGPU.Decoder
	cfg.Limits.GPUMemory = c.LimitGPU.Memory / 1024 / 1024
	cfg.Limits.WaitFor = c.LimitWaitFor

	cfg.Options = make([]string, len(c.Options))
	copy(cfg.Options, c.Options)

	for _, x := range c.Input {
		io := ProcessConfigIO{
			ID:      x.ID,
			Address: x.Address,
		}

		io.Options = make([]string, len(x.Options))
		copy(io.Options, x.Options)

		cfg.Input = append(cfg.Input, io)
	}

	for _, x := range c.Output {
		io := ProcessConfigIO{
			ID:      x.ID,
			Address: x.Address,
		}

		io.Options = make([]string, len(x.Options))
		copy(io.Options, x.Options)

		for _, c := range x.Cleanup {
			io.Cleanup = append(io.Cleanup, ProcessConfigIOCleanup{
				Pattern:       c.Pattern,
				MaxFiles:      c.MaxFiles,
				MaxFileAge:    c.MaxFileAge,
				PurgeOnDelete: c.PurgeOnDelete,
			})
		}

		cfg.Output = append(cfg.Output, io)
	}

	cfg.LogPatterns = make([]string, len(c.LogPatterns))
	copy(cfg.LogPatterns, c.LogPatterns)

	cfg.Metadata = metadata
}

func (p *ProcessConfig) ProcessID() app.ProcessID {
	return app.ProcessID{
		ID:     p.ID,
		Domain: p.Domain,
	}
}

// ProcessState represents the current state of an ffmpeg process
type ProcessState struct {
	Order     string       `json:"order" jsonschema:"enum=start,enum=stop"`
	State     string       `json:"exec" jsonschema:"enum=finished,enum=starting,enum=running,enum=finishing,enum=killed,enum=failed"`
	Runtime   int64        `json:"runtime_seconds" jsonschema:"minimum=0" format:"int64"`
	Reconnect int64        `json:"reconnect_seconds" format:"int64"`
	LastLog   string       `json:"last_logline"`
	Progress  *Progress    `json:"progress"`
	Memory    uint64       `json:"memory_bytes" format:"uint64"`                            // deprecated, use Resources.CPU.Current
	CPU       json.Number  `json:"cpu_usage" swaggertype:"number" jsonschema:"type=number"` // deprecated, use Resources.Memory.Current
	LimitMode string       `json:"limit_mode"`
	Resources ProcessUsage `json:"resources"`
	Command   []string     `json:"command"`
}

// Unmarshal converts a core ffmpeg process state to a state in API representation
func (s *ProcessState) Unmarshal(state *app.State) {
	if state == nil {
		return
	}

	s.Order = state.Order
	s.State = state.State
	s.Runtime = int64(state.Duration)
	s.Reconnect = int64(state.Reconnect)
	s.LastLog = state.LastLog
	s.Progress = &Progress{}
	s.Memory = state.Memory
	s.CPU = json.ToNumber(state.CPU)
	s.LimitMode = state.LimitMode
	s.Resources.Unmarshal(&state.Resources)
	s.Command = state.Command

	s.Progress.Unmarshal(&state.Progress)
}

type ProcessUsageCPU struct {
	NCPU         json.Number `json:"ncpu" swaggertype:"number" jsonschema:"type=number"`
	Current      json.Number `json:"cur" swaggertype:"number" jsonschema:"type=number"`
	Average      json.Number `json:"avg" swaggertype:"number" jsonschema:"type=number"`
	Max          json.Number `json:"max" swaggertype:"number" jsonschema:"type=number"`
	Limit        json.Number `json:"limit" swaggertype:"number" jsonschema:"type=number"`
	IsThrottling bool        `json:"throttling"`
}

func (p *ProcessUsageCPU) Unmarshal(pp *app.ProcessUsageCPU) {
	p.NCPU = json.ToNumber(pp.NCPU)
	p.Current = json.ToNumber(pp.Current)
	p.Average = json.ToNumber(pp.Average)
	p.Max = json.ToNumber(pp.Max)
	p.Limit = json.ToNumber(pp.Limit)
	p.IsThrottling = pp.IsThrottling
}

func (p *ProcessUsageCPU) Marshal() app.ProcessUsageCPU {
	pp := app.ProcessUsageCPU{
		IsThrottling: p.IsThrottling,
	}

	if x, err := p.NCPU.Float64(); err == nil {
		pp.NCPU = x
	}

	if x, err := p.Current.Float64(); err == nil {
		pp.Current = x
	}

	if x, err := p.Average.Float64(); err == nil {
		pp.Average = x
	}

	if x, err := p.Max.Float64(); err == nil {
		pp.Max = x
	}

	if x, err := p.Limit.Float64(); err == nil {
		pp.Limit = x
	}

	return pp
}

type ProcessUsageMemory struct {
	Current uint64      `json:"cur" format:"uint64"`
	Average json.Number `json:"avg" swaggertype:"number" jsonschema:"type=number"`
	Max     uint64      `json:"max" format:"uint64"`
	Limit   uint64      `json:"limit" format:"uint64"`
}

func (p *ProcessUsageMemory) Unmarshal(pp *app.ProcessUsageMemory) {
	p.Current = pp.Current
	p.Average = json.ToNumber(float64(pp.Average))
	p.Max = pp.Max
	p.Limit = pp.Limit
}

func (p *ProcessUsageMemory) Marshal() app.ProcessUsageMemory {
	pp := app.ProcessUsageMemory{
		Current: p.Current,
		Max:     p.Max,
		Limit:   p.Limit,
	}

	if x, err := p.Average.Float64(); err == nil {
		pp.Average = uint64(x)
	}

	return pp
}

type ProcessUsageGPUMemory struct {
	Current uint64      `json:"cur" format:"uint64"`
	Average json.Number `json:"avg" swaggertype:"number" jsonschema:"type=number"`
	Max     uint64      `json:"max" format:"uint64"`
	Limit   uint64      `json:"limit" format:"uint64"`
}

func (p *ProcessUsageGPUMemory) Unmarshal(pp *app.ProcessUsageGPUMemory) {
	p.Current = pp.Current
	p.Average = json.ToNumber(float64(pp.Average))
	p.Max = pp.Max
	p.Limit = pp.Limit
}

func (p *ProcessUsageGPUMemory) Marshal() app.ProcessUsageGPUMemory {
	pp := app.ProcessUsageGPUMemory{
		Current: p.Current,
		Max:     p.Max,
		Limit:   p.Limit,
	}

	if x, err := p.Average.Float64(); err == nil {
		pp.Average = uint64(x)
	}

	return pp
}

type ProcessUsageGPUUsage struct {
	Current json.Number `json:"cur" swaggertype:"number" jsonschema:"type=number"`
	Average json.Number `json:"avg" swaggertype:"number" jsonschema:"type=number"`
	Max     json.Number `json:"max" swaggertype:"number" jsonschema:"type=number"`
	Limit   json.Number `json:"limit" swaggertype:"number" jsonschema:"type=number"`
}

func (p *ProcessUsageGPUUsage) Unmarshal(pp *app.ProcessUsageGPUUsage) {
	p.Current = json.ToNumber(pp.Current)
	p.Average = json.ToNumber(pp.Average)
	p.Max = json.ToNumber(pp.Max)
	p.Limit = json.ToNumber(pp.Limit)
}

func (p *ProcessUsageGPUUsage) Marshal() app.ProcessUsageGPUUsage {
	pp := app.ProcessUsageGPUUsage{}

	if x, err := p.Current.Float64(); err == nil {
		pp.Current = x
	}

	if x, err := p.Average.Float64(); err == nil {
		pp.Average = x
	}

	if x, err := p.Max.Float64(); err == nil {
		pp.Max = x
	}

	if x, err := p.Limit.Float64(); err == nil {
		pp.Limit = x
	}

	return pp
}

type ProcessUsageGPU struct {
	Index   int                   `json:"index"`
	Memory  ProcessUsageGPUMemory `json:"memory_bytes"`
	Usage   ProcessUsageGPUUsage  `json:"usage"`
	Encoder ProcessUsageGPUUsage  `json:"encoder"`
	Decoder ProcessUsageGPUUsage  `json:"decoder"`
}

func (p *ProcessUsageGPU) Unmarshal(pp *app.ProcessUsageGPU) {
	p.Index = pp.Index
	p.Memory.Unmarshal(&pp.Memory)
	p.Usage.Unmarshal(&pp.Usage)
	p.Encoder.Unmarshal(&pp.Encoder)
	p.Decoder.Unmarshal(&pp.Decoder)
}

func (p *ProcessUsageGPU) Marshal() app.ProcessUsageGPU {
	pp := app.ProcessUsageGPU{
		Index:   p.Index,
		Memory:  p.Memory.Marshal(),
		Usage:   p.Usage.Marshal(),
		Encoder: p.Encoder.Marshal(),
		Decoder: p.Decoder.Marshal(),
	}

	return pp
}

type ProcessUsage struct {
	CPU    ProcessUsageCPU    `json:"cpu_usage"`
	Memory ProcessUsageMemory `json:"memory_bytes"`
	GPU    ProcessUsageGPU    `json:"gpu"`
}

func (p *ProcessUsage) Unmarshal(pp *app.ProcessUsage) {
	p.CPU.Unmarshal(&pp.CPU)
	p.Memory.Unmarshal(&pp.Memory)
	p.GPU.Unmarshal(&pp.GPU)
}

func (p *ProcessUsage) Marshal() app.ProcessUsage {
	pp := app.ProcessUsage{
		CPU:    p.CPU.Marshal(),
		Memory: p.Memory.Marshal(),
		GPU:    p.GPU.Marshal(),
	}

	return pp
}
