package restream

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/datarhei/core/v16/ffmpeg"
	"github.com/datarhei/core/v16/ffmpeg/skills"
	"github.com/datarhei/core/v16/glob"
	"github.com/datarhei/core/v16/io/fs"
	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/net"
	"github.com/datarhei/core/v16/net/url"
	"github.com/datarhei/core/v16/resources"
	"github.com/datarhei/core/v16/restream/app"
	rfs "github.com/datarhei/core/v16/restream/fs"
	"github.com/datarhei/core/v16/restream/replace"
	"github.com/datarhei/core/v16/restream/rewrite"
	"github.com/datarhei/core/v16/restream/store"

	"github.com/Masterminds/semver/v3"
)

// The Restreamer interface
type Restreamer interface {
	ID() string           // ID of this instance
	Name() string         // Arbitrary name of this instance
	CreatedAt() time.Time // Time of when this instance has been created
	Start()               // Start all processes that have a "start" order
	Stop()                // Stop all running process but keep their "start" order

	Skills() skills.Skills                          // Get the ffmpeg skills
	ReloadSkills() error                            // Reload the ffmpeg skills
	SetMetadata(key string, data interface{}) error // Set general metadata
	GetMetadata(key string) (interface{}, error)    // Get previously set general metadata

	AddProcess(config *app.Config) error                                                                              // Add a new process
	GetProcessIDs(idpattern, refpattern, ownerpattern, domainpattern string) []app.ProcessID                          // Get a list of process IDs based on patterns for ID and reference
	DeleteProcess(id app.ProcessID) error                                                                             // Delete a process
	UpdateProcess(id app.ProcessID, config *app.Config) error                                                         // Update a process
	StartProcess(id app.ProcessID) error                                                                              // Start a process
	StopProcess(id app.ProcessID) error                                                                               // Stop a process
	RestartProcess(id app.ProcessID) error                                                                            // Restart a process
	ReloadProcess(id app.ProcessID) error                                                                             // Reload a process
	GetProcess(id app.ProcessID) (*app.Process, error)                                                                // Get a process
	GetProcessState(id app.ProcessID) (*app.State, error)                                                             // Get the state of a process
	GetProcessReport(id app.ProcessID) (*app.Report, error)                                                           // Get the logs of a process
	SetProcessReport(id app.ProcessID, report *app.Report) error                                                      // Set the log history of a process
	SearchProcessLogHistory(idpattern, refpattern, state string, from, to *time.Time) []app.ReportHistorySearchResult // Search the log history of all processes
	GetPlayout(id app.ProcessID, inputid string) (string, error)                                                      // Get the URL of the playout API for a process
	SetProcessMetadata(id app.ProcessID, key string, data interface{}) error                                          // Set metatdata to a process
	GetProcessMetadata(id app.ProcessID, key string) (interface{}, error)                                             // Get previously set metadata from a process

	Probe(config *app.Config, timeout time.Duration) app.Probe // Probe a process with specific timeout
}

// Config is the required configuration for a new restreamer instance.
type Config struct {
	ID           string
	Name         string
	Store        store.Store
	Filesystems  []fs.Filesystem
	Replace      replace.Replacer
	Rewrite      rewrite.Rewriter
	FFmpeg       ffmpeg.FFmpeg
	MaxProcesses int64
	Resources    resources.Resources
	Logger       log.Logger
}

type restream struct {
	id        string
	name      string
	createdAt time.Time
	store     store.Store
	ffmpeg    ffmpeg.FFmpeg
	maxProc   int64
	nProc     int64
	fs        struct {
		list []rfs.Filesystem
	}
	replace  replace.Replacer
	rewrite  rewrite.Rewriter
	tasks    *Storage               // domain:ProcessID
	metadata map[string]interface{} // global metadata
	logger   log.Logger

	resources       resources.Resources
	enableSoftLimit bool

	lock sync.RWMutex

	cancelObserver context.CancelFunc

	startOnce sync.Once
	stopOnce  sync.Once
}

// New returns a new instance that implements the Restreamer interface
func New(config Config) (Restreamer, error) {
	r := &restream{
		id:        config.ID,
		name:      config.Name,
		createdAt: time.Now(),
		store:     config.Store,
		replace:   config.Replace,
		rewrite:   config.Rewrite,
		logger:    config.Logger,
		tasks:     NewStorage(),
		metadata:  map[string]interface{}{},
	}

	if r.logger == nil {
		r.logger = log.New("")
	}

	if len(config.Filesystems) == 0 {
		return nil, fmt.Errorf("at least one filesystem must be provided")
	}

	for _, fs := range config.Filesystems {
		newfs, err := rfs.New(rfs.Config{
			FS:       fs,
			Interval: 5 * time.Second,
			Logger:   r.logger.WithComponent("Cleanup"),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create cleaup fs for %s", fs.Name())
		}

		r.fs.list = append(r.fs.list, newfs)
	}

	if r.replace == nil {
		r.replace = replace.New()
	}

	r.ffmpeg = config.FFmpeg
	if r.ffmpeg == nil {
		return nil, fmt.Errorf("ffmpeg must be provided")
	}

	r.maxProc = config.MaxProcesses
	r.resources = config.Resources
	if r.resources == nil {
		rsc, err := resources.New(resources.Config{})
		if err != nil {
			return nil, fmt.Errorf("failed to create dummy resource manager: %w", err)
		}

		r.resources = rsc
	}

	r.enableSoftLimit = r.resources.HasLimits()

	if err := r.load(); err != nil {
		return nil, fmt.Errorf("failed to load data from DB: %w", err)
	}

	r.save()

	r.stopOnce.Do(func() {})

	return r, nil
}

func (r *restream) Start() {
	r.startOnce.Do(func() {
		r.lock.Lock()
		defer r.lock.Unlock()

		ctx, cancel := context.WithCancel(context.Background())
		r.cancelObserver = cancel

		if r.enableSoftLimit {
			go r.resourceObserver(ctx, r.resources, time.Second)
		}

		r.tasks.Range(true, func(id app.ProcessID, t *task, token string) bool {
			defer t.Release(token)

			t.Restore()

			// The filesystem cleanup rules can be set
			r.setCleanup(id, t.config)

			return true
		})

		for _, fs := range r.fs.list {
			fs.Start()

			if fs.Type() == "disk" {
				go r.filesystemObserver(ctx, fs, 10*time.Second)
			}
		}

		r.stopOnce = sync.Once{}
	})
}

func (r *restream) Stop() {
	r.stopOnce.Do(func() {
		r.lock.Lock()
		defer r.lock.Unlock()

		wg := sync.WaitGroup{}

		// Stop the currently running processes without altering their order such that on a subsequent
		// Start() they will get restarted.

		r.tasks.Range(true, func(_ app.ProcessID, t *task, token string) bool {
			wg.Add(1)
			go func(t *task) {
				defer t.Release(token)
				defer wg.Done()
				t.Kill()
			}(t)

			return true
		})

		wg.Wait()

		r.tasks.Range(true, func(id app.ProcessID, t *task, token string) bool {
			defer t.Release(token)
			r.unsetCleanup(id)
			return true
		})

		r.cancelObserver()

		// Stop the cleanup jobs
		for _, fs := range r.fs.list {
			fs.Stop()
		}

		r.startOnce = sync.Once{}
	})
}

func (r *restream) filesystemObserver(ctx context.Context, fs fs.Filesystem, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			size, limit := fs.Size()
			isFull := false
			if limit > 0 && size >= limit {
				isFull = true
			}

			if isFull {
				// Stop all tasks that write to this filesystem
				r.tasks.Range(true, func(id app.ProcessID, t *task, token string) bool {
					defer t.Release(token)
					if !t.UsesDisk() {
						return true
					}

					r.logger.Warn().WithField("id", id).Log("Shutting down because filesystem is full")
					t.Stop()

					return true
				})
			}
		}
	}
}

