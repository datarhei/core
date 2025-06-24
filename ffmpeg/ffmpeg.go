package ffmpeg

import (
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/datarhei/core/v16/ffmpeg/parse"
	"github.com/datarhei/core/v16/ffmpeg/probe"
	"github.com/datarhei/core/v16/ffmpeg/skills"
	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/net"
	"github.com/datarhei/core/v16/process"
	"github.com/datarhei/core/v16/resources"
	"github.com/datarhei/core/v16/session"
)

type FFmpeg interface {
	New(config ProcessConfig) (process.Process, error)
	NewProcessParser(logger log.Logger, id, reference string, logpatterns []string) parse.Parser
	NewProbeParser(logger log.Logger) probe.Parser
	ValidateInputAddress(address string) bool
	ValidateOutputAddress(address string) bool
	Skills() skills.Skills
	ReloadSkills() error
	GetPort() (int, error)
	PutPort(port int)
	States() process.States
}

type ProcessConfig struct {
	Binary          string                           // Override the default binary
	Reconnect       bool                             // Whether to reconnect
	ReconnectDelay  time.Duration                    // Duration until next reconnect
	StaleTimeout    time.Duration                    // Duration to wait until killing the process if there is no progress in the process
	Timeout         time.Duration                    // Duration to wait until killing the process
	LimitCPU        float64                          // Kill the process if the CPU usage in percent is above this value.
	LimitMemory     uint64                           // Kill the process if the memory consumption in bytes is above this value.
	LimitGPUUsage   float64                          // Kill the process id the GPU usage (general) in percent is above this value.
	LimitGPUEncoder float64                          // Kill the process id the GPU usage (encoder) in percent is above this value.
	LimitGPUDecoder float64                          // Kill the process id the GPU usage (decoder) in percent is above this value.
	LimitGPUMemory  uint64                           // Kill the process if the GPU memory consumption in bytes is above this value.
	LimitDuration   time.Duration                    // Kill the process if the limits are exceeded for this duration.
	LimitMode       string                           // How to limit the process, "hard" or "soft"
	Scheduler       string                           // A scheduler for starting the process, either a concrete date (RFC3339) or in crontab syntax
	Args            []string                         // Arguments for the process
	Parser          process.Parser                   // Parser for the process output
	Logger          log.Logger                       // Logger
	OnBeforeStart   func([]string) ([]string, error) // Callback which is called before the process will be started. The string slice is the list of arguments which can be modified. If error is non-nil, the start will be refused.
	OnStart         func()                           // Callback called after process has been started
	OnExit          func(state string)               // Callback called after the process stopped with exit state as argument
	OnStateChange   func(from, to string)            // Callback called on state change
}

// Config is the configuration for ffmpeg that is part of the configuration
// for the restreamer instance.
type Config struct {
	Binary                  string
	MaxProc                 int64
	MaxLogLines             int
	LogHistoryLength        int
	LogMinimalHistoryLength int
	ValidatorInput          Validator
	ValidatorOutput         Validator
	Portrange               net.Portranger
	Collector               session.Collector
	Resource                resources.Resources // Resource observer
	Throttling              bool                // Whether to allow CPU throttling
}

type ffmpeg struct {
	binary       string
	validatorIn  Validator
	validatorOut Validator
	portrange    net.Portranger
	skills       skills.Skills

	logLines             int
	historyLength        int
	minimalHistoryLength int

	collector session.Collector

	states     process.States
	statesLock sync.RWMutex

	resources  resources.Resources
	throttling bool
}

func New(config Config) (FFmpeg, error) {
	f := &ffmpeg{}

	if config.Resource == nil {
		return nil, fmt.Errorf("resources are required")
	}

	f.resources = config.Resource
	f.throttling = config.Throttling

	binary, err := exec.LookPath(config.Binary)
	if err != nil {
		return nil, fmt.Errorf("invalid ffmpeg binary given: %w", err)
	}

	f.binary = binary
	f.historyLength = config.LogHistoryLength
	f.minimalHistoryLength = config.LogMinimalHistoryLength
	f.logLines = config.MaxLogLines

	f.portrange = config.Portrange
	if f.portrange == nil {
		f.portrange = net.NewDummyPortrange()
	}

	f.validatorIn = config.ValidatorInput
	if f.validatorIn == nil {
		f.validatorIn, _ = NewValidator(nil, nil)
	}

	f.validatorOut = config.ValidatorOutput
	if f.validatorOut == nil {
		f.validatorOut, _ = NewValidator(nil, nil)
	}

	f.collector = config.Collector
	if f.collector == nil {
		f.collector = session.NewNullCollector()
	}

	s, err := skills.New(f.binary)
	if err != nil {
		return nil, fmt.Errorf("invalid ffmpeg binary given: %w", err)
	}
	f.skills = s

	return f, nil
}

