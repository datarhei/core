package json

import (
	"slices"

	"github.com/datarhei/core/v16/restream/app"
)

type ProcessConfigIOCleanup struct {
	Pattern       string `json:"pattern"`
	MaxFiles      uint   `json:"max_files"`
	MaxFileAge    uint   `json:"max_file_age_seconds"`
	PurgeOnDelete bool   `json:"purge_on_delete"`
}

func (p *ProcessConfigIOCleanup) Marshal(a *app.ConfigIOCleanup) {
	p.Pattern = a.Pattern
	p.MaxFiles = a.MaxFiles
	p.MaxFileAge = a.MaxFileAge
	p.PurgeOnDelete = a.PurgeOnDelete
}

func (p *ProcessConfigIOCleanup) Unmarshal() app.ConfigIOCleanup {
	a := app.ConfigIOCleanup{
		Pattern:       p.Pattern,
		MaxFiles:      p.MaxFiles,
		MaxFileAge:    p.MaxFileAge,
		PurgeOnDelete: p.PurgeOnDelete,
	}

	return a
}

type ProcessConfigIO struct {
	ID      string                   `json:"id"`
	Address string                   `json:"address"`
	Options []string                 `json:"options"`
	Cleanup []ProcessConfigIOCleanup `json:"cleanup"`
}

func (p *ProcessConfigIO) Marshal(a *app.ConfigIO) {
	p.ID = a.ID
	p.Address = a.Address

	p.Options = make([]string, len(a.Options))
	copy(p.Options, a.Options)

	if len(a.Cleanup) != 0 {
		p.Cleanup = make([]ProcessConfigIOCleanup, len(a.Cleanup))
		for x, cleanup := range a.Cleanup {
			p.Cleanup[x].Marshal(&cleanup)
		}
	} else {
		p.Cleanup = nil
	}
}

func (p *ProcessConfigIO) Unmarshal() app.ConfigIO {
	a := app.ConfigIO{
		ID:      p.ID,
		Address: p.Address,
	}

	a.Options = make([]string, len(p.Options))
	copy(a.Options, p.Options)

	if len(p.Cleanup) != 0 {
		a.Cleanup = make([]app.ConfigIOCleanup, len(p.Cleanup))
		for x, cleanup := range p.Cleanup {
			a.Cleanup[x] = cleanup.Unmarshal()
		}
	}

	return a
}

type ProcessConfig struct {
	ID             string            `json:"id"`
	Reference      string            `json:"reference"`
	Owner          string            `json:"owner"`
	Domain         string            `json:"domain"`
	Binary         string            `json:"binary"`
	FFVersion      string            `json:"ffversion"`
	Input          []ProcessConfigIO `json:"input"`
	Output         []ProcessConfigIO `json:"output"`
	Options        []string          `json:"options"`
	Reconnect      bool              `json:"reconnect"`
	ReconnectDelay uint64            `json:"reconnect_delay_seconds"` // seconds
	Autostart      bool              `json:"autostart"`
	StaleTimeout   uint64            `json:"stale_timeout_seconds"` // seconds
	Timeout        uint64            `json:"timeout"`               // seconds
	Scheduler      string            `json:"scheduler"`             // crontab pattern or RFC3339 timestamp
	LogPatterns    []string          `json:"log_patterns"`          // will we interpreted as regualr expressions
	LimitLogRate   float64           `json:"limit_log_rate"`        // allow this number of log events per seconds, otherwise skip
	LimitCPU       float64           `json:"limit_cpu_usage"`       // percent
	LimitMemory    uint64            `json:"limit_memory_bytes"`    // bytes
	LimitGPU       ConfigLimitGPU    `json:"limit_gpu_usage"`       // GPU limits
	LimitWaitFor   uint64            `json:"limit_waitfor_seconds"` // seconds
}

type ConfigLimitGPU struct {
	Usage   float64 `json:"usage"`   // percent 0-100
	Encoder float64 `json:"encoder"` // percent 0-100
	Decoder float64 `json:"decoder"` // percent 0-100
	Memory  uint64  `json:"memory"`  // bytes
}

func (p *ConfigLimitGPU) Marshal(a app.ConfigLimitGPU) {
	p.Usage = a.Usage
	p.Encoder = a.Encoder
	p.Decoder = a.Decoder
	p.Memory = a.Memory
}

func (p *ConfigLimitGPU) Unmarshal() app.ConfigLimitGPU {
	a := app.ConfigLimitGPU{
		Usage:   p.Usage,
		Encoder: p.Encoder,
		Decoder: p.Decoder,
		Memory:  p.Memory,
	}

	return a
}