func (r *restream) resourceObserver(ctx context.Context, rsc resources.Resources, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	limitCPU, limitMemory := false, false
	var limitGPUs []bool = nil

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cpu, memory, gpu := rsc.ShouldLimit()

			hasChanges := false

			if cpu != limitCPU {
				limitCPU = cpu
				hasChanges = true
			}

			if memory != limitMemory {
				limitMemory = memory
				hasChanges = true
			}

			if limitGPUs == nil {
				limitGPUs = make([]bool, len(gpu))
			}

			for i, g := range gpu {
				if g != limitGPUs[i] {
					limitGPUs[i] = g
					hasChanges = true
				}
			}

			if !hasChanges {
				break
			}

			r.tasks.Range(true, func(id app.ProcessID, t *task, token string) bool {
				defer t.Release(token)
				limitGPU := false
				gpuindex := t.GetHWDevice()
				if gpuindex >= 0 {
					limitGPU = limitGPUs[gpuindex]
				}
				if t.Limit(limitCPU, limitMemory, limitGPU) {
					r.logger.Debug().WithFields(log.Fields{
						"limit_cpu":    limitCPU,
						"limit_memory": limitMemory,
						"limit_gpu":    limitGPU,
						"id":           id,
					}).Log("Limiting process CPU, memory, and GPU consumption")
				}

				return true
			})
		}
	}
}

func (r *restream) load() error {
	if r.store == nil {
		return nil
	}

	data, err := r.store.Load()

	if err != nil {
		return err
	}

	tasks := NewStorage()

	skills := r.ffmpeg.Skills()
	ffversion := skills.FFmpeg.Version

	if v, err := semver.NewVersion(ffversion); err == nil {
		// Remove the patch level for the constraint
		ffversion = fmt.Sprintf("%d.%d.0", v.Major(), v.Minor())
	}

	for _, domain := range data.Process {
		for _, p := range domain {
			if len(p.Process.Config.FFVersion) == 0 {
				p.Process.Config.FFVersion = "^" + ffversion
			}

			t := NewTask(p.Process, r.logger.WithFields(log.Fields{
				"id":        p.Process.ID,
				"owner":     p.Process.Owner,
				"domain":    p.Process.Domain,
				"reference": p.Process.Reference,
			}))

			t.ImportMetadata(p.Metadata)

			// Replace all placeholders in the config
			resolveStaticPlaceholders(t.config, r.replace)

			tasks.Store(t.ID(), t)
		}
	}

	// Now that all tasks are defined and all placeholders are
	// replaced, we can resolve references and validate the
	// inputs and outputs.

	tasks.Range(false, func(_ app.ProcessID, t *task, token string) bool {
		defer t.Release(token)

		// Just warn if the ffmpeg version constraint doesn't match the available ffmpeg version
		if c, err := semver.NewConstraint(t.config.FFVersion); err == nil {
			if v, err := semver.NewVersion(skills.FFmpeg.Version); err == nil {
				if !c.Check(v) {
					t.logger.Warn().WithFields(log.Fields{
						"constraint": t.config.FFVersion,
						"version":    skills.FFmpeg.Version,
					}).WithError(fmt.Errorf("available FFmpeg version doesn't fit constraint; you have to update this process to adjust the constraint")).Log("")
				}
			} else {
				t.logger.Warn().WithError(err).Log("")
			}
		} else {
			t.logger.Warn().WithError(err).Log("")
		}

		err := r.resolveAddresses(tasks, t.config)
		if err != nil {
			t.logger.Warn().WithError(err).Log("Ignoring")
			return true
		}

		// Validate config with all placeholders replaced. However, we need to take care
		// that the config with the task keeps its dynamic placeholders for process starts.
		config := t.config.Clone()
		resolveDynamicPlaceholder(config, r.replace, map[string]string{
			"hwdevice": "0",
		}, map[string]string{
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})

		t.usesDisk, err = validateConfig(config, r.fs.list, r.ffmpeg)
		if err != nil {
			t.logger.Warn().WithError(err).Log("Ignoring")
			return true
		}

		err = r.setPlayoutPorts(t)
		if err != nil {
			t.logger.Warn().WithError(err).Log("Ignoring")
			return true
		}

		t.command = t.config.CreateCommand()
		t.parser = r.ffmpeg.NewProcessParser(t.logger, t.String(), t.reference, t.config.LogPatterns)

		limitMode := "hard"
		if r.enableSoftLimit {
			limitMode = "soft"
		}

		ffmpeg, err := r.ffmpeg.New(ffmpeg.ProcessConfig{
			Binary:          t.config.Binary,
			Reconnect:       t.config.Reconnect,
			ReconnectDelay:  time.Duration(t.config.ReconnectDelay) * time.Second,
			StaleTimeout:    time.Duration(t.config.StaleTimeout) * time.Second,
			Timeout:         time.Duration(t.config.Timeout) * time.Second,
			LimitCPU:        t.config.LimitCPU,
			LimitMemory:     t.config.LimitMemory,
			LimitGPUUsage:   t.config.LimitGPU.Usage,
			LimitGPUEncoder: t.config.LimitGPU.Encoder,
			LimitGPUDecoder: t.config.LimitGPU.Decoder,
			LimitGPUMemory:  t.config.LimitGPU.Memory,
			LimitDuration:   time.Duration(t.config.LimitWaitFor) * time.Second,
			LimitMode:       limitMode,
			Scheduler:       t.config.Scheduler,
			Args:            t.command,
			Parser:          t.parser,
			Logger:          t.logger,
			OnBeforeStart:   r.onBeforeStart(t.config.Clone()),
		})
		if err != nil {
			return true
		}

		t.ffmpeg = ffmpeg
		t.SetValid(true)

		return true
	})

	r.tasks.Clear()
	r.tasks = tasks

	r.metadata = data.Metadata

	return nil
}