func (f *ffmpeg) New(config ProcessConfig) (process.Process, error) {
	var scheduler process.Scheduler = nil
	var err error

	if len(config.Scheduler) != 0 {
		scheduler, err = process.NewScheduler(config.Scheduler)
		if err != nil {
			return nil, err
		}
	}

	limitMode := process.LimitModeHard
	if config.LimitMode == "soft" {
		limitMode = process.LimitModeSoft
	}

	binary := f.binary
	if len(config.Binary) != 0 {
		binary = config.Binary
	}

	ffmpeg, err := process.New(process.Config{
		Binary:          binary,
		Args:            config.Args,
		Reconnect:       config.Reconnect,
		ReconnectDelay:  config.ReconnectDelay,
		StaleTimeout:    config.StaleTimeout,
		Timeout:         config.Timeout,
		LimitCPU:        config.LimitCPU,
		Throttling:      f.throttling,
		LimitMemory:     config.LimitMemory,
		LimitGPUUsage:   config.LimitGPUUsage,
		LimitGPUEncoder: config.LimitGPUEncoder,
		LimitGPUDecoder: config.LimitGPUDecoder,
		LimitGPUMemory:  config.LimitGPUMemory,
		LimitDuration:   config.LimitDuration,
		LimitMode:       limitMode,
		Scheduler:       scheduler,
		Parser:          config.Parser,
		Logger:          config.Logger,
		OnBeforeStart:   config.OnBeforeStart,
		OnStart:         config.OnStart,
		OnExit:          config.OnExit,
		OnStateChange: func(from, to string) {
			f.statesLock.Lock()
			switch to {
			case "finished":
				f.states.Finished++
			case "starting":
				f.states.Starting++
			case "running":
				f.states.Running++
			case "finishing":
				f.states.Finishing++
			case "failed":
				f.states.Failed++
			case "killed":
				f.states.Killed++
			default:
			}
			f.statesLock.Unlock()

			if config.OnStateChange != nil {
				config.OnStateChange(from, to)
			}
		},
		Resources: f.resources,
	})

	return ffmpeg, err
}

func (f *ffmpeg) NewProcessParser(logger log.Logger, id, reference string, logpatterns []string) parse.Parser {
	p := parse.New(parse.Config{
		LogLines:          f.logLines,
		LogHistory:        f.historyLength,
		LogMinimalHistory: f.minimalHistoryLength,
		Patterns:          logpatterns,
		Logger:            logger,
		Collector:         NewWrappedCollector(id, reference, f.collector),
	})

	return p
}

func (f *ffmpeg) NewProbeParser(logger log.Logger) probe.Parser {
	p := probe.New(probe.Config{
		Logger: logger,
	})

	return p
}

func (f *ffmpeg) ValidateInputAddress(address string) bool {
	return f.validatorIn.IsValid(address)
}

func (f *ffmpeg) ValidateOutputAddress(address string) bool {
	return f.validatorOut.IsValid(address)
}

func (f *ffmpeg) Skills() skills.Skills {
	return f.skills
}

func (f *ffmpeg) ReloadSkills() error {
	s, err := skills.New(f.binary)
	if err != nil {
		return fmt.Errorf("invalid ffmpeg binary given: %w", err)
	}

	f.skills = s

	return nil
}

func (f *ffmpeg) GetPort() (int, error) {
	return f.portrange.Get()
}

func (f *ffmpeg) PutPort(port int) {
	f.portrange.Put(port)
}

func (f *ffmpeg) States() process.States {
	f.statesLock.RLock()
	defer f.statesLock.RUnlock()

	return f.states
}
