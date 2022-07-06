package app

import (
	"github.com/datarhei/core/v16/process"
	"github.com/datarhei/core/v16/restream/replace"
)

type ConfigIOCleanup struct {
	Pattern       string `json:"pattern"`
	MaxFiles      uint   `json:"max_files"`
	MaxFileAge    uint   `json:"max_file_age_seconds"`
	PurgeOnDelete bool   `json:"purge_on_delete"`
}

type ConfigIO struct {
	ID      string            `json:"id"`
	Address string            `json:"address"`
	Options []string          `json:"options"`
	Cleanup []ConfigIOCleanup `json:"cleanup"`
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
	ID             string     `json:"id"`
	Reference      string     `json:"reference"`
	Input          []ConfigIO `json:"input"`
	Output         []ConfigIO `json:"output"`
	Options        []string   `json:"options"`
	Reconnect      bool       `json:"reconnect"`
	ReconnectDelay uint64     `json:"reconnect_delay_seconds"` // seconds
	Autostart      bool       `json:"autostart"`
	StaleTimeout   uint64     `json:"stale_timeout_seconds"` // seconds
	LimitCPU       float64    `json:"limit_cpu_usage"`       // percent
	LimitMemory    uint64     `json:"limit_memory_bytes"`    // bytes
	LimitWaitFor   uint64     `json:"limit_waitfor_seconds"` // seconds
}

func (config *Config) Clone() *Config {
	clone := &Config{
		ID:             config.ID,
		Reference:      config.Reference,
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

// ReplacePlaceholders replaces all placeholders in the config. The config
// will be modified in place.
func (config *Config) ResolvePlaceholders(r replace.Replacer) {
	for i, option := range config.Options {
		// Replace any known placeholders
		option = r.Replace(option, "diskfs", "")

		config.Options[i] = option
	}

	// Resolving the given inputs
	for i, input := range config.Input {
		// Replace any known placeholders
		input.ID = r.Replace(input.ID, "processid", config.ID)
		input.ID = r.Replace(input.ID, "reference", config.Reference)
		input.Address = r.Replace(input.Address, "inputid", input.ID)
		input.Address = r.Replace(input.Address, "processid", config.ID)
		input.Address = r.Replace(input.Address, "reference", config.Reference)
		input.Address = r.Replace(input.Address, "diskfs", "")
		input.Address = r.Replace(input.Address, "memfs", "")
		input.Address = r.Replace(input.Address, "rtmp", "")
		input.Address = r.Replace(input.Address, "srt", "")

		for j, option := range input.Options {
			// Replace any known placeholders
			option = r.Replace(option, "inputid", input.ID)
			option = r.Replace(option, "processid", config.ID)
			option = r.Replace(option, "reference", config.Reference)
			option = r.Replace(option, "diskfs", "")
			option = r.Replace(option, "memfs", "")

			input.Options[j] = option
		}

		config.Input[i] = input
	}

	// Resolving the given outputs
	for i, output := range config.Output {
		// Replace any known placeholders
		output.ID = r.Replace(output.ID, "processid", config.ID)
		output.Address = r.Replace(output.Address, "outputid", output.ID)
		output.Address = r.Replace(output.Address, "processid", config.ID)
		output.Address = r.Replace(output.Address, "reference", config.Reference)
		output.Address = r.Replace(output.Address, "diskfs", "")
		output.Address = r.Replace(output.Address, "memfs", "")
		output.Address = r.Replace(output.Address, "rtmp", "")
		output.Address = r.Replace(output.Address, "srt", "")

		for j, option := range output.Options {
			// Replace any known placeholders
			option = r.Replace(option, "outputid", output.ID)
			option = r.Replace(option, "processid", config.ID)
			option = r.Replace(option, "reference", config.Reference)
			option = r.Replace(option, "diskfs", "")
			option = r.Replace(option, "memfs", "")

			output.Options[j] = option
		}

		for j, cleanup := range output.Cleanup {
			// Replace any known placeholders
			cleanup.Pattern = r.Replace(cleanup.Pattern, "outputid", output.ID)
			cleanup.Pattern = r.Replace(cleanup.Pattern, "processid", config.ID)
			cleanup.Pattern = r.Replace(cleanup.Pattern, "reference", config.Reference)

			output.Cleanup[j] = cleanup
		}

		config.Output[i] = output
	}
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
	ID        string  `json:"id"`
	Reference string  `json:"reference"`
	Config    *Config `json:"config"`
	CreatedAt int64   `json:"created_at"`
	Order     string  `json:"order"`
}

func (process *Process) Clone() *Process {
	clone := &Process{
		ID:        process.ID,
		Reference: process.Reference,
		Config:    process.Config.Clone(),
		CreatedAt: process.CreatedAt,
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