func (r *restream) save() {
	if r.store == nil {
		return
	}

	data := store.NewData()

	r.tasks.Range(true, func(tid app.ProcessID, t *task, token string) bool {
		defer t.Release(token)

		if !t.IsValid() {
			return true
		}

		domain := data.Process[tid.Domain]
		if domain == nil {
			domain = map[string]store.Process{}
		}

		domain[tid.ID] = store.Process{
			Process:  t.process.Clone(),
			Metadata: t.metadata,
		}

		data.Process[tid.Domain] = domain

		return true
	})

	data.Metadata = r.metadata

	r.store.Store(data)
}

func (r *restream) ID() string {
	return r.id
}

func (r *restream) Name() string {
	return r.name
}

func (r *restream) CreatedAt() time.Time {
	return r.createdAt
}

var ErrUnknownProcess = errors.New("unknown process")
var ErrUnknownProcessGroup = errors.New("unknown process group")
var ErrProcessExists = errors.New("process already exists")
var ErrForbidden = errors.New("forbidden")

func (r *restream) AddProcess(config *app.Config) error {
	t, err := r.createTask(config)
	if err != nil {
		return err
	}

	tid := t.ID()

	_, ok := r.tasks.LoadOrStore(tid, t)
	if ok {
		t.Destroy()
		return ErrProcessExists
	}

	// set filesystem cleanup rules
	r.setCleanup(tid, t.config)

	err = t.Restore()
	if err != nil {
		r.tasks.Delete(tid)
		t.Destroy()
		return err
	}

	r.save()

	return nil
}

// createTask creates a new task based on a process config.
func (r *restream) createTask(config *app.Config) (*task, error) {
	id := strings.TrimSpace(config.ID)

	if len(id) == 0 {
		return nil, fmt.Errorf("an empty ID is not allowed")
	}

	config.FFVersion = "^" + r.ffmpeg.Skills().FFmpeg.Version
	if v, err := semver.NewVersion(config.FFVersion); err == nil {
		// Remove the patch level for the constraint
		config.FFVersion = fmt.Sprintf("^%d.%d.0", v.Major(), v.Minor())
	}

	process := &app.Process{
		ID:        config.ID,
		Owner:     config.Owner,
		Domain:    config.Domain,
		Reference: config.Reference,
		Config:    config.Clone(),
		Order:     app.NewOrder("stop"),
		CreatedAt: time.Now().Unix(),
	}

	process.UpdatedAt = process.CreatedAt

	if config.Autostart {
		process.Order.Set("start")
	}

	t := NewTask(process, r.logger.WithFields(log.Fields{
		"id":        process.ID,
		"owner":     process.Owner,
		"reference": process.Reference,
		"domain":    process.Domain,
	}))

	resolveStaticPlaceholders(t.config, r.replace)

	err := r.resolveAddresses(r.tasks, t.config)
	if err != nil {
		return nil, err
	}

	{
		// Validate config with all placeholders replaced. However, we need to take care
		// that the config with the task keeps its dynamic placeholders for process starts.
		config := t.config.Clone()
		resolveDynamicPlaceholder(config, r.replace, map[string]string{
			"hwdevice": "0",
		}, map[string]string{
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})

		t.usesDisk, err = validateConfig(config, r.fs.list, r.ffmpeg)
		if err != nil {
			return nil, err
		}
	}

	err = r.setPlayoutPorts(t)
	if err != nil {
		return nil, err
	}

	t.command = t.config.CreateCommand()
	t.parser = r.ffmpeg.NewProcessParser(t.logger, t.String(), t.reference, t.config.LogPatterns)

	limitMode := "hard"
	if r.enableSoftLimit {
		limitMode = "soft"
	}

	ffmpeg, err := r.ffmpeg.New(ffmpeg.ProcessConfig{
		Binary:          t.config.Binary,
		Reconnect:       t.config.Reconnect,
		ReconnectDelay:  time.Duration(t.config.ReconnectDelay) * time.Second,
		StaleTimeout:    time.Duration(t.config.StaleTimeout) * time.Second,
		Timeout:         time.Duration(t.config.Timeout) * time.Second,
		LimitCPU:        t.config.LimitCPU,
		LimitMemory:     t.config.LimitMemory,
		LimitGPUUsage:   t.config.LimitGPU.Usage,
		LimitGPUEncoder: t.config.LimitGPU.Encoder,
		LimitGPUDecoder: t.config.LimitGPU.Decoder,
		LimitGPUMemory:  t.config.LimitGPU.Memory,
		LimitDuration:   time.Duration(t.config.LimitWaitFor) * time.Second,
		LimitMode:       limitMode,
		Scheduler:       t.config.Scheduler,
		Args:            t.command,
		Parser:          t.parser,
		Logger:          t.logger,
		OnBeforeStart:   r.onBeforeStart(t.config.Clone()),
	})
	if err != nil {
		return nil, err
	}

	t.ffmpeg = ffmpeg

	t.SetValid(true)

	return t, nil
}

