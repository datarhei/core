package app

import (
	"bytes"
	"crypto/md5"
	"strconv"
	"strings"

	"github.com/datarhei/core/v16/process"
)

type ConfigIOCleanup struct {
	Pattern       string
	MaxFiles      uint
	MaxFileAge    uint
	PurgeOnDelete bool
}

func (c *ConfigIOCleanup) HashString() string {
	b := strings.Builder{}

	b.WriteString(c.Pattern)
	b.WriteString(strconv.FormatUint(uint64(c.MaxFiles), 10))
	b.WriteString(strconv.FormatUint(uint64(c.MaxFileAge), 10))
	b.WriteString(strconv.FormatBool(c.PurgeOnDelete))

	return b.String()
}

type ConfigIO struct {
	ID      string
	Address string
	Options []string
	Cleanup []ConfigIOCleanup
}

func (io *ConfigIO) Clone() ConfigIO {
	clone := ConfigIO{
		ID:      io.ID,
		Address: io.Address,
	}

	clone.Options = make([]string, len(io.Options))
	copy(clone.Options, io.Options)

	clone.Cleanup = make([]ConfigIOCleanup, len(io.Cleanup))
	copy(clone.Cleanup, io.Cleanup)

	return clone
}

func (io *ConfigIO) HashString() string {
	b := strings.Builder{}

	b.WriteString(io.ID)
	b.WriteString(io.Address)
	b.WriteString(strings.Join(io.Options, ","))

	for _, x := range io.Cleanup {
		b.WriteString(x.HashString())
	}

	return b.String()
}

type Config struct {
	ID             string
	Reference      string
	Owner          string
	Domain         string
	FFVersion      string
	Input          []ConfigIO
	Output         []ConfigIO
	Options        []string
	Reconnect      bool
	ReconnectDelay uint64 // seconds
	Autostart      bool
	StaleTimeout   uint64   // seconds
	Timeout        uint64   // seconds
	Scheduler      string   // crontab pattern or RFC3339 timestamp
	LogPatterns    []string // will we interpreted as regular expressions
	LimitCPU       float64  // percent
	LimitMemory    uint64   // bytes
	LimitWaitFor   uint64   // seconds
}

func (config *Config) Clone() *Config {
	clone := &Config{
		ID:             config.ID,
		Reference:      config.Reference,
		Owner:          config.Owner,
		Domain:         config.Domain,
		FFVersion:      config.FFVersion,
		Reconnect:      config.Reconnect,
		ReconnectDelay: config.ReconnectDelay,
		Autostart:      config.Autostart,
		StaleTimeout:   config.StaleTimeout,
		Timeout:        config.Timeout,
		Scheduler:      config.Scheduler,
		LimitCPU:       config.LimitCPU,
		LimitMemory:    config.LimitMemory,
		LimitWaitFor:   config.LimitWaitFor,
	}

	clone.Input = make([]ConfigIO, len(config.Input))
	for i, io := range config.Input {
		clone.Input[i] = io.Clone()
	}

	clone.Output = make([]ConfigIO, len(config.Output))
	for i, io := range config.Output {
		clone.Output[i] = io.Clone()
	}

	clone.Options = make([]string, len(config.Options))
	copy(clone.Options, config.Options)

	clone.LogPatterns = make([]string, len(config.LogPatterns))
	copy(clone.LogPatterns, config.LogPatterns)

	return clone
}

// CreateCommand created the FFmpeg command from this config.
func (config *Config) CreateCommand() []string {
	var command []string

	// Copy global options
	command = append(command, config.Options...)

	for _, input := range config.Input {
		// Add the resolved input to the process command
		command = append(command, input.Options...)
		command = append(command, "-i", input.Address)
	}

	for _, output := range config.Output {
		// Add the resolved output to the process command
		command = append(command, output.Options...)
		command = append(command, output.Address)
	}

	return command
}

