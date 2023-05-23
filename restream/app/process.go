package app

import (
	"github.com/datarhei/core/v16/process"
)

type ConfigIOCleanup struct {
	Pattern       string
	MaxFiles      uint
	MaxFileAge    uint
	PurgeOnDelete bool
}

type ConfigIO struct {
	ID      string
	Address string
	Options []string
	Cleanup []ConfigIOCleanup
}

func (io ConfigIO) Clone() ConfigIO {
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
	StaleTimeout   uint64  // seconds
	LimitCPU       float64 // percent
	LimitMemory    uint64  // bytes
	LimitWaitFor   uint64  // seconds
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
	Command   []string      // ffmpeg command line parameters
}