// onBeforeStart is a callback that gets called by a process before it will be started.
// It evalutes the dynamic placeholders in a process config and returns the resulting command line to the process.
func (r *restream) onBeforeStart(cfg *app.Config) func([]string) ([]string, error) {
	return func(args []string) ([]string, error) {
		selectedGPU := -1
		if r.enableSoftLimit {
			res, err := r.resources.Request(resources.Request{
				CPU:        cfg.LimitCPU,
				Memory:     cfg.LimitMemory,
				GPUUsage:   cfg.LimitGPU.Usage,
				GPUEncoder: cfg.LimitGPU.Encoder,
				GPUDecoder: cfg.LimitGPU.Decoder,
				GPUMemory:  cfg.LimitGPU.Memory,
			})
			if err != nil {
				return []string{}, err
			}

			selectedGPU = res.GPU
		} else {
			selectedGPU = 0
		}

		if t, token, hasTask := r.tasks.Load(cfg.ProcessID()); hasTask {
			t.SetHWDevice(selectedGPU)
			t.Release(token)
		}

		config := cfg.Clone()

		resolveDynamicPlaceholder(config, r.replace, map[string]string{
			"hwdevice": fmt.Sprintf("%d", selectedGPU),
		}, map[string]string{
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})

		_, err := validateConfig(config, r.fs.list, r.ffmpeg)
		if err != nil {
			return []string{}, err
		}

		return config.CreateCommand(), nil
	}
}

func (r *restream) setCleanup(id app.ProcessID, config *app.Config) {
	for _, output := range config.Output {
		for _, c := range output.Cleanup {
			name, path, found := strings.Cut(c.Pattern, ":")
			if !found {
				r.logger.Warn().WithField("pattern", c.Pattern).Log("invalid pattern, no prefix")
				continue
			}

			if len(config.Domain) != 0 {
				path = "/" + filepath.Join(config.Domain, path)
			}

			// Support legacy names
			if name == "diskfs" {
				name = "disk"
			} else if name == "memfs" {
				name = "mem"
			}

			for _, fs := range r.fs.list {
				if fs.Name() != name {
					continue
				}

				pattern := rfs.Pattern{
					Pattern:       path,
					MaxFiles:      c.MaxFiles,
					MaxFileAge:    time.Duration(c.MaxFileAge) * time.Second,
					PurgeOnDelete: c.PurgeOnDelete,
				}

				fs.SetCleanup(id.String(), []rfs.Pattern{
					pattern,
				})

				break
			}
		}
	}
}

func (r *restream) unsetCleanup(id app.ProcessID) {
	for _, fs := range r.fs.list {
		fs.UnsetCleanup(id.String())
	}
}

func (r *restream) setPlayoutPorts(task *task) error {
	r.unsetPlayoutPorts(task)

	task.playout = make(map[string]int)

	for i, input := range task.config.Input {
		if !strings.HasPrefix(input.Address, "avstream:") && !strings.HasPrefix(input.Address, "playout:") {
			continue
		}

		options := []string{}
		skip := false

		for _, o := range input.Options {
			if skip {
				continue
			}

			if o == "-playout_httpport" {
				skip = true
				continue
			}

			options = append(options, o)
		}

		if port, err := r.ffmpeg.GetPort(); err == nil {
			options = append(options, "-playout_httpport", strconv.Itoa(port))

			task.logger.WithFields(log.Fields{
				"port":  port,
				"input": input.ID,
			}).Debug().Log("Assinging playout port")

			task.playout[input.ID] = port
		} else if err != net.ErrNoPortrangerProvided {
			return err
		}

		input.Options = options
		task.config.Input[i] = input
	}

	return nil
}

func (r *restream) unsetPlayoutPorts(t *task) {
	if t.playout == nil {
		return
	}

	for _, port := range t.playout {
		r.ffmpeg.PutPort(port)
	}

	t.playout = nil
}

// validateConfig verifies a process config, whether the accessed files (read and write) can be accessed
// based on the provided filesystems and the ffmpeg validators. Returns an error if somethingis wrong,
// otherwise nil and whether there is a disk filesystem involved.
func validateConfig(config *app.Config, fss []rfs.Filesystem, ffmpeg ffmpeg.FFmpeg) (bool, error) {
	if len(config.Input) == 0 {
		return false, fmt.Errorf("at least one input must be defined for the process '%s'", config.ID)
	}

	var err error

	ids := map[string]bool{}

	for _, io := range config.Input {
		io.ID = strings.TrimSpace(io.ID)

		if len(io.ID) == 0 {
			return false, fmt.Errorf("empty input IDs are not allowed (process '%s')", config.ID)
		}

		if _, found := ids[io.ID]; found {
			return false, fmt.Errorf("the input ID '%s' is already in use for the process `%s`", io.ID, config.ID)
		}

		ids[io.ID] = true

		io.Address = strings.TrimSpace(io.Address)

		if len(io.Address) == 0 {
			return false, fmt.Errorf("the address for input '#%s:%s' must not be empty", config.ID, io.ID)
		}

		maxFails := 0
		for _, fs := range fss {
			basedir := "/"
			if fs.Type() == "disk" {
				basedir = fs.Metadata("base")
			}

			io.Address, err = validateInputAddress(io.Address, basedir, ffmpeg)
			if err != nil {
				maxFails++
			}
		}

		if maxFails == len(fss) {
			return false, fmt.Errorf("the address for input '#%s:%s' (%s) is invalid: %w", config.ID, io.ID, io.Address, err)
		}
	}

	if len(config.Output) == 0 {
		return false, fmt.Errorf("at least one output must be defined for the process '#%s'", config.ID)
	}

	ids = map[string]bool{}
	hasFiles := false

	for _, io := range config.Output {
		io.ID = strings.TrimSpace(io.ID)

		if len(io.ID) == 0 {
			return false, fmt.Errorf("empty output IDs are not allowed (process '%s')", config.ID)
		}

		if _, found := ids[io.ID]; found {
			return false, fmt.Errorf("the output ID '%s' is already in use for the process `%s`", io.ID, config.ID)
		}

		ids[io.ID] = true

		io.Address = strings.TrimSpace(io.Address)

		if len(io.Address) == 0 {
			return false, fmt.Errorf("the address for output '#%s:%s' must not be empty", config.ID, io.ID)
		}

		maxFails := 0
		for _, fs := range fss {
			basedir := "/"
			if fs.Type() == "disk" {
				basedir = fs.Metadata("base")
			}

			isFile := false
			io.Address, isFile, err = validateOutputAddress(io.Address, basedir, ffmpeg)
			if err != nil {
				maxFails++
			}

			if isFile {
				if fs.Type() == "disk" {
					hasFiles = true
				}

				dir := filepath.Dir(strings.TrimPrefix(io.Address, "file:"+basedir))
				fs.MkdirAll(dir, 0755)
			}
		}

		if maxFails == len(fss) {
			return false, fmt.Errorf("the address for output '#%s:%s' is invalid: %w", config.ID, io.ID, err)
		}

	}

	return hasFiles, nil
}