func (p *ProcessConfig) Marshal(a *app.Config) {
	p.ID = a.ID
	p.Reference = a.Reference
	p.Owner = a.Owner
	p.Domain = a.Domain
	p.Binary = a.Binary
	p.FFVersion = a.FFVersion
	p.Options = slices.Clone(a.Options)
	p.Reconnect = a.Reconnect
	p.ReconnectDelay = a.ReconnectDelay
	p.Autostart = a.Autostart
	p.StaleTimeout = a.StaleTimeout
	p.Timeout = a.Timeout
	p.Scheduler = a.Scheduler
	p.LogPatterns = slices.Clone(a.LogPatterns)
	p.LimitLogRate = a.LimitLogRate
	p.LimitCPU = a.LimitCPU
	p.LimitMemory = a.LimitMemory
	p.LimitGPU.Marshal(a.LimitGPU)
	p.LimitWaitFor = a.LimitWaitFor

	p.Input = make([]ProcessConfigIO, len(a.Input))
	for x, input := range a.Input {
		p.Input[x].Marshal(&input)
	}

	p.Output = make([]ProcessConfigIO, len(a.Output))
	for x, output := range a.Output {
		p.Output[x].Marshal(&output)
	}

	if p.LogPatterns == nil {
		p.LogPatterns = []string{}
	}
}

func (p *ProcessConfig) Unmarshal() *app.Config {
	a := &app.Config{
		ID:             p.ID,
		Reference:      p.Reference,
		Owner:          p.Owner,
		Domain:         p.Domain,
		Binary:         p.Binary,
		FFVersion:      p.FFVersion,
		Input:          []app.ConfigIO{},
		Output:         []app.ConfigIO{},
		Options:        slices.Clone(p.Options),
		Reconnect:      p.Reconnect,
		ReconnectDelay: p.ReconnectDelay,
		Autostart:      p.Autostart,
		StaleTimeout:   p.StaleTimeout,
		Timeout:        p.Timeout,
		Scheduler:      p.Scheduler,
		LogPatterns:    slices.Clone(p.LogPatterns),
		LimitLogRate:   p.LimitLogRate,
		LimitCPU:       p.LimitCPU,
		LimitMemory:    p.LimitMemory,
		LimitGPU:       p.LimitGPU.Unmarshal(),
		LimitWaitFor:   p.LimitWaitFor,
	}

	a.Input = make([]app.ConfigIO, len(p.Input))
	for x, input := range p.Input {
		a.Input[x] = input.Unmarshal()
	}

	a.Output = make([]app.ConfigIO, len(p.Output))
	for x, output := range p.Output {
		a.Output[x] = output.Unmarshal()
	}

	if a.LogPatterns == nil {
		a.LogPatterns = []string{}
	}

	return a
}

type Process struct {
	ID        string        `json:"id"`
	Owner     string        `json:"owner"`
	Domain    string        `json:"domain"`
	Reference string        `json:"reference"`
	Config    ProcessConfig `json:"config"`
	CreatedAt int64         `json:"created_at"`
	UpdatedAt int64         `json:"updated_at"`
	Order     string        `json:"order"`
}

func MarshalProcess(a *app.Process) Process {
	p := Process{
		ID:        a.ID,
		Owner:     a.Owner,
		Domain:    a.Domain,
		Reference: a.Reference,
		Config:    ProcessConfig{},
		CreatedAt: a.CreatedAt,
		UpdatedAt: a.UpdatedAt,
		Order:     a.Order.String(),
	}

	p.Config.Marshal(a.Config)

	return p
}

func UnmarshalProcess(p Process) *app.Process {
	a := &app.Process{
		ID:        p.ID,
		Owner:     p.Owner,
		Domain:    p.Domain,
		Reference: p.Reference,
		Config:    &app.Config{},
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
		Order:     app.NewOrder(p.Order),
	}

	a.Config = p.Config.Unmarshal()

	return a
}

type Domain struct {
	Process  map[string]Process                `json:"process"`
	Metadata map[string]map[string]interface{} `json:"metadata"`
}

type Data struct {
	Version uint64 `json:"version"`

	Process  map[string]Process `json:"process"`
	Domain   map[string]Domain  `json:"domain"`
	Metadata struct {
		System  map[string]interface{}            `json:"system"`
		Process map[string]map[string]interface{} `json:"process"`
	} `json:"metadata"`
}

var version uint64 = 4

func NewData() Data {
	c := Data{
		Version: version,
	}

	c.Process = make(map[string]Process)
	c.Domain = make(map[string]Domain)
	c.Metadata.System = make(map[string]interface{})
	c.Metadata.Process = make(map[string]map[string]interface{})

	return c
}

func (c *Data) IsEmpty() bool {
	if len(c.Process) != 0 {
		return false
	}

	if len(c.Domain) != 0 {
		return false
	}

	if len(c.Metadata.Process) != 0 {
		return false
	}

	if len(c.Metadata.System) != 0 {
		return false
	}

	return true
}
