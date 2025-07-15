// Package process is a wrapper of exec.Cmd for controlling a ffmpeg process.
// It could be used to run other executables but it is tailored to the specifics
// of ffmpeg, e.g. only stderr is captured, and some exit codes != 0 plus certain
// signals are still considered as a non-error exit condition.
package process

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/resources"
)

// Process represents a process and ways to control it
// and to extract information.
type Process interface {
	// Status returns the current status of this process
	Status() Status

	// Start starts the process. If the process stops by itself
	// it will restart automatically if it is defined to do so.
	Start() error

	// Stop stops the process and will not let it restart
	// automatically.
	Stop(wait bool) error

	// Kill stops the process such that it will restart
	// automatically if it is defined to do so.
	Kill(wait bool, reason string) error

	// IsRunning returns whether the process is currently
	// running or not.
	IsRunning() bool

	// Limit enables or disables CPU and memory limiting. CPU will be throttled
	// into the configured limit. If memory consumption is above the configured
	// limit, the process will be killed.
	Limit(cpu, memory, gpu bool) error
}

// Config is the configuration of a process
type Config struct {
	Binary          string                                // Path to the ffmpeg binary.
	Args            []string                              // List of arguments for the binary.
	Reconnect       bool                                  // Whether to restart the process if it exited.
	ReconnectDelay  time.Duration                         // Duration to wait before restarting the process.
	StaleTimeout    time.Duration                         // Kill the process after this duration if it doesn't produce any output.
	Timeout         time.Duration                         // Kill the process after this duration.
	LimitCPU        float64                               // Kill the process if the CPU usage in percent is above this value, in percent 0-100 in hard mode, 0-100*ncpu in soft mode.
	Throttling      bool                                  // Whether to allow CPU throttling
	LimitMemory     uint64                                // Kill the process if the memory consumption in bytes is above this value.
	LimitGPUUsage   float64                               // Kill the process if the GPU usage in percent is above this value, in percent 0-100.
	LimitGPUEncoder float64                               // Kill the process if the GPU encoder usage in percent is above this value, in percent 0-100.
	LimitGPUDecoder float64                               // Kill the process if the GPU decoder usage in percent is above this value, in percent 0-100.
	LimitGPUMemory  uint64                                // Kill the process if the GPU memory consumption in bytes is above this value.
	LimitDuration   time.Duration                         // Kill the process if the limits are exceeded for this duration.
	LimitMode       LimitMode                             // Select limiting mode
	Scheduler       Scheduler                             // A scheduler.
	Parser          Parser                                // A parser for the output of the process.
	OnBeforeStart   func(args []string) ([]string, error) // A callback which is called before the process will be started. The string slice is the arguments of the command line. If error is non-nil, the start will be refused.
	OnStart         func()                                // A callback which is called after the process started.
	OnExit          func(state string)                    // A callback which is called after the process exited with the exit state.
	OnStateChange   func(from, to string)                 // A callback which is called after a state changed.
	Resources       resources.Resources
	Logger          log.Logger
}

// Status represents the current status of a process
type Status struct {
	PID         int32         // Last known process ID, -1 if not running
	State       string        // State is the current state of the process. See stateType for the known states.
	States      States        // States is the cumulative history of states the process had.
	Order       string        // Order is the wanted condition of process, either "start" or "stop"
	Reconnect   time.Duration // Reconnect is the time until the next reconnect, negative if no reconnect is scheduled.
	Duration    time.Duration // Duration is the time since the last change of the state
	Time        time.Time     // Time is the time of the last change of the state
	CommandArgs []string      // Currently running command arguments
	LimitMode   string        // The limiting mode
	CPU         StatusCPU     // CPU consumption in percent
	Memory      StatusMemory  // Memory consumption in bytes
	GPU         StatusGPU     // GPU consumption
}

type StatusCPU struct {
	NCPU         float64 // Number of logical CPUs
	Current      float64 // Currently consumed CPU in percent
	Average      float64 // Average consumed CPU in percent
	Max          float64 // Max. consumed CPU in percent
	Limit        float64 // Usage limit in percent
	IsThrottling bool    // Whether the CPU is currently limited
}