// validateInputAddress checks whether the given input address is valid and is allowed to be used.
func validateInputAddress(address, _ string, ffmpeg ffmpeg.FFmpeg) (string, error) {
	if ok := url.HasScheme(address); ok {
		if err := url.Validate(address); err != nil {
			return address, err
		}
	}

	if !ffmpeg.ValidateInputAddress(address) {
		return address, fmt.Errorf("address is not allowed")
	}

	return address, nil
}

// validateOutputAddress checks whether the given output address is valid and is allowed to be used.
func validateOutputAddress(address, basedir string, ffmpeg ffmpeg.FFmpeg) (string, bool, error) {
	// If the address contains a "|" or it starts with a "[", then assume that it
	// is an address for the tee muxer.
	if strings.Contains(address, "|") || strings.HasPrefix(address, "[") {
		addresses := strings.Split(address, "|")

		isFile := false

		teeOptions := regexp.MustCompile(`^\[[^\]]*\]`)

		for i, a := range addresses {
			options := teeOptions.FindString(a)
			a = teeOptions.ReplaceAllString(a, "")

			va, file, err := validateOutputAddress(a, basedir, ffmpeg)
			if err != nil {
				return address, false, err
			}

			if file {
				isFile = true
			}

			addresses[i] = options + va
		}

		return strings.Join(addresses, "|"), isFile, nil
	}

	address = strings.TrimPrefix(address, "file:")

	if ok := url.HasScheme(address); ok {
		if err := url.Validate(address); err != nil {
			return address, false, err
		}

		if !ffmpeg.ValidateOutputAddress(address) {
			return address, false, fmt.Errorf("address is not allowed")
		}

		return address, false, nil
	}

	if address == "-" {
		return "pipe:", false, nil
	}

	address = filepath.Clean(address)

	if !filepath.IsAbs(address) {
		address = filepath.Join(basedir, address)
	}

	if strings.HasPrefix(address, "/dev/") {
		if !ffmpeg.ValidateOutputAddress("file:" + address) {
			return address, false, fmt.Errorf("address is not allowed")
		}

		return "file:" + address, false, nil
	}

	if !strings.HasPrefix(address, basedir) {
		return address, false, fmt.Errorf("%s is not inside of %s", address, basedir)
	}

	if !ffmpeg.ValidateOutputAddress("file:" + address) {
		return address, false, fmt.Errorf("address is not allowed")
	}

	return "file:" + address, true, nil
}

// resolveAddresses replaces the addresse reference from each input in a config with the actual address.
func (r *restream) resolveAddresses(tasks *Storage, config *app.Config) error {
	for i, input := range config.Input {
		// Resolve any references
		address, err := r.resolveAddress(tasks, config.ID, input.Address)
		if err != nil {
			return fmt.Errorf("reference error for '#%s:%s': %w", config.ID, input.ID, err)
		}

		input.Address = address

		config.Input[i] = input
	}

	return nil
}

// resolveAddress replaces the address reference with the actual address.
func (r *restream) resolveAddress(tasks *Storage, id, address string) (string, error) {
	matches, err := parseAddressReference(address)
	if err != nil {
		return address, err
	}

	// Address is not a reference
	if _, ok := matches["address"]; ok {
		return address, nil
	}

	if matches["id"] == id {
		return address, fmt.Errorf("self-reference is not allowed (%s)", address)
	}

	var t *task = nil
	var ttoken string = ""

	tasks.Range(true, func(_ app.ProcessID, task *task, token string) bool {
		if task.id == matches["id"] && task.domain == matches["domain"] {
			t = task
			ttoken = token
			return false
		}

		task.Release(token)
		return true
	})

	if t == nil {
		return address, fmt.Errorf("unknown process '%s' in domain '%s' (%s)", matches["id"], matches["domain"], address)
	}

	defer t.Release(ttoken)

	teeOptions := regexp.MustCompile(`^\[[^\]]*\]`)

	for _, x := range t.config.Output {
		if x.ID != matches["output"] {
			continue
		}

		// Check for non-tee output
		if !strings.Contains(x.Address, "|") && !strings.HasPrefix(x.Address, "[") {
			return r.rewrite.RewriteAddress(x.Address, t.config.Owner, rewrite.READ), nil
		}

		// Split tee output in its individual addresses

		addresses := strings.Split(x.Address, "|")
		if len(addresses) == 0 {
			return x.Address, nil
		}

		// Remove tee options
		for i, a := range addresses {
			addresses[i] = teeOptions.ReplaceAllString(a, "")
		}

		if len(matches["source"]) == 0 {
			return r.rewrite.RewriteAddress(addresses[0], t.config.Owner, rewrite.READ), nil
		}

		for _, a := range addresses {
			u, err := url.Parse(a)
			if err != nil {
				// Ignore invalid addresses
				continue
			}

			if matches["source"] == "hls" {
				if (u.Scheme == "http" || u.Scheme == "https") && strings.HasSuffix(u.RawPath, ".m3u8") {
					return r.rewrite.RewriteAddress(a, t.config.Owner, rewrite.READ), nil
				}
			} else if matches["source"] == "rtmp" {
				if u.Scheme == "rtmp" {
					return r.rewrite.RewriteAddress(a, t.config.Owner, rewrite.READ), nil
				}
			} else if matches["source"] == "srt" {
				if u.Scheme == "srt" {
					return r.rewrite.RewriteAddress(a, t.config.Owner, rewrite.READ), nil
				}
			}
		}

		// If none of the sources matched, return the first address
		return r.rewrite.RewriteAddress(addresses[0], t.config.Owner, rewrite.READ), nil
	}

	return address, fmt.Errorf("the process '%s' in group '%s' has no outputs with the ID '%s' (%s)", matches["id"], matches["group"], matches["output"], address)
}