func (config *Config) Hash() []byte {
	b := bytes.Buffer{}

	b.WriteString(config.ID)
	b.WriteString(config.Reference)
	b.WriteString(config.Owner)
	b.WriteString(config.Domain)
	b.WriteString(config.Scheduler)
	b.WriteString(strings.Join(config.Options, ","))
	b.WriteString(strings.Join(config.LogPatterns, ","))
	b.WriteString(strconv.FormatBool(config.Reconnect))
	b.WriteString(strconv.FormatBool(config.Autostart))
	b.WriteString(strconv.FormatUint(config.ReconnectDelay, 10))
	b.WriteString(strconv.FormatUint(config.StaleTimeout, 10))
	b.WriteString(strconv.FormatUint(config.Timeout, 10))
	b.WriteString(strconv.FormatUint(config.LimitMemory, 10))
	b.WriteString(strconv.FormatUint(config.LimitWaitFor, 10))
	b.WriteString(strconv.FormatFloat(config.LimitCPU, 'f', -1, 64))

	for _, x := range config.Input {
		b.WriteString(x.HashString())
	}

	for _, x := range config.Output {
		b.WriteString(x.HashString())
	}

	sum := md5.Sum(b.Bytes())

	return sum[:]
}

func (c *Config) Equal(a *Config) bool {
	return bytes.Equal(c.Hash(), a.Hash())
}

func (c *Config) ProcessID() ProcessID {
	return ProcessID{
		ID:     c.ID,
		Domain: c.Domain,
	}
}

type Process struct {
	ID        string
	Owner     string
	Domain    string
	Reference string
	Config    *Config
	CreatedAt int64
	UpdatedAt int64
	Order     string
}

func (process *Process) Clone() *Process {
	clone := &Process{
		ID:        process.ID,
		Owner:     process.Owner,
		Domain:    process.Domain,
		Reference: process.Reference,
		Config:    process.Config.Clone(),
		CreatedAt: process.CreatedAt,
		UpdatedAt: process.UpdatedAt,
		Order:     process.Order,
	}

	return clone
}

func (process *Process) ProcessID() ProcessID {
	return ProcessID{
		ID:     process.ID,
		Domain: process.Domain,
	}
}

type ProcessStates struct {
	Finished  uint64
	Starting  uint64
	Running   uint64
	Finishing uint64
	Failed    uint64
	Killed    uint64
}

func (p *ProcessStates) Marshal(s process.States) {
	p.Finished = s.Finished
	p.Starting = s.Starting
	p.Running = s.Running
	p.Finishing = s.Finishing
	p.Failed = s.Failed
	p.Killed = s.Killed
}

type State struct {
	Order     string        // Current order, e.g. "start", "stop"
	State     string        // Current state, e.g. "running"
	States    ProcessStates // Cumulated process states
	Time      int64         // Unix timestamp of last status change
	Duration  float64       // Runtime in seconds since last status change
	Reconnect float64       // Seconds until next reconnect, negative if not reconnecting
	LastLog   string        // Last recorded line from the process
	Progress  Progress      // Progress data of the process
	Memory    uint64        // Current memory consumption in bytes
	CPU       float64       // Current CPU consumption in percent
	LimitMode string        // How the process is limited (hard or soft)
	Resources ProcessUsage  // Current resource usage, include CPU and memory consumption
	Command   []string      // ffmpeg command line parameters
}

type ProcessUsageCPU struct {
	NCPU         float64 // Number of logical CPUs
	Current      float64 // percent 0-100*ncpu
	Average      float64 // percent 0-100*ncpu
	Max          float64 // percent 0-100*ncpu
	Limit        float64 // percent 0-100*ncpu
	IsThrottling bool
}

type ProcessUsageMemory struct {
	Current uint64  // bytes
	Average float64 // bytes
	Max     uint64  // bytes
	Limit   uint64  // bytes
}

type ProcessUsage struct {
	CPU    ProcessUsageCPU
	Memory ProcessUsageMemory
}

type ProcessID struct {
	ID     string
	Domain string
}

func NewProcessID(id, domain string) ProcessID {
	return ProcessID{
		ID:     id,
		Domain: domain,
	}
}

func ParseProcessID(pid string) ProcessID {
	p := ProcessID{}

	p.Parse(pid)

	return p
}

func (p ProcessID) String() string {
	return p.ID + "@" + p.Domain
}

func (p ProcessID) Equal(b ProcessID) bool {
	if p.ID == b.ID && p.Domain == b.Domain {
		return true
	}

	return false
}

func (p *ProcessID) Parse(pid string) {
	i := strings.LastIndex(pid, "@")
	if i == -1 {
		p.ID = pid
		p.Domain = ""
	}

	p.ID = pid[:i]
	p.Domain = pid[i+1:]
}

func (p ProcessID) MarshalText() ([]byte, error) {
	return []byte(p.String()), nil
}

func (p *ProcessID) UnmarshalText(text []byte) error {
	p.Parse(string(text))
	return nil
}