type StatusMemory struct {
	Current uint64 // Currently consumed memory in bytes
	Average uint64 // Average consumed memory in bytes
	Max     uint64 // Max. consumed memory in bytes
	Limit   uint64 // Usage limit in bytes
}

type StatusGPUMemory struct {
	Current uint64 // Currently consumed memory in bytes
	Average uint64 // Average consumed memory in bytes
	Max     uint64 // Max. consumed memory in bytes
	Limit   uint64 // Usage limit in bytes
}

type StatusGPUUsage struct {
	Current float64 // Currently consumed GPU usage in percent
	Average float64 // Average consumed GPU usage in percent
	Max     float64 // Max. consumed GPU usage in percent
	Limit   float64 // Usage limit in percent
}

type StatusGPU struct {
	Index   int
	Memory  StatusGPUMemory // GPU memory consumption
	Usage   StatusGPUUsage  // GPU usage in percent
	Encoder StatusGPUUsage  // GPU encoder usage in percent
	Decoder StatusGPUUsage  // GPU decoder usage in percent
}

// States
//
// finished - Process has been stopped
//
//	starting - if process has been actively started or has been waiting for reconnect (order=start, reconnect=any)
//	finished - if process shall not reconnect (order=stop, reconnect=any)
//
// starting - Process is about to start
//
//	finishing - if process should be immediately stopped (order=stop, reconnect=any)
//	running - if process could be started (order=start, reconnect=any)
//	failed - if process couldn't be started (e.g. binary not found) (order=start, reconnect=any)
//
// running - Process is running
//
//	finished - if process exited normally (order=any, reconnect=any)
//	finishing - if process has been actively stopped (order=stop, reconnect=any)
//	failed - if process exited abnormally (order=any, reconnect=any)
//	killed - if process has been actively killed with SIGKILL (order=any, reconnect=any)
//
// finishing - Process has been actively stopped and will be killed
//
//	finished - if process has been actively killed with SIGINT and ffmpeg exited normally (order=stop, reconnect=any)
//	killed - if process has been actively killed with SIGKILL (order=stop, reconnect=any)
//
// failed - Process has been failed either by starting or during running
//
//	starting - if process has been waiting for reconnect (order=start, reconnect=true)
//	failed - if process shall not reconnect (order=any, reconnect=false)
//
// killed - Process has been stopped
//
//	starting - if process has been waiting for reconnect (order=start, reconnect=true)
//	killed - if process shall not reconnect (order=start, reconnect=false)
type stateType string

const (
	stateFinished  stateType = "finished"
	stateStarting  stateType = "starting"
	stateRunning   stateType = "running"
	stateFinishing stateType = "finishing"
	stateFailed    stateType = "failed"
	stateKilled    stateType = "killed"
)

// String returns a string representation of the state
func (s stateType) String() string {
	return string(s)
}

// IsRunning returns whether the state is representing a running state
func (s stateType) IsRunning() bool {
	if s == stateStarting || s == stateRunning || s == stateFinishing {
		return true
	}

	return false
}

type States struct {
	Finished  uint64
	Starting  uint64
	Running   uint64
	Finishing uint64
	Failed    uint64
	Killed    uint64
}

// Process represents a ffmpeg process
type process struct {
	binary  string
	args    []string
	cmdArgs []string
	cmd     *exec.Cmd
	pid     atomic.Int32
	stdout  io.ReadCloser
	state   struct {
		state  stateType
		time   time.Time
		states States
		lock   sync.RWMutex
	}
	order struct {
		order string
		lock  sync.Mutex
	}
	parser Parser
	stale  struct {
		last    time.Time
		timeout time.Duration
		cancel  context.CancelFunc
		lock    sync.Mutex
	}
	reconn struct {
		enable      bool
		delay       time.Duration
		reconnectAt time.Time
		timer       *time.Timer
		lock        sync.Mutex
	}
	timeout        time.Duration
	stopTimer      *time.Timer
	stopTimerLock  sync.Mutex
	stopReason     string
	stopReasonLock sync.Mutex
	killTimer      *time.Timer
	killTimerLock  sync.Mutex
	logger         log.Logger
	debuglogger    log.Logger
	callbacks      struct {
		onBeforeStart func(args []string) ([]string, error)
		onStart       func()
		onExit        func(state string)
		onStateChange func(from, to string)
		lock          sync.Mutex
	}
	limits    Limiter
	scheduler Scheduler
	resources resources.Resources
}