func parseAddressReference(address string) (map[string]string, error) {
	if len(address) == 0 {
		return nil, fmt.Errorf("empty address")
	}

	if address[0] != '#' {
		return map[string]string{
			"address": address,
		}, nil
	}

	re := regexp.MustCompile(`:(output|domain|source)=(.+)`)

	results := map[string]string{}

	idEnd := -1
	value := address
	key := ""

	for {
		matches := re.FindStringSubmatchIndex(value)
		if matches == nil {
			break
		}

		if idEnd < 0 {
			idEnd = matches[2] - 1
		}

		if len(key) != 0 {
			results[key] = value[:matches[2]-1]
		}

		key = value[matches[2]:matches[3]]
		value = value[matches[4]:matches[5]]

		results[key] = value
	}

	if idEnd < 0 {
		return nil, fmt.Errorf("invalid format (%s)", address)
	}

	results["id"] = address[1:idEnd]

	return results, nil
}

func (r *restream) UpdateProcess(id app.ProcessID, config *app.Config) error {
	task, ok := r.tasks.LoadAndLock(id)
	if !ok {
		return ErrUnknownProcess
	}

	err := r.updateProcess(task, config)

	task.Unlock()

	if err != nil {
		return err
	}

	r.save()

	return nil
}

func (r *restream) updateProcess(task *task, config *app.Config) error {
	// If the new config has the same hash as the current config, do nothing.
	if task.Equal(config) {
		return nil
	}

	t, err := r.createTask(config)
	if err != nil {
		return err
	}

	tid := t.ID()

	if !tid.Equal(task.ID()) {
		if r.tasks.Has(tid) {
			t.Destroy()
			return ErrProcessExists
		}
	}

	order := task.Order()
	if len(order) == 0 {
		t.Destroy()
		return ErrUnknownProcess
	}

	t.process.Order.Set(order)

	if err := r.stopProcess(task); err != nil {
		t.Destroy()
		return fmt.Errorf("stop process: %w", err)
	}

	// This would require a major version jump
	//t.process.CreatedAt = task.process.CreatedAt

	// Transfer the report history to the new process
	t.ImportParserReportHistory(task.ExportParserReportHistory())

	// Transfer the metadata to the new process
	t.ImportMetadata(task.ExportMetadata())

	if err := r.deleteProcess(task); err != nil {
		t.Destroy()
		return fmt.Errorf("delete process: %w", err)
	}

	r.tasks.Store(tid, t)

	// set filesystem cleanup rules
	r.setCleanup(tid, t.config)

	t.Restore()

	return nil
}

func (r *restream) GetProcessIDs(idpattern, refpattern, ownerpattern, domainpattern string) []app.ProcessID {
	var idglob glob.Glob = nil
	var refglob glob.Glob = nil
	var ownerglob glob.Glob = nil
	var domainglob glob.Glob = nil

	if len(idpattern) != 0 {
		idglob, _ = glob.Compile(idpattern)
	}

	if len(refpattern) != 0 {
		refglob, _ = glob.Compile(refpattern)
	}

	if len(ownerpattern) != 0 {
		ownerglob, _ = glob.Compile(ownerpattern)
	}

	if len(domainpattern) != 0 {
		domainglob, _ = glob.Compile(domainpattern)
	}

	var ids []app.ProcessID

	if idglob == nil && refglob == nil && ownerglob == nil && domainglob == nil {
		ids = make([]app.ProcessID, 0, r.tasks.Size())

		r.tasks.Range(true, func(id app.ProcessID, t *task, token string) bool {
			defer t.Release(token)

			ids = append(ids, id)

			return true
		})
	} else {
		ids = []app.ProcessID{}

		r.tasks.Range(true, func(id app.ProcessID, t *task, token string) bool {
			defer t.Release(token)

			if !t.Match(idglob, refglob, ownerglob, domainglob) {
				return true
			}

			ids = append(ids, id)

			return true
		})
	}

	return ids
}

func (r *restream) GetProcess(id app.ProcessID) (*app.Process, error) {
	task, token, ok := r.tasks.Load(id)
	if !ok {
		return &app.Process{}, ErrUnknownProcess
	}
	defer task.Release(token)

	return task.Process(), nil
}

func (r *restream) DeleteProcess(id app.ProcessID) error {
	task, ok := r.tasks.LoadAndLock(id)
	if !ok {
		return ErrUnknownProcess
	}

	err := r.deleteProcess(task)

	task.Unlock()

	if err != nil {
		return err
	}

	r.save()

	return nil
}

func (r *restream) deleteProcess(task *task) error {
	if task.Order() != "stop" {
		return fmt.Errorf("the process with the ID '%s' is still running", task.String())
	}

	r.unsetPlayoutPorts(task)
	r.unsetCleanup(task.ID())

	r.tasks.Delete(task.ID())

	task.Destroy()

	return nil
}

func (r *restream) StartProcess(id app.ProcessID) error {
	task, token, ok := r.tasks.Load(id)
	if !ok {
		return ErrUnknownProcess
	}

	err := r.startProcess(task)

	task.Release(token)

	if err != nil {
		return err
	}

	r.save()

	return nil
}

func (r *restream) startProcess(task *task) error {
	err := task.Start()
	if err != nil {
		return err
	}

	r.nProc++

	return nil
}

func (r *restream) StopProcess(id app.ProcessID) error {
	task, token, ok := r.tasks.Load(id)
	if !ok {
		return ErrUnknownProcess
	}

	err := r.stopProcess(task)

	task.Release(token)

	if err != nil {
		return err
	}

	r.save()

	return nil
}

func (r *restream) stopProcess(task *task) error {
	// TODO: aufpassen mit nProc und nil error. In task.Stop() noch einen error einführen, falls der process nicht läuft.
	err := task.Stop()
	if err != nil {
		return err
	}

	r.nProc--

	return nil
}

func (r *restream) RestartProcess(id app.ProcessID) error {
	task, token, ok := r.tasks.Load(id)
	if !ok {
		return ErrUnknownProcess
	}
	defer task.Release(token)

	return r.restartProcess(task)
}

func (r *restream) restartProcess(task *task) error {
	task.Restart()

	return nil
}

func (r *restream) ReloadProcess(id app.ProcessID) error {
	task, ok := r.tasks.LoadAndLock(id)
	if !ok {
		return ErrUnknownProcess
	}

	err := r.reloadProcess(task)

	task.Unlock()

	if err != nil {
		return err
	}

	r.save()

	return nil
}

func (r *restream) reloadProcess(task *task) error {
	t, err := r.createTask(task.Config())
	if err != nil {
		return err
	}

	tid := t.ID()

	order := task.Order()
	if len(order) == 0 {
		t.Destroy()
		return ErrUnknownProcess
	}

	t.process.Order.Set(order)

	if err := task.Stop(); err != nil {
		t.Destroy()
		return fmt.Errorf("stop process: %w", err)
	}

	// Transfer the report history to the new process
	t.parser.ImportReportHistory(task.parser.ReportHistory())

	// Transfer the metadata to the new process
	t.metadata = task.metadata

	if err := r.deleteProcess(task); err != nil {
		t.Destroy()
		return fmt.Errorf("delete process: %w", err)
	}

	r.tasks.Store(tid, t)

	// set filesystem cleanup rules
	r.setCleanup(tid, t.config)

	t.Restore()

	return nil
}

func (r *restream) GetProcessState(id app.ProcessID) (*app.State, error) {
	state := &app.State{}

	task, token, ok := r.tasks.Load(id)
	if !ok {
		return state, ErrUnknownProcess
	}
	defer task.Release(token)

	return task.State()
}

func (r *restream) GetProcessReport(id app.ProcessID) (*app.Report, error) {
	report := &app.Report{}

	task, token, ok := r.tasks.Load(id)
	if !ok {
		return report, ErrUnknownProcess
	}
	defer task.Release(token)

	return task.Report()
}

func (r *restream) SetProcessReport(id app.ProcessID, report *app.Report) error {
	task, ok := r.tasks.LoadAndLock(id)
	if !ok {
		return ErrUnknownProcess
	}
	defer task.Unlock()

	return task.SetReport(report)
}

func (r *restream) SearchProcessLogHistory(idpattern, refpattern, state string, from, to *time.Time) []app.ReportHistorySearchResult {
	result := []app.ReportHistorySearchResult{}

	ids := r.GetProcessIDs(idpattern, refpattern, "", "")

	for _, id := range ids {
		task, token, ok := r.tasks.Load(id)
		if !ok {
			continue
		}

		presult := task.SearchReportHistory(state, from, to)
		result = append(result, presult...)

		task.Release(token)
	}

	return result
}

func (r *restream) Probe(config *app.Config, timeout time.Duration) app.Probe {
	probe := app.Probe{}

	config = config.Clone()

	resolveStaticPlaceholders(config, r.replace)

	err := r.resolveAddresses(r.tasks, config)
	if err != nil {
		probe.Log = append(probe.Log, err.Error())
		return probe
	}

	resolveDynamicPlaceholder(config, r.replace, map[string]string{
		"hwdevice": "0",
	}, map[string]string{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})

	_, err = validateConfig(config, r.fs.list, r.ffmpeg)
	if err != nil {
		probe.Log = append(probe.Log, err.Error())
		return probe
	}

	var command []string

	// Copy global options
	command = append(command, config.Options...)

	for _, input := range config.Input {
		// Add the resolved input to the process command
		command = append(command, input.Options...)
		command = append(command, "-i", input.Address)
	}

	logbuffer := log.NewBufferWriter(log.Ldebug, 1000)
	logger := log.New("").WithOutput(logbuffer)

	prober := r.ffmpeg.NewProbeParser(logger)

	var wg sync.WaitGroup

	wg.Add(1)

	ffmpeg, err := r.ffmpeg.New(ffmpeg.ProcessConfig{
		Reconnect:      false,
		ReconnectDelay: 0,
		StaleTimeout:   timeout,
		Args:           command,
		Parser:         prober,
		Logger:         logger,
		OnExit: func(string) {
			wg.Done()
		},
	})

	if err != nil {
		formatter := log.NewConsoleFormatter(false)

		for _, e := range logbuffer.Events() {
			probe.Log = append(probe.Log, strings.TrimSpace(formatter.String(e)))
		}

		probe.Log = append(probe.Log, err.Error())

		return probe
	}

	ffmpeg.Start()

	wg.Wait()

	p := prober.Probe()
	probe.UnmarshalProber(&p)

	return probe
}

func (r *restream) Skills() skills.Skills {
	return r.ffmpeg.Skills()
}

func (r *restream) ReloadSkills() error {
	return r.ffmpeg.ReloadSkills()
}

func (r *restream) GetPlayout(id app.ProcessID, inputid string) (string, error) {
	task, token, ok := r.tasks.Load(id)
	if !ok {
		return "", ErrUnknownProcess
	}
	defer task.Release(token)

	if !task.IsValid() {
		return "", fmt.Errorf("invalid process definition")
	}

	port, ok := task.playout[inputid]
	if !ok {
		return "", fmt.Errorf("no playout for input ID '%s' and process '%s'", inputid, id)
	}

	return "127.0.0.1:" + strconv.Itoa(port), nil
}

func (r *restream) SetProcessMetadata(id app.ProcessID, key string, data interface{}) error {
	task, ok := r.tasks.LoadAndLock(id)
	if !ok {
		return ErrUnknownProcess
	}

	err := task.SetMetadata(key, data)

	task.Unlock()

	if err != nil {
		return err
	}

	r.save()

	return nil
}

func (r *restream) GetProcessMetadata(id app.ProcessID, key string) (interface{}, error) {
	task, token, ok := r.tasks.Load(id)
	if !ok {
		return nil, ErrUnknownProcess
	}
	defer task.Release(token)

	return task.GetMetadata(key)
}

func (r *restream) SetMetadata(key string, data interface{}) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	if len(key) == 0 {
		return fmt.Errorf("a key for storing the data has to be provided")
	}

	if r.metadata == nil {
		r.metadata = make(map[string]interface{})
	}

	if data == nil {
		delete(r.metadata, key)
	} else {
		r.metadata[key] = data
	}

	if len(r.metadata) == 0 {
		r.metadata = nil
	}

	r.save()

	return nil
}

func (r *restream) GetMetadata(key string) (interface{}, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	if len(key) == 0 {
		if len(r.metadata) == 0 {
			return nil, nil
		}

		return r.metadata, nil
	}

	data, ok := r.metadata[key]
	if !ok {
		return nil, ErrMetadataKeyNotFound
	}

	return data, nil
}