var _ Process = &process{}

// New creates a new process wrapper
func New(config Config) (Process, error) {
	p := &process{
		binary:    config.Binary,
		cmd:       nil,
		timeout:   config.Timeout,
		parser:    config.Parser,
		logger:    config.Logger,
		scheduler: config.Scheduler,
		resources: config.Resources,
	}

	p.pid.Store(-1)

	if p.resources == nil {
		return nil, fmt.Errorf("resources are required")
	}

	p.args = make([]string, len(config.Args))
	copy(p.args, config.Args)
	p.cmdArgs = make([]string, len(config.Args))
	copy(p.cmdArgs, config.Args)

	// This is a loose check on purpose. If the e.g. the binary
	// doesn't exist or it is not executable, it will be
	// reflected in the resulting state.
	if len(p.binary) == 0 {
		return nil, fmt.Errorf("no valid binary given")
	}

	if p.parser == nil {
		p.parser = NewNullParser()
	}

	if p.logger == nil {
		p.logger = log.New("Process")
	}

	p.debuglogger = p.logger.WithFields(log.Fields{
		"binary": p.binary,
		"args":   p.args,
	})

	p.setOrder("stop")

	p.initState(stateFinished)

	p.reconn.enable = config.Reconnect
	p.reconn.delay = config.ReconnectDelay

	p.stale.last = time.Now()
	p.stale.timeout = config.StaleTimeout

	p.callbacks.onBeforeStart = config.OnBeforeStart
	p.callbacks.onStart = config.OnStart
	p.callbacks.onExit = config.OnExit
	p.callbacks.onStateChange = config.OnStateChange

	ncpu := p.resources.Info().CPU.NCPU

	limits, err := NewLimiter(LimiterConfig{
		CPU:        config.LimitCPU,
		Throttling: config.Throttling,
		NCPU:       ncpu,
		Memory:     config.LimitMemory,
		GPUUsage:   config.LimitGPUUsage,
		GPUEncoder: config.LimitGPUEncoder,
		GPUDecoder: config.LimitGPUDecoder,
		GPUMemory:  config.LimitGPUMemory,
		WaitFor:    config.LimitDuration,
		Mode:       config.LimitMode,
		Logger:     p.logger.WithComponent("ProcessLimiter"),
		OnLimit: func(cpu float64, memory uint64, gpuusage, gpuencoder, gpudecoder float64, gpumemory uint64) {
			if !p.isRunning() {
				return
			}

			p.logger.WithFields(log.Fields{
				"cpu":        cpu,
				"memory":     memory,
				"gpuusage":   gpuusage,
				"gpuencoder": gpuencoder,
				"gpudecoder": gpudecoder,
				"gpumemmory": gpumemory,
			}).Warn().Log("Killed because limits are exceeded")
			p.Kill(false, fmt.Sprintf("Killed because limits are exceeded (mode: %s, tolerance: %s): %.2f (%.2f) CPU, %d (%d) bytes memory, %.2f/%.2f/%.2f (%.2f) GPU usage, %d (%d) bytes GPU memory", config.LimitMode.String(), config.LimitDuration.String(), cpu, config.LimitCPU, memory, config.LimitMemory, gpuusage, gpuencoder, gpudecoder, config.LimitGPUUsage, gpumemory, config.LimitGPUMemory))
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize limiter")
	}
	p.limits = limits

	p.logger.Info().Log("Created")
	p.debuglogger.Debug().Log("Created")

	return p, nil
}

func (p *process) initState(state stateType) {
	p.state.lock.Lock()
	defer p.state.lock.Unlock()

	p.state.state = state
	p.state.time = time.Now()
}

// setState sets a new state. It also checks if the transition
// of the current state to the new state is allowed. If not,
// the current state will not be changed. It returns the previous
// state or an error
func (p *process) setState(state stateType) (stateType, error) {
	p.state.lock.Lock()
	defer p.state.lock.Unlock()

	prevState := p.state.state
	failed := false

	if prevState == state {
		return prevState, nil
	}

	if p.state.state == stateFinished {
		switch state {
		case stateStarting:
			p.state.state = state
			p.state.states.Starting++
		default:
			failed = true
		}
	} else if p.state.state == stateStarting {
		switch state {
		case stateFinishing:
			p.state.state = state
			p.state.states.Finishing++
		case stateRunning:
			p.state.state = state
			p.state.states.Running++
		case stateFailed:
			p.state.state = state
			p.state.states.Failed++
		default:
			failed = true
		}
	} else if p.state.state == stateRunning {
		switch state {
		case stateFinished:
			p.state.state = state
			p.state.states.Finished++
		case stateFinishing:
			p.state.state = state
			p.state.states.Finishing++
		case stateFailed:
			p.state.state = state
			p.state.states.Failed++
		case stateKilled:
			p.state.state = state
			p.state.states.Killed++
		default:
			failed = true
		}
	} else if p.state.state == stateFinishing {
		switch state {
		case stateFinished:
			p.state.state = state
			p.state.states.Finished++
		case stateFailed:
			p.state.state = state
			p.state.states.Failed++
		case stateKilled:
			p.state.state = state
			p.state.states.Killed++
		default:
			failed = true
		}
	} else if p.state.state == stateFailed {
		switch state {
		case stateStarting:
			p.state.state = state
			p.state.states.Starting++
		default:
			failed = true
		}
	} else if p.state.state == stateKilled {
		switch state {
		case stateStarting:
			p.state.state = state
			p.state.states.Starting++
		default:
			failed = true
		}
	} else {
		return "", fmt.Errorf("current state is unhandled: %s", p.state.state)
	}

	if failed {
		return "", fmt.Errorf("can't change from state %s to %s", p.state.state, state)
	}

	p.state.time = time.Now()

	if p.callbacks.onStateChange != nil {
		p.callbacks.onStateChange(prevState.String(), p.state.state.String())
	}

	return prevState, nil
}

func (p *process) getState() stateType {
	p.state.lock.RLock()
	defer p.state.lock.RUnlock()

	return p.state.state
}

func (p *process) getOrder() string {
	p.order.lock.Lock()
	defer p.order.lock.Unlock()

	return p.order.order
}

// setOrder sets the order to the given value. If the
// order already has that order, it returns true. Otherwise
// false.
func (p *process) setOrder(order string) bool {
	p.order.lock.Lock()
	defer p.order.lock.Unlock()

	if p.order.order == order {
		return true
	}

	p.order.order = order

	return false
}

func (p *process) isRunning() bool {
	p.state.lock.RLock()
	defer p.state.lock.RUnlock()

	return p.state.state.IsRunning()
}

func (p *process) getStateString() string {
	p.state.lock.RLock()
	defer p.state.lock.RUnlock()

	return p.state.state.String()
}

// Status returns the current status of the process
func (p *process) Status() Status {
	usage := p.limits.Usage()

	p.state.lock.RLock()
	stateTime := p.state.time
	state := p.state.state
	states := p.state.states
	p.state.lock.RUnlock()

	if state == stateRunning && !p.parser.IsRunning() {
		state = stateStarting
	}

	order := p.getOrder()

	s := Status{
		PID:       p.pid.Load(),
		State:     state.String(),
		States:    states,
		Order:     order,
		Reconnect: time.Duration(-1),
		Duration:  time.Since(stateTime),
		Time:      stateTime,
		LimitMode: p.limits.Mode().String(),
		CPU: StatusCPU{
			NCPU:         usage.CPU.NCPU,
			Current:      usage.CPU.Current,
			Average:      usage.CPU.Average,
			Max:          usage.CPU.Max,
			Limit:        usage.CPU.Limit,
			IsThrottling: usage.CPU.IsThrottling,
		},
		Memory: StatusMemory{
			Current: usage.Memory.Current,
			Average: uint64(usage.Memory.Average),
			Max:     usage.Memory.Max,
			Limit:   usage.Memory.Limit,
		},
		GPU: StatusGPU{
			Index: usage.GPU.Index,
			Memory: StatusGPUMemory{
				Current: usage.GPU.Memory.Current,
				Average: uint64(usage.GPU.Memory.Average),
				Max:     usage.GPU.Memory.Max,
				Limit:   usage.GPU.Memory.Limit,
			},
			Usage: StatusGPUUsage{
				Current: usage.GPU.Usage.Current,
				Average: usage.GPU.Usage.Average,
				Max:     usage.GPU.Usage.Max,
				Limit:   usage.GPU.Usage.Limit,
			},
			Encoder: StatusGPUUsage{
				Current: usage.GPU.Encoder.Current,
				Average: usage.GPU.Encoder.Average,
				Max:     usage.GPU.Encoder.Max,
				Limit:   usage.GPU.Encoder.Limit,
			},
			Decoder: StatusGPUUsage{
				Current: usage.GPU.Decoder.Current,
				Average: usage.GPU.Decoder.Average,
				Max:     usage.GPU.Decoder.Max,
				Limit:   usage.GPU.Decoder.Limit,
			},
		},
	}

	s.CommandArgs = make([]string, len(p.cmdArgs))
	copy(s.CommandArgs, p.cmdArgs)

	if order == "start" && !state.IsRunning() {
		p.reconn.lock.Lock()
		s.Reconnect = time.Until(p.reconn.reconnectAt)
		p.reconn.lock.Unlock()
	}

	return s
}

// IsRunning returns whether the process is considered running
func (p *process) IsRunning() bool {
	return p.isRunning()
}

func (p *process) Limit(cpu, memory, gpu bool) error {
	if !p.isRunning() {
		return nil
	}

	if p.limits == nil {
		return nil
	}

	p.logger.Warn().WithFields(log.Fields{
		"limit_cpu":       cpu,
		"limit_memory":    memory,
		"limit_gpumemory": gpu,
	}).Log("Limiter triggered")

	return p.limits.Limit(cpu, memory, gpu)
}

// Start will start the process and sets the order to "start". If the
// process has alread the "start" order, nothing will be done. Returns
// an error if start failed.
func (p *process) Start() error {
	if p.setOrder("start") {
		return nil
	}

	if p.scheduler != nil {
		next, err := p.scheduler.Next()
		if err != nil {
			return err
		}

		p.reconnect(next)

		return nil
	}

	err := p.start()
	if err != nil {
		p.debuglogger.WithFields(log.Fields{
			"state": p.getStateString(),
			"order": p.getOrder(),
			"error": err,
		}).Debug().Log("Starting failed")
	}

	return err
}

// start will start the process considering the current order. Returns an
// error in case something goes wrong, and it will try to restart the process.
func (p *process) start() error {
	var err error

	// Bail out if the process is already running
	if p.isRunning() {
		return nil
	}

	p.logger.Info().Log("Starting")
	p.debuglogger.WithFields(log.Fields{
		"state": p.getStateString(),
		"order": p.getOrder(),
	}).Debug().Log("Starting")

	// Stop any restart timer in order to start the process immediately
	p.unreconnect()

	p.setState(stateStarting)

	args := p.args

	if p.callbacks.onBeforeStart != nil {
		args = make([]string, len(p.args))
		copy(args, p.args)

		args, err = p.callbacks.onBeforeStart(args)
		if err != nil {
			p.setState(stateFailed)

			p.parser.Parse([]byte(err.Error()))
			p.logger.WithError(err).Error().Log("Starting failed")

			p.reconnect(p.delay(stateFailed))

			return err
		}

		p.cmdArgs = args
	}

	p.cmd = exec.Command(p.binary, args...)
	p.cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	p.cmd.Env = []string{}

	p.stdout, err = p.cmd.StderrPipe()
	if err != nil {
		p.setState(stateFailed)

		p.parser.Parse([]byte(err.Error()))
		p.logger.WithError(err).Error().Log("Command failed")

		p.reconnect(p.delay(stateFailed))

		return err
	}

	if err := p.cmd.Start(); err != nil {
		p.setState(stateFailed)

		p.parser.Parse([]byte(err.Error()))
		p.logger.WithError(err).Error().Log("Command failed")
		p.reconnect(p.delay(stateFailed))

		p.callbacks.lock.Lock()
		if p.callbacks.onExit != nil {
			p.callbacks.onExit(stateFailed.String())
		}
		p.callbacks.lock.Unlock()

		return err
	}

	p.pid.Store(int32(p.cmd.Process.Pid))

	// Start the stop timeout if enabled
	if p.timeout > time.Duration(0) {
		p.stopTimerLock.Lock()
		if p.stopTimer == nil {
			// Only create a new timer if there isn't already one running
			p.stopTimer = time.AfterFunc(p.timeout, func() {
				p.Kill(false, fmt.Sprintf("Killed because timeout triggered (%s)", p.timeout))

				p.stopTimerLock.Lock()
				p.stopTimer.Stop()
				p.stopTimer = nil
				p.stopTimerLock.Unlock()
			})
		}
		p.stopTimerLock.Unlock()
	}

	if proc, err := p.resources.Process(p.pid.Load()); err == nil {
		p.limits.Start(proc)
	}

	p.setState(stateRunning)

	p.logger.Info().Log("Started")
	p.debuglogger.Debug().Log("Started")

	if p.callbacks.onStart != nil {
		p.callbacks.onStart()
	}

	// Start the reader
	go p.reader()

	// Start the stale timeout if enabled
	if p.stale.timeout != 0 {
		var ctx context.Context

		p.stale.lock.Lock()
		ctx, p.stale.cancel = context.WithCancel(context.Background())
		p.stale.lock.Unlock()

		go p.staler(ctx)
	}

	return nil
}

// Stop will stop the process and set the order to "stop"
func (p *process) Stop(wait bool) error {
	if p.setOrder("stop") {
		return nil
	}

	err := p.stop(wait, "")
	if err != nil {
		p.debuglogger.WithFields(log.Fields{
			"state": p.getStateString(),
			"order": p.getOrder(),
			"error": err,
		}).Debug().Log("Stopping failed")
	}

	return err
}

// Kill will stop the process without changing the order such that it
// will restart automatically if enabled.
func (p *process) Kill(wait bool, reason string) error {
	// If the process is currently not running, we don't need
	// to do anything.
	if !p.isRunning() {
		return nil
	}

	err := p.stop(wait, reason)

	return err
}

// stop will stop a process considering the current order and state.
func (p *process) stop(wait bool, reason string) error {
	// Stop the restart timer
	p.unreconnect()

	// If the process is currently not running, bail out
	if !p.isRunning() {
		return nil
	}

	// If the process in starting state, wait until the process has been started
	for {
		if p.getState() == stateStarting {
			time.Sleep(500 * time.Millisecond)
			continue
		}

		break
	}

	// If the process is already in the finishing state, don't do anything
	if state, _ := p.setState(stateFinishing); state == stateFinishing {
		return nil
	}

	p.stopReasonLock.Lock()
	p.stopReason = reason
	p.stopReasonLock.Unlock()

	p.logger.Info().Log("Stopping")
	p.debuglogger.WithFields(log.Fields{
		"state": p.getStateString(),
		"order": p.getOrder(),
	}).Debug().Log("Stopping")

	wg := sync.WaitGroup{}

	if wait {
		wg.Add(1)

		p.callbacks.lock.Lock()
		if p.callbacks.onExit == nil {
			p.callbacks.onExit = func(string) {
				wg.Done()

				p.callbacks.onExit = nil
			}
		} else {
			cb := p.callbacks.onExit
			p.callbacks.onExit = func(state string) {
				cb(state)
				wg.Done()

				p.callbacks.onExit = cb
			}
		}
		p.callbacks.lock.Unlock()
	}

	var err error
	if runtime.GOOS == "windows" {
		// Windows doesn't know the SIGINT
		p.cmd.Process.Kill()
	} else {
		// First try to kill the process gracefully. On a SIGINT ffmpeg will exit
		// normally as if "q" has been pressed.
		p.cmd.Process.Signal(os.Interrupt)
	}

	// Set up a timer to kill the process with SIGKILL in case SIGINT didn't have
	// an effect.
	p.killTimerLock.Lock()
	p.killTimer = time.AfterFunc(5*time.Second, func() {
		p.cmd.Process.Kill()
		p.stdout.Close()
	})
	p.killTimerLock.Unlock()

	if wait {
		wg.Wait()
	}

	return err
}

// reconnect will setup a timer to restart the process
func (p *process) reconnect(delay time.Duration) {
	p.reconn.lock.Lock()
	defer p.reconn.lock.Unlock()

	if p.reconn.timer != nil {
		p.reconn.timer.Stop()
		p.reconn.timer = nil
	}

	if delay < time.Duration(0) {
		return
	}

	p.logger.Info().Log("Scheduling restart in %s", delay)

	p.reconn.reconnectAt = time.Now().Add(delay)
	p.reconn.timer = time.AfterFunc(delay, func() {
		p.start()
	})
}

// unreconnect will stop the restart timer
func (p *process) unreconnect() {
	p.reconn.lock.Lock()
	defer p.reconn.lock.Unlock()

	if p.reconn.timer == nil {
		return
	}

	p.reconn.timer.Stop()
	p.reconn.timer = nil
}

// staler checks if the currently running process is stale, i.e. the reader
// didn't update the time of the last read. If the timeout is reached, the
// process will be stopped such that it can restart automatically afterwards.
func (p *process) staler(ctx context.Context) {
	p.stale.lock.Lock()
	p.stale.last = time.Now()
	p.stale.lock.Unlock()

	p.debuglogger.Debug().Log("Starting stale watcher")

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.debuglogger.Debug().Log("Stopping stale watcher")
			return
		case t := <-ticker.C:
			p.stale.lock.Lock()
			last := p.stale.last
			timeout := p.stale.timeout
			p.stale.lock.Unlock()

			d := t.Sub(last)
			if d.Seconds() > timeout.Seconds() {
				p.logger.Info().Log("Stale timeout after %s (%.2fs).", timeout, d.Seconds())
				p.stop(false, fmt.Sprintf("Stale timeout after %s, no output received from process", timeout))
				return
			}
		}
	}
}