// resolveStaticPlaceholders replaces all placeholders in the config. The config will be modified in place.
func resolveStaticPlaceholders(config *app.Config, r replace.Replacer) {
	vars := map[string]string{
		"processid": config.ID,
		"owner":     config.Owner,
		"reference": config.Reference,
		"domain":    config.Domain,
	}

	for i, option := range config.Options {
		// Replace any known placeholders
		option = r.Replace(option, "diskfs", "", vars, config, "global")
		option = r.Replace(option, "fs:*", "", vars, config, "global")

		config.Options[i] = option
	}

	// Resolving the given inputs
	for i, input := range config.Input {
		// Replace any known placeholders
		input.ID = r.Replace(input.ID, "processid", config.ID, vars, config, "input")
		input.ID = r.Replace(input.ID, "domain", config.Domain, vars, config, "input")
		input.ID = r.Replace(input.ID, "reference", config.Reference, vars, config, "input")

		vars["inputid"] = input.ID

		input.Address = r.Replace(input.Address, "inputid", input.ID, vars, config, "input")
		input.Address = r.Replace(input.Address, "processid", config.ID, vars, config, "input")
		input.Address = r.Replace(input.Address, "domain", config.Domain, vars, config, "input")
		input.Address = r.Replace(input.Address, "reference", config.Reference, vars, config, "input")
		input.Address = r.Replace(input.Address, "diskfs", "", vars, config, "input")
		input.Address = r.Replace(input.Address, "memfs", "", vars, config, "input")
		input.Address = r.Replace(input.Address, "fs:*", "", vars, config, "input")
		input.Address = r.Replace(input.Address, "rtmp", "", vars, config, "input")
		input.Address = r.Replace(input.Address, "srt", "", vars, config, "input")

		for j, option := range input.Options {
			// Replace any known placeholders
			option = r.Replace(option, "inputid", input.ID, vars, config, "input")
			option = r.Replace(option, "processid", config.ID, vars, config, "input")
			option = r.Replace(option, "domain", config.Domain, vars, config, "input")
			option = r.Replace(option, "reference", config.Reference, vars, config, "input")
			option = r.Replace(option, "diskfs", "", vars, config, "input")
			option = r.Replace(option, "memfs", "", vars, config, "input")
			option = r.Replace(option, "fs:*", "", vars, config, "input")

			input.Options[j] = option
		}

		delete(vars, "inputid")

		config.Input[i] = input
	}

	// Resolving the given outputs
	for i, output := range config.Output {
		// Replace any known placeholders
		output.ID = r.Replace(output.ID, "processid", config.ID, vars, config, "output")
		output.ID = r.Replace(output.ID, "domain", config.Domain, vars, config, "output")
		output.ID = r.Replace(output.ID, "reference", config.Reference, vars, config, "output")

		vars["outputid"] = output.ID

		output.Address = r.Replace(output.Address, "outputid", output.ID, vars, config, "output")
		output.Address = r.Replace(output.Address, "processid", config.ID, vars, config, "output")
		output.Address = r.Replace(output.Address, "domain", config.Domain, vars, config, "output")
		output.Address = r.Replace(output.Address, "reference", config.Reference, vars, config, "output")
		output.Address = r.Replace(output.Address, "diskfs", "", vars, config, "output")
		output.Address = r.Replace(output.Address, "memfs", "", vars, config, "output")
		output.Address = r.Replace(output.Address, "fs:*", "", vars, config, "output")
		output.Address = r.Replace(output.Address, "rtmp", "", vars, config, "output")
		output.Address = r.Replace(output.Address, "srt", "", vars, config, "output")

		for j, option := range output.Options {
			// Replace any known placeholders
			option = r.Replace(option, "outputid", output.ID, vars, config, "output")
			option = r.Replace(option, "processid", config.ID, vars, config, "output")
			option = r.Replace(option, "domain", config.Domain, vars, config, "output")
			option = r.Replace(option, "reference", config.Reference, vars, config, "output")
			option = r.Replace(option, "diskfs", "", vars, config, "output")
			option = r.Replace(option, "memfs", "", vars, config, "output")
			option = r.Replace(option, "fs:*", "", vars, config, "output")

			output.Options[j] = option
		}

		for j, cleanup := range output.Cleanup {
			// Replace any known placeholders
			cleanup.Pattern = r.Replace(cleanup.Pattern, "outputid", output.ID, vars, config, "output")
			cleanup.Pattern = r.Replace(cleanup.Pattern, "processid", config.ID, vars, config, "output")
			cleanup.Pattern = r.Replace(cleanup.Pattern, "domain", config.Domain, vars, config, "output")
			cleanup.Pattern = r.Replace(cleanup.Pattern, "reference", config.Reference, vars, config, "output")

			output.Cleanup[j] = cleanup
		}

		delete(vars, "outputid")

		config.Output[i] = output
	}
}

// resolveDynamicPlaceholder replaces placeholders in the config that should be replaced at process start.
// The config will be modified in place.
func resolveDynamicPlaceholder(config *app.Config, r replace.Replacer, values map[string]string, vars map[string]string) {
	placeholders := []string{"date", "hwdevice"}

	for i, option := range config.Options {
		for _, placeholder := range placeholders {
			option = r.Replace(option, placeholder, values[placeholder], vars, config, "global")
		}

		config.Options[i] = option
	}

	for i, input := range config.Input {
		for _, placeholder := range placeholders {
			input.Address = r.Replace(input.Address, placeholder, values[placeholder], vars, config, "input")
		}

		for j, option := range input.Options {
			for _, placeholder := range placeholders {
				option = r.Replace(option, placeholder, values[placeholder], vars, config, "input")
			}

			input.Options[j] = option
		}

		config.Input[i] = input
	}

	for i, output := range config.Output {
		for _, placeholder := range placeholders {
			output.Address = r.Replace(output.Address, placeholder, values[placeholder], vars, config, "output")
		}

		for j, option := range output.Options {
			for _, placeholder := range placeholders {
				option = r.Replace(option, placeholder, values[placeholder], vars, config, "output")
			}

			output.Options[j] = option
		}

		for j, cleanup := range output.Cleanup {
			for _, placeholder := range placeholders {
				cleanup.Pattern = r.Replace(cleanup.Pattern, placeholder, values[placeholder], vars, config, "output")
			}

			output.Cleanup[j] = cleanup
		}

		config.Output[i] = output
	}
}