// reader reads the output from the process line by line and gives
// each line to the parser. The parser returns a postive number to
// indicate progress. If the returned number is zero, then the time
// of the last progress will not be updated thus the stale timeout
// may kick in.
func (p *process) reader() {
	scanner := bufio.NewScanner(p.stdout)
	scanner.Split(scanLines)

	// Reset the parser statistics
	p.parser.ResetStats()

	// Reset the parser logs
	p.parser.ResetLog()

	var n uint64 = 0

	for scanner.Scan() {
		line := scanner.Bytes()

		// Parse the output line from ffmpeg
		n = p.parser.Parse(line)

		// Reset the stale progress timer only if the
		// parser reports progress
		if n != 0 {
			p.stale.lock.Lock()
			p.stale.last = time.Now()
			p.stale.lock.Unlock()
		}
	}

	if err := scanner.Err(); err != nil {
		p.logger.Debug().WithError(err).Log("")
		p.parser.Parse([]byte(err.Error()))
	}

	p.stopReasonLock.Lock()
	if len(p.stopReason) != 0 {
		p.parser.Parse([]byte(p.stopReason))
		p.stopReason = ""
	}
	p.stopReasonLock.Unlock()

	// Wait for the process to finish
	p.waiter()
}

// waiter waits for the process to finish. If enabled, the process will
// be scheduled for a restart.
func (p *process) waiter() {
	// The process exited normally, i.e. the return code is zero and no signal has been raised
	state := stateFinished
	enableReconnect := true

	if err := p.cmd.Wait(); err != nil {
		// The process exited abnormally, i.e. the return code is non-zero or a signal
		// has been raised.
		if exiterr, ok := err.(*exec.ExitError); ok {
			// The process exited and the status can be examined
			status := exiterr.Sys().(syscall.WaitStatus)

			p.debuglogger.WithFields(log.Fields{
				"exited":      status.Exited(),
				"signaled":    status.Signaled(),
				"status":      status.ExitStatus(),
				"exit_code":   exiterr.ExitCode(),
				"exit_string": exiterr.String(),
				"signal":      status.Signal().String(),
			}).Debug().Log("Exited")

			if int(status.Signal()) == 6 { // If ffmpeg has been killed with SIGABRT, it will disable a reconnect.
				enableReconnect = false
			}

			if status.Exited() {
				if status.ExitStatus() == 255 {
					// If ffmpeg has been killed with a SIGINT, SIGTERM, etc., then it exited normally,
					// i.e. closing all stream properly such that all written data is sane.
					p.logger.Info().Log("Finished")
					state = stateFinished
				} else {
					// The process exited by itself with a non-zero return code
					p.logger.Info().Log("Failed")
					state = stateFailed
				}
			} else if status.Signaled() {
				// If ffmpeg has been killed the hard way, something went wrong and
				// it can be assumed that any written data is not sane.
				p.logger.Info().Log("Killed")
				state = stateKilled
			} else {
				// The process exited because of something else (e.g. coredump, ...)
				p.logger.Info().Log("Killed")
				state = stateKilled
			}
		} else {
			// Some other error regarding I/O triggered during Wait()
			p.logger.Info().Log("Killed")
			p.logger.WithError(err).Debug().Log("Killed")
			state = stateKilled
		}
	}

	p.setState(state)
	p.pid.Store(-1)

	p.logger.Info().Log("Stopped")
	p.debuglogger.WithField("log", p.parser.Log()).Debug().Log("Stopped")

	pusage := p.limits.Usage()
	p.limits.Stop()

	// Stop the stop timer
	if state == stateFinished {
		// Only clear the timer if the process finished normally
		p.stopTimerLock.Lock()
		if p.stopTimer != nil {
			p.stopTimer.Stop()
			p.stopTimer = nil
		}
		p.stopTimerLock.Unlock()
	}

	// Stop the kill timer
	p.killTimerLock.Lock()
	if p.killTimer != nil {
		p.killTimer.Stop()
		p.killTimer = nil
	}
	p.killTimerLock.Unlock()

	// Stop the stale progress timer
	p.stale.lock.Lock()
	if p.stale.cancel != nil {
		p.stale.cancel()
		p.stale.cancel = nil
	}
	p.stale.lock.Unlock()

	// Send exit state to the parser
	p.parser.Stop(state.String(), pusage)

	// Reset the parser stats
	p.parser.ResetStats()

	p.debuglogger.WithFields(log.Fields{
		"state": state.String(),
		"order": p.getOrder(),
	}).Debug().Log("Waiting")

	// Restart the process
	if p.getOrder() == "start" && enableReconnect {
		p.reconnect(p.delay(state))
	}

	// Call the onExit callback
	p.callbacks.lock.Lock()
	if p.callbacks.onExit != nil {
		p.callbacks.onExit(state.String())
	}
	p.callbacks.lock.Unlock()
}

// delay returns the duration for the next reconnect of the process. If no reconnect is
// wanted, it returns a negative duration.
func (p *process) delay(state stateType) time.Duration {
	// By default, reconnect after the configured delay.
	delay := p.reconn.delay

	if p.scheduler == nil {
		// No scheduler has been provided, reconnect in any case, if enabled.
		if !p.reconn.enable {
			return time.Duration(-1)
		}

		return delay
	}

	// Get the next scheduled start time.
	next, err := p.scheduler.Next()
	if err == nil {
		// There's a next scheduled time.
		if state == stateFinished {
			// If the process finished without error, reconnect at the next scheduled time.
			delay = next
		} else {
			// The process finished with an error.
			if !p.reconn.enable {
				// If reconnect is not enabled, reconnect at the next scheduled time.
				delay = next
			} else {
				// If the next scheduled time is closer than the next configured delay,
				// reconnect at the next scheduled time
				if next < p.reconn.delay {
					delay = next
				}
			}
		}
	} else {
		// No next scheduled time.
		if state == stateFinished {
			// If the process finished without error, don't reconnect.
			delay = time.Duration(-1)
		} else {
			// If the process finished with an error, reconnect if enabled.
			if !p.reconn.enable {
				delay = time.Duration(-1)
			}
		}
	}

	return delay
}

// scanLine splits the data on \r, \n, or \r\n line endings
func scanLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	// Skip leading spaces.
	start := 0
	for width := 0; start < len(data); start += width {
		var r rune
		r, width = utf8.DecodeRune(data[start:])
		if r != '\n' && r != '\r' {
			break
		}

	}

	// Scan until new line, marking end of line.
	for width, i := 0, start; i < len(data); i += width {
		var r rune
		r, width = utf8.DecodeRune(data[i:])
		if r == '\n' || r == '\r' {
			return i + width, data[start:i], nil
		}

	}

	// If we're at EOF, we have a final, non-empty, non-terminated word. Return it.
	if atEOF && len(data) > start {
		return len(data), data[start:], nil
	}

	// Request more data.
	return start, nil, nil
}
