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
	"github.com/datarhei/core/v16/ffmpeg/parse"
	"github.com/datarhei/core/v16/ffmpeg/probe"
	"github.com/datarhei/core/v16/ffmpeg/skills"
	"github.com/datarhei/core/v16/glob"
	"github.com/datarhei/core/v16/iam"
	"github.com/datarhei/core/v16/io/fs"
	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/net"
	"github.com/datarhei/core/v16/net/url"
	"github.com/datarhei/core/v16/process"
	"github.com/datarhei/core/v16/restream/app"
	rfs "github.com/datarhei/core/v16/restream/fs"
	"github.com/datarhei/core/v16/restream/replace"
	"github.com/datarhei/core/v16/restream/resources"
	"github.com/datarhei/core/v16/restream/rewrite"
	"github.com/datarhei/core/v16/restream/store"
	jsonstore "github.com/datarhei/core/v16/restream/store/json"

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

	AddProcess(config *app.Config) error                                                                           // Add a new process
	GetProcessIDs(idpattern, refpattern, ownerpattern, domainpattern string) []TaskID                              // Get a list of process IDs based on patterns for ID and reference
	DeleteProcess(id TaskID) error                                                                                 // Delete a process
	UpdateProcess(id TaskID, config *app.Config) error                                                             // Update a process
	StartProcess(id TaskID) error                                                                                  // Start a process
	StopProcess(id TaskID) error                                                                                   // Stop a process
	RestartProcess(id TaskID) error                                                                                // Restart a process
	ReloadProcess(id TaskID) error                                                                                 // Reload a process
	GetProcess(id TaskID) (*app.Process, error)                                                                    // Get a process
	GetProcessState(id TaskID) (*app.State, error)                                                                 // Get the state of a process
	GetProcessLog(id TaskID) (*app.Log, error)                                                                     // Get the logs of a process
	SearchProcessLogHistory(idpattern, refpattern, state string, from, to *time.Time) []app.LogHistorySearchResult // Search the log history of all processes
	GetPlayout(id TaskID, inputid string) (string, error)                                                          // Get the URL of the playout API for a process
	Probe(id TaskID) app.Probe                                                                                     // Probe a process
	ProbeWithTimeout(id TaskID, timeout time.Duration) app.Probe                                                   // Probe a process with specific timeout
	SetProcessMetadata(id TaskID, key string, data interface{}) error                                              // Set metatdata to a process
	GetProcessMetadata(id TaskID, key string) (interface{}, error)                                                 // Get previously set metadata from a process
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
	MaxCPU       float64 // percent 0-100
	MaxMemory    float64 // percent 0-100
	Logger       log.Logger
	IAM          iam.IAM
}

type task struct {
	valid     bool
	id        string // ID of the task/process
	owner     string
	domain    string
	reference string
	process   *app.Process
	config    *app.Config
	command   []string // The actual command parameter for ffmpeg
	ffmpeg    process.Process
	parser    parse.Parser
	playout   map[string]int
	logger    log.Logger
	usesDisk  bool // Whether this task uses the disk
	metadata  map[string]interface{}
}

func (t *task) ID() TaskID {
	return TaskID{
		ID:     t.id,
		Domain: t.domain,
	}
}

func (t *task) String() string {
	return t.ID().String()
}

type TaskID struct {
	ID     string
	Domain string
}

func (t TaskID) String() string {
	return t.ID + "@" + t.Domain
}

func (t TaskID) Equals(b TaskID) bool {
	if t.ID == b.ID && t.Domain == b.Domain {
		return true
	}

	return false
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
	tasks    map[TaskID]*task       // domain:processid
	metadata map[string]interface{} // global metadata
	logger   log.Logger

	resources       resources.Resources
	enableSoftLimit bool

	lock sync.RWMutex

	cancelObserver context.CancelFunc

	startOnce sync.Once
	stopOnce  sync.Once

	iam iam.IAM
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
		iam:       config.IAM,
	}

	if r.logger == nil {
		r.logger = log.New("")
	}

	if r.iam == nil {
		return nil, fmt.Errorf("missing IAM")
	}

	if r.store == nil {
		dummyfs, _ := fs.NewMemFilesystem(fs.MemConfig{})
		s, err := jsonstore.New(jsonstore.Config{
			Filesystem: dummyfs,
		})
		if err != nil {
			return nil, err
		}
		r.store = s
	}

	if len(config.Filesystems) == 0 {
		return nil, fmt.Errorf("at least one filesystem must be provided")
	}

	for _, fs := range config.Filesystems {
		fs := rfs.New(rfs.Config{
			FS:     fs,
			Logger: r.logger.WithComponent("Cleanup"),
		})

		r.fs.list = append(r.fs.list, fs)
	}

	if r.replace == nil {
		r.replace = replace.New()
	}

	r.ffmpeg = config.FFmpeg
	if r.ffmpeg == nil {
		return nil, fmt.Errorf("ffmpeg must be provided")
	}

	r.maxProc = config.MaxProcesses

	if config.MaxCPU > 0 || config.MaxMemory > 0 {
		resources, err := resources.New(resources.Config{
			MaxCPU:    config.MaxCPU,
			MaxMemory: config.MaxMemory,
			Logger:    r.logger.WithComponent("Resources"),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to initialize resource manager: %w", err)
		}
		r.resources = resources
		r.enableSoftLimit = true

		r.logger.Debug().Log("Enabling resource manager")
	}

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
			go r.resourceObserver(ctx, r.resources)
		}

		for id, t := range r.tasks {
			if t.process.Order == "start" {
				r.startProcess(id)
			}

			// The filesystem cleanup rules can be set
			r.setCleanup(id, t.config)
		}

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

		// Stop the currently running processes without altering their order such that on a subsequent
		// Start() they will get restarted.
		for id, t := range r.tasks {
			if t.ffmpeg != nil {
				t.ffmpeg.Stop(true)
			}

			r.unsetCleanup(id)
		}

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
				r.lock.Lock()
				for id, t := range r.tasks {
					if !t.valid {
						continue
					}

					if !t.usesDisk {
						continue
					}

					if t.process.Order != "start" {
						continue
					}

					r.logger.Warn().Log("Shutting down because filesystem is full")
					r.stopProcess(id)
				}
				r.lock.Unlock()
			}
		}
	}
}

func (r *restream) resourceObserver(ctx context.Context, rsc resources.Resources) {
	rsc.Start()
	defer rsc.Stop()

	limitCPU, limitMemory := false, false

	for {
		select {
		case <-ctx.Done():
			return
		case limitCPU = <-rsc.LimitCPU():
			r.lock.Lock()
			for id, t := range r.tasks {
				if !t.valid {
					continue
				}

				r.logger.Debug().WithFields(log.Fields{
					"limit_cpu": limitCPU,
					"id":        id,
				}).Log("Limiting process CPU consumption")
				t.ffmpeg.Limit(limitCPU, limitMemory)
			}
			r.lock.Unlock()
		case limitMemory = <-rsc.LimitMemory():
			r.lock.Lock()
			for id, t := range r.tasks {
				if !t.valid {
					continue
				}

				r.logger.Debug().WithFields(log.Fields{
					"limit_memory": limitMemory,
					"id":           id,
				}).Log("Limiting process memory consumption")
				t.ffmpeg.Limit(limitCPU, limitMemory)
			}
			r.lock.Unlock()
		}
	}
}

func (r *restream) load() error {
	data, err := r.store.Load()
	if err != nil {
		return err
	}

	tasks := make(map[TaskID]*task)

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

			t := &task{
				id:        p.Process.ID,
				owner:     p.Process.Owner,
				domain:    p.Process.Domain,
				reference: p.Process.Reference,
				process:   p.Process,
				config:    p.Process.Config.Clone(),
				logger: r.logger.WithFields(log.Fields{
					"id":     p.Process.ID,
					"owner":  p.Process.Owner,
					"domain": p.Process.Domain,
				}),
			}

			t.metadata = p.Metadata

			// Replace all placeholders in the config
			resolveStaticPlaceholders(t.config, r.replace)

			tasks[t.ID()] = t
		}
	}

	// Now that all tasks are defined and all placeholders are
	// replaced, we can resolve references and validate the
	// inputs and outputs.
	for _, t := range tasks {
		t := t

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
			continue
		}

		// Validate config with all placeholders replaced. However, we need to take care
		// that the config with the task keeps its dynamic placeholders for process starts.
		config := t.config.Clone()
		resolveDynamicPlaceholder(config, r.replace)

		t.usesDisk, err = validateConfig(config, r.fs.list, r.ffmpeg)
		if err != nil {
			t.logger.Warn().WithError(err).Log("Ignoring")
			continue
		}

		err = r.setPlayoutPorts(t)
		if err != nil {
			t.logger.Warn().WithError(err).Log("Ignoring")
			continue
		}

		t.command = t.config.CreateCommand()
		t.parser = r.ffmpeg.NewProcessParser(t.logger, t.String(), t.reference, t.config.LogPatterns)

		limitMode := "hard"
		if r.enableSoftLimit {
			limitMode = "soft"
		}

		ffmpeg, err := r.ffmpeg.New(ffmpeg.ProcessConfig{
			Reconnect:      t.config.Reconnect,
			ReconnectDelay: time.Duration(t.config.ReconnectDelay) * time.Second,
			StaleTimeout:   time.Duration(t.config.StaleTimeout) * time.Second,
			Timeout:        time.Duration(t.config.Timeout) * time.Second,
			LimitCPU:       t.config.LimitCPU,
			LimitMemory:    t.config.LimitMemory,
			LimitDuration:  time.Duration(t.config.LimitWaitFor) * time.Second,
			LimitMode:      limitMode,
			Scheduler:      t.config.Scheduler,
			Args:           t.command,
			Parser:         t.parser,
			Logger:         t.logger,
			OnArgs:         r.onArgs(t.config.Clone()),
			OnBeforeStart: func() error {
				if !r.enableSoftLimit {
					return nil
				}

				if err := r.resources.Request(t.config.LimitCPU, t.config.LimitMemory); err != nil {
					return err
				}

				return nil
			},
		})
		if err != nil {
			return err
		}

		t.ffmpeg = ffmpeg
		t.valid = true
	}

	r.tasks = tasks
	r.metadata = data.Metadata

	return nil
}

func (r *restream) save() {
	data := store.NewData()

	for tid, t := range r.tasks {
		domain := data.Process[tid.Domain]
		if domain == nil {
			domain = map[string]store.Process{}
		}

		domain[tid.ID] = store.Process{
			Process:  t.process.Clone(),
			Metadata: t.metadata,
		}

		data.Process[tid.Domain] = domain
	}

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
	r.lock.RLock()
	t, err := r.createTask(config)
	r.lock.RUnlock()

	if err != nil {
		return err
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	tid := t.ID()

	_, ok := r.tasks[tid]
	if ok {
		return ErrProcessExists
	}

	r.tasks[tid] = t

	// set filesystem cleanup rules
	r.setCleanup(tid, t.config)

	if t.process.Order == "start" {
		err := r.startProcess(tid)
		if err != nil {
			delete(r.tasks, tid)
			return err
		}
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
		Domain:    config.Domain,
		Reference: config.Reference,
		Config:    config.Clone(),
		Order:     "stop",
		CreatedAt: time.Now().Unix(),
	}

	process.UpdatedAt = process.CreatedAt

	if config.Autostart {
		process.Order = "start"
	}

	t := &task{
		id:        config.ID,
		domain:    config.Domain,
		reference: process.Reference,
		process:   process,
		config:    process.Config.Clone(),
		logger: r.logger.WithFields(log.Fields{
			"id":        process.ID,
			"reference": process.Reference,
			"domain":    process.Domain,
		}),
	}

	resolveStaticPlaceholders(t.config, r.replace)

	err := r.resolveAddresses(r.tasks, t.config)
	if err != nil {
		return nil, err
	}

	{
		// Validate config with all placeholders replaced. However, we need to take care
		// that the config with the task keeps its dynamic placeholders for process starts.
		config := t.config.Clone()
		resolveDynamicPlaceholder(config, r.replace)

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
		Reconnect:      t.config.Reconnect,
		ReconnectDelay: time.Duration(t.config.ReconnectDelay) * time.Second,
		StaleTimeout:   time.Duration(t.config.StaleTimeout) * time.Second,
		Timeout:        time.Duration(t.config.Timeout) * time.Second,
		LimitCPU:       t.config.LimitCPU,
		LimitMemory:    t.config.LimitMemory,
		LimitDuration:  time.Duration(t.config.LimitWaitFor) * time.Second,
		LimitMode:      limitMode,
		Scheduler:      t.config.Scheduler,
		Args:           t.command,
		Parser:         t.parser,
		Logger:         t.logger,
		OnArgs:         r.onArgs(t.config.Clone()),
		OnBeforeStart: func() error {
			if !r.enableSoftLimit {
				return nil
			}

			if err := r.resources.Request(t.config.LimitCPU, t.config.LimitMemory); err != nil {
				return err
			}

			return nil
		},
	})
	if err != nil {
		return nil, err
	}

	t.ffmpeg = ffmpeg

	t.valid = true

	return t, nil
}

// onArgs is a callback that gets called by a process before it will be started.
// It evalutes the dynamic placeholders in a process config and returns the
// resulting command line to the process.
func (r *restream) onArgs(cfg *app.Config) func([]string) []string {
	return func(args []string) []string {
		config := cfg.Clone()

		resolveDynamicPlaceholder(config, r.replace)

		_, err := validateConfig(config, r.fs.list, r.ffmpeg)
		if err != nil {
			return []string{}
		}

		return config.CreateCommand()
	}
}

func (r *restream) setCleanup(id TaskID, config *app.Config) {
	rePrefix := regexp.MustCompile(`^([a-z]+):`)

	for _, output := range config.Output {
		for _, c := range output.Cleanup {
			// TODO: strings.Cut(c.Pattern, ":")
			matches := rePrefix.FindStringSubmatch(c.Pattern)
			if matches == nil {
				continue
			}

			name := matches[1]

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
					Pattern:       rePrefix.ReplaceAllString(c.Pattern, ""),
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

func (r *restream) unsetCleanup(id TaskID) {
	for _, fs := range r.fs.list {
		fs.UnsetCleanup(id.String())
	}
}

func (r *restream) setPlayoutPorts(t *task) error {
	r.unsetPlayoutPorts(t)

	t.playout = make(map[string]int)

	for i, input := range t.config.Input {
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

			t.logger.WithFields(log.Fields{
				"port":  port,
				"input": input.ID,
			}).Debug().Log("Assinging playout port")

			t.playout[input.ID] = port
		} else if err != net.ErrNoPortrangerProvided {
			return err
		}

		input.Options = options
		t.config.Input[i] = input
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
				fs.MkdirAll(dir, 0744)
			}
		}

		if maxFails == len(fss) {
			return false, fmt.Errorf("the address for output '#%s:%s' is invalid: %w", config.ID, io.ID, err)
		}

	}

	return hasFiles, nil
}

// validateInputAddress checks whether the given input address is valid and is allowed to be used.
func validateInputAddress(address, basedir string, ffmpeg ffmpeg.FFmpeg) (string, error) {
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
func (r *restream) resolveAddresses(tasks map[TaskID]*task, config *app.Config) error {
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
func (r *restream) resolveAddress(tasks map[TaskID]*task, id, address string) (string, error) {
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

	for _, tsk := range tasks {
		if tsk.id == matches["id"] && tsk.domain == matches["domain"] {
			t = tsk
			break
		}
	}

	if t == nil {
		return address, fmt.Errorf("unknown process '%s' in domain '%s' (%s)", matches["id"], matches["domain"], address)
	}

	identity, _ := r.iam.GetVerifier(t.config.Owner)

	teeOptions := regexp.MustCompile(`^\[[^\]]*\]`)

	for _, x := range t.config.Output {
		if x.ID != matches["output"] {
			continue
		}

		// Check for non-tee output
		if !strings.Contains(x.Address, "|") && !strings.HasPrefix(x.Address, "[") {
			return r.rewrite.RewriteAddress(x.Address, identity, rewrite.READ), nil
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
			return r.rewrite.RewriteAddress(addresses[0], identity, rewrite.READ), nil
		}

		for _, a := range addresses {
			u, err := url.Parse(a)
			if err != nil {
				// Ignore invalid addresses
				continue
			}

			if matches["source"] == "hls" {
				if (u.Scheme == "http" || u.Scheme == "https") && strings.HasSuffix(u.RawPath, ".m3u8") {
					return r.rewrite.RewriteAddress(a, identity, rewrite.READ), nil
				}
			} else if matches["source"] == "rtmp" {
				if u.Scheme == "rtmp" {
					return r.rewrite.RewriteAddress(a, identity, rewrite.READ), nil
				}
			} else if matches["source"] == "srt" {
				if u.Scheme == "srt" {
					return r.rewrite.RewriteAddress(a, identity, rewrite.READ), nil
				}
			}
		}

		// If none of the sources matched, return the first address
		return r.rewrite.RewriteAddress(addresses[0], identity, rewrite.READ), nil
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

func (r *restream) UpdateProcess(id TaskID, config *app.Config) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	task, ok := r.tasks[id]
	if !ok {
		return ErrUnknownProcess
	}

	t, err := r.createTask(config)
	if err != nil {
		return err
	}

	tid := t.ID()

	if !tid.Equals(id) {
		_, ok := r.tasks[tid]
		if ok {
			return ErrProcessExists
		}
	}

	t.process.Order = task.process.Order

	if err := r.stopProcess(id); err != nil {
		return fmt.Errorf("stop process: %w", err)
	}

	if err := r.deleteProcess(id); err != nil {
		return fmt.Errorf("delete process: %w", err)
	}

	// This would require a major version jump
	//t.process.CreatedAt = task.process.CreatedAt
	t.process.UpdatedAt = time.Now().Unix()
	task.parser.TransferReportHistory(t.parser)

	r.tasks[tid] = t

	// set filesystem cleanup rules
	r.setCleanup(tid, t.config)

	if t.process.Order == "start" {
		r.startProcess(tid)
	}

	r.save()

	return nil
}

func (r *restream) GetProcessIDs(idpattern, refpattern, ownerpattern, domainpattern string) []TaskID {
	r.lock.RLock()
	defer r.lock.RUnlock()

	ids := []TaskID{}

	for _, t := range r.tasks {
		count := 0
		matches := 0
		if len(idpattern) != 0 {
			count++
			match, err := glob.Match(idpattern, t.id)
			if err != nil {
				return nil
			}

			if match {
				matches++
			}
		}

		if len(refpattern) != 0 {
			count++
			match, err := glob.Match(refpattern, t.reference)
			if err != nil {
				return nil
			}

			if match {
				matches++
			}
		}

		if len(ownerpattern) != 0 {
			count++
			match, err := glob.Match(ownerpattern, t.owner)
			if err != nil {
				return nil
			}

			if match {
				matches++
			}
		}

		if len(domainpattern) != 0 {
			count++
			match, err := glob.Match(domainpattern, t.domain)
			if err != nil {
				return nil
			}

			if match {
				matches++
			}
		}

		if count != matches {
			continue
		}

		tid := TaskID{
			ID:     t.id,
			Domain: t.domain,
		}

		ids = append(ids, tid)
	}

	return ids
}

func (r *restream) GetProcess(id TaskID) (*app.Process, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	task, ok := r.tasks[id]
	if !ok {
		return &app.Process{}, ErrUnknownProcess
	}

	process := task.process.Clone()

	return process, nil
}

func (r *restream) DeleteProcess(id TaskID) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	err := r.deleteProcess(id)
	if err != nil {
		return err
	}

	r.save()

	return nil
}

func (r *restream) deleteProcess(tid TaskID) error {
	task, ok := r.tasks[tid]
	if !ok {
		return ErrUnknownProcess
	}

	if task.process.Order != "stop" {
		return fmt.Errorf("the process with the ID '%s' is still running", tid)
	}

	r.unsetPlayoutPorts(task)
	r.unsetCleanup(tid)

	delete(r.tasks, tid)

	return nil
}

func (r *restream) StartProcess(id TaskID) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	err := r.startProcess(id)
	if err != nil {
		return err
	}

	r.save()

	return nil
}

func (r *restream) startProcess(tid TaskID) error {
	task, ok := r.tasks[tid]
	if !ok {
		return ErrUnknownProcess
	}

	if !task.valid {
		return fmt.Errorf("invalid process definition")
	}

	if task.ffmpeg != nil {
		status := task.ffmpeg.Status()

		if task.process.Order == "start" && status.Order == "start" {
			return nil
		}
	}

	if r.maxProc > 0 && r.nProc >= r.maxProc {
		return fmt.Errorf("max. number of running processes (%d) reached", r.maxProc)
	}

	task.process.Order = "start"

	task.ffmpeg.Start()

	r.nProc++

	return nil
}

func (r *restream) StopProcess(id TaskID) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	err := r.stopProcess(id)
	if err != nil {
		return err
	}

	r.save()

	return nil
}

func (r *restream) stopProcess(tid TaskID) error {
	task, ok := r.tasks[tid]
	if !ok {
		return ErrUnknownProcess
	}

	if task.ffmpeg == nil {
		return nil
	}

	status := task.ffmpeg.Status()

	if task.process.Order == "stop" && status.Order == "stop" {
		return nil
	}

	task.process.Order = "stop"

	task.ffmpeg.Stop(true)

	r.nProc--

	return nil
}

func (r *restream) RestartProcess(id TaskID) error {
	r.lock.RLock()
	defer r.lock.RUnlock()

	return r.restartProcess(id)
}

func (r *restream) restartProcess(tid TaskID) error {
	task, ok := r.tasks[tid]
	if !ok {
		return ErrUnknownProcess
	}

	if !task.valid {
		return fmt.Errorf("invalid process definition")
	}

	if task.process.Order == "stop" {
		return nil
	}

	if task.ffmpeg != nil {
		task.ffmpeg.Stop(true)
		task.ffmpeg.Start()
	}

	return nil
}

func (r *restream) ReloadProcess(id TaskID) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	err := r.reloadProcess(id)
	if err != nil {
		return err
	}

	r.save()

	return nil
}

func (r *restream) reloadProcess(tid TaskID) error {
	t, ok := r.tasks[tid]
	if !ok {
		return ErrUnknownProcess
	}

	t.valid = false

	t.config = t.process.Config.Clone()

	resolveStaticPlaceholders(t.config, r.replace)

	err := r.resolveAddresses(r.tasks, t.config)
	if err != nil {
		return err
	}

	// Validate config with all placeholders replaced. However, we need to take care
	// that the config with the task keeps its dynamic placeholders for process starts.
	config := t.config.Clone()
	resolveDynamicPlaceholder(config, r.replace)

	t.usesDisk, err = validateConfig(config, r.fs.list, r.ffmpeg)
	if err != nil {
		return err
	}

	err = r.setPlayoutPorts(t)
	if err != nil {
		return err
	}

	t.command = t.config.CreateCommand()

	order := "stop"
	if t.process.Order == "start" {
		order = "start"
		r.stopProcess(tid)
	}

	parser := r.ffmpeg.NewProcessParser(t.logger, t.String(), t.reference, t.config.LogPatterns)
	t.parser.TransferReportHistory(parser)
	t.parser = parser

	limitMode := "hard"
	if r.enableSoftLimit {
		limitMode = "soft"
	}

	ffmpeg, err := r.ffmpeg.New(ffmpeg.ProcessConfig{
		Reconnect:      t.config.Reconnect,
		ReconnectDelay: time.Duration(t.config.ReconnectDelay) * time.Second,
		StaleTimeout:   time.Duration(t.config.StaleTimeout) * time.Second,
		Timeout:        time.Duration(t.config.Timeout) * time.Second,
		LimitCPU:       t.config.LimitCPU,
		LimitMemory:    t.config.LimitMemory,
		LimitDuration:  time.Duration(t.config.LimitWaitFor) * time.Second,
		LimitMode:      limitMode,
		Scheduler:      t.config.Scheduler,
		Args:           t.command,
		Parser:         t.parser,
		Logger:         t.logger,
		OnArgs:         r.onArgs(t.config.Clone()),
		OnBeforeStart: func() error {
			if !r.enableSoftLimit {
				return nil
			}

			if err := r.resources.Request(t.config.LimitCPU, t.config.LimitMemory); err != nil {
				return err
			}

			return nil
		},
	})
	if err != nil {
		return err
	}

	t.ffmpeg = ffmpeg
	t.valid = true

	if order == "start" {
		r.startProcess(tid)
	}

	return nil
}

func (r *restream) GetProcessState(id TaskID) (*app.State, error) {
	state := &app.State{}

	r.lock.RLock()
	defer r.lock.RUnlock()

	task, ok := r.tasks[id]
	if !ok {
		return state, ErrUnknownProcess
	}

	if !task.valid {
		return state, nil
	}

	status := task.ffmpeg.Status()

	state.Order = task.process.Order
	state.State = status.State
	state.States.Marshal(status.States)
	state.Time = status.Time.Unix()
	state.Memory = status.Memory.Current
	state.CPU = status.CPU.Current / status.CPU.NCPU
	state.Resources.CPU = status.CPU
	state.Resources.Memory = status.Memory
	state.Duration = status.Duration.Round(10 * time.Millisecond).Seconds()
	state.Reconnect = -1
	state.Command = status.CommandArgs
	state.LastLog = task.parser.LastLogline()

	if status.Reconnect >= time.Duration(0) {
		state.Reconnect = status.Reconnect.Round(10 * time.Millisecond).Seconds()
	}

	convertProgressFromParser(&state.Progress, task.parser.Progress())

	for i, p := range state.Progress.Input {
		if int(p.Index) >= len(task.process.Config.Input) {
			continue
		}

		state.Progress.Input[i].ID = task.process.Config.Input[p.Index].ID
	}

	for i, p := range state.Progress.Output {
		if int(p.Index) >= len(task.process.Config.Output) {
			continue
		}

		state.Progress.Output[i].ID = task.process.Config.Output[p.Index].ID
	}

	return state, nil
}

// convertProgressFromParser converts a ffmpeg/parse.Progress type into a restream/app.Progress type.
func convertProgressFromParser(progress *app.Progress, pprogress parse.Progress) {
	progress.Frame = pprogress.Frame
	progress.Packet = pprogress.Packet
	progress.FPS = pprogress.FPS
	progress.PPS = pprogress.PPS
	progress.Quantizer = pprogress.Quantizer
	progress.Size = pprogress.Size
	progress.Time = pprogress.Time
	progress.Bitrate = pprogress.Bitrate
	progress.Speed = pprogress.Speed
	progress.Drop = pprogress.Drop
	progress.Dup = pprogress.Dup

	for _, pinput := range pprogress.Input {
		input := app.ProgressIO{
			Address:   pinput.Address,
			Index:     pinput.Index,
			Stream:    pinput.Stream,
			Format:    pinput.Format,
			Type:      pinput.Type,
			Codec:     pinput.Codec,
			Coder:     pinput.Coder,
			Frame:     pinput.Frame,
			Keyframe:  pinput.Keyframe,
			Framerate: pinput.Framerate,
			FPS:       pinput.FPS,
			Packet:    pinput.Packet,
			PPS:       pinput.PPS,
			Size:      pinput.Size,
			Bitrate:   pinput.Bitrate,
			Extradata: pinput.Extradata,
			Pixfmt:    pinput.Pixfmt,
			Quantizer: pinput.Quantizer,
			Width:     pinput.Width,
			Height:    pinput.Height,
			Sampling:  pinput.Sampling,
			Layout:    pinput.Layout,
			Channels:  pinput.Channels,
			AVstream:  nil,
		}

		if pinput.AVstream != nil {
			avstream := &app.AVstream{
				Input: app.AVstreamIO{
					State:  pinput.AVstream.Input.State,
					Packet: pinput.AVstream.Input.Packet,
					Time:   pinput.AVstream.Input.Time,
					Size:   pinput.AVstream.Input.Size,
				},
				Output: app.AVstreamIO{
					State:  pinput.AVstream.Output.State,
					Packet: pinput.AVstream.Output.Packet,
					Time:   pinput.AVstream.Output.Time,
					Size:   pinput.AVstream.Output.Size,
				},
				Aqueue:         pinput.AVstream.Aqueue,
				Queue:          pinput.AVstream.Queue,
				Dup:            pinput.AVstream.Dup,
				Drop:           pinput.AVstream.Drop,
				Enc:            pinput.AVstream.Enc,
				Looping:        pinput.AVstream.Looping,
				LoopingRuntime: pinput.AVstream.LoopingRuntime,
				Duplicating:    pinput.AVstream.Duplicating,
				GOP:            pinput.AVstream.GOP,
			}

			input.AVstream = avstream
		}

		progress.Input = append(progress.Input, input)
	}

	for _, poutput := range pprogress.Output {
		output := app.ProgressIO{
			Address:   poutput.Address,
			Index:     poutput.Index,
			Stream:    poutput.Stream,
			Format:    poutput.Format,
			Type:      poutput.Type,
			Codec:     poutput.Codec,
			Coder:     poutput.Coder,
			Frame:     poutput.Frame,
			Keyframe:  poutput.Keyframe,
			Framerate: poutput.Framerate,
			FPS:       poutput.FPS,
			Packet:    poutput.Packet,
			PPS:       poutput.PPS,
			Size:      poutput.Size,
			Bitrate:   poutput.Bitrate,
			Extradata: poutput.Extradata,
			Pixfmt:    poutput.Pixfmt,
			Quantizer: poutput.Quantizer,
			Width:     poutput.Width,
			Height:    poutput.Height,
			Sampling:  poutput.Sampling,
			Layout:    poutput.Layout,
			Channels:  poutput.Channels,
			AVstream:  nil,
		}

		progress.Output = append(progress.Output, output)
	}
}

func (r *restream) GetProcessLog(id TaskID) (*app.Log, error) {
	log := &app.Log{}

	r.lock.RLock()
	defer r.lock.RUnlock()

	task, ok := r.tasks[id]
	if !ok {
		return log, ErrUnknownProcess
	}

	if !task.valid {
		return log, nil
	}

	current := task.parser.Report()

	log.CreatedAt = current.CreatedAt
	log.Prelude = current.Prelude
	log.Log = make([]app.LogLine, len(current.Log))
	for i, line := range current.Log {
		log.Log[i] = app.LogLine{
			Timestamp: line.Timestamp,
			Data:      line.Data,
		}
	}
	log.Matches = current.Matches

	history := task.parser.ReportHistory()

	for _, h := range history {
		e := app.LogHistoryEntry{
			LogEntry: app.LogEntry{
				CreatedAt: h.CreatedAt,
				Prelude:   h.Prelude,
				Matches:   h.Matches,
			},
			ExitedAt:  h.ExitedAt,
			ExitState: h.ExitState,
			Usage: app.ProcessUsage{
				CPU: app.ProcessUsageCPU{
					NCPU:    h.Usage.CPU.NCPU,
					Average: h.Usage.CPU.Average,
					Max:     h.Usage.CPU.Max,
					Limit:   h.Usage.CPU.Limit,
				},
				Memory: app.ProcessUsageMemory{
					Average: h.Usage.Memory.Average,
					Max:     h.Usage.Memory.Max,
					Limit:   h.Usage.Memory.Limit,
				},
			},
		}

		convertProgressFromParser(&e.Progress, h.Progress)

		for i, p := range e.Progress.Input {
			if int(p.Index) >= len(task.process.Config.Input) {
				continue
			}

			e.Progress.Input[i].ID = task.process.Config.Input[p.Index].ID
		}

		for i, p := range e.Progress.Output {
			if int(p.Index) >= len(task.process.Config.Output) {
				continue
			}

			e.Progress.Output[i].ID = task.process.Config.Output[p.Index].ID
		}

		e.LogEntry.Log = make([]app.LogLine, len(h.Log))
		for i, line := range h.Log {
			e.LogEntry.Log[i] = app.LogLine{
				Timestamp: line.Timestamp,
				Data:      line.Data,
			}
		}

		log.History = append(log.History, e)
	}

	return log, nil
}

func (r *restream) SearchProcessLogHistory(idpattern, refpattern, state string, from, to *time.Time) []app.LogHistorySearchResult {
	r.lock.RLock()
	defer r.lock.RUnlock()

	result := []app.LogHistorySearchResult{}

	ids := r.GetProcessIDs(idpattern, refpattern, "", "")

	for _, id := range ids {
		task, ok := r.tasks[id]
		if !ok {
			continue
		}

		presult := task.parser.SearchReportHistory(state, from, to)

		for _, f := range presult {
			result = append(result, app.LogHistorySearchResult{
				ProcessID: task.id,
				Reference: task.reference,
				ExitState: f.ExitState,
				CreatedAt: f.CreatedAt,
				ExitedAt:  f.ExitedAt,
			})
		}
	}

	return result
}

func (r *restream) Probe(id TaskID) app.Probe {
	return r.ProbeWithTimeout(id, 20*time.Second)
}

func (r *restream) ProbeWithTimeout(id TaskID, timeout time.Duration) app.Probe {
	appprobe := app.Probe{}

	r.lock.RLock()

	task, ok := r.tasks[id]
	if !ok {
		appprobe.Log = append(appprobe.Log, fmt.Sprintf("Unknown process ID (%s)", id))
		r.lock.RUnlock()
		return appprobe
	}

	r.lock.RUnlock()

	if !task.valid {
		return appprobe
	}

	var command []string

	// Copy global options
	command = append(command, task.config.Options...)

	for _, input := range task.config.Input {
		// Add the resolved input to the process command
		command = append(command, input.Options...)
		command = append(command, "-i", input.Address)
	}

	prober := r.ffmpeg.NewProbeParser(task.logger)

	var wg sync.WaitGroup

	wg.Add(1)

	ffmpeg, err := r.ffmpeg.New(ffmpeg.ProcessConfig{
		Reconnect:      false,
		ReconnectDelay: 0,
		StaleTimeout:   timeout,
		Args:           command,
		Parser:         prober,
		Logger:         task.logger,
		OnExit: func(string) {
			wg.Done()
		},
	})

	if err != nil {
		appprobe.Log = append(appprobe.Log, err.Error())
		return appprobe
	}

	ffmpeg.Start()

	wg.Wait()

	convertProbeFromProber(&appprobe, prober.Probe())

	return appprobe
}

// convertProbeFromProber converts a ffmpeg/probe.Probe type into an restream/app.Probe type.
func convertProbeFromProber(appprobe *app.Probe, pprobe probe.Probe) {
	appprobe.Log = make([]string, len(pprobe.Log))
	copy(appprobe.Log, pprobe.Log)

	for _, s := range pprobe.Streams {
		stream := app.ProbeIO{
			Address:  s.Address,
			Index:    s.Index,
			Stream:   s.Stream,
			Language: s.Language,
			Format:   s.Format,
			Type:     s.Type,
			Codec:    s.Codec,
			Coder:    s.Coder,
			Bitrate:  s.Bitrate,
			Duration: s.Duration,
			Pixfmt:   s.Pixfmt,
			Width:    s.Width,
			Height:   s.Height,
			FPS:      s.FPS,
			Sampling: s.Sampling,
			Layout:   s.Layout,
			Channels: s.Channels,
		}

		appprobe.Streams = append(appprobe.Streams, stream)
	}
}

func (r *restream) Skills() skills.Skills {
	return r.ffmpeg.Skills()
}

func (r *restream) ReloadSkills() error {
	return r.ffmpeg.ReloadSkills()
}

func (r *restream) GetPlayout(id TaskID, inputid string) (string, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	task, ok := r.tasks[id]
	if !ok {
		return "", ErrUnknownProcess
	}

	if !task.valid {
		return "", fmt.Errorf("invalid process definition")
	}

	port, ok := task.playout[inputid]
	if !ok {
		return "", fmt.Errorf("no playout for input ID '%s' and process '%s'", inputid, id)
	}

	return "127.0.0.1:" + strconv.Itoa(port), nil
}

var ErrMetadataKeyNotFound = errors.New("unknown key")

func (r *restream) SetProcessMetadata(id TaskID, key string, data interface{}) error {
	if len(key) == 0 {
		return fmt.Errorf("a key for storing the data has to be provided")
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	task, ok := r.tasks[id]
	if !ok {
		return ErrUnknownProcess
	}

	if task.metadata == nil {
		task.metadata = make(map[string]interface{})
	}

	if data == nil {
		delete(task.metadata, key)
	} else {
		task.metadata[key] = data
	}

	if len(task.metadata) == 0 {
		task.metadata = nil
	}

	r.save()

	return nil
}

func (r *restream) GetProcessMetadata(id TaskID, key string) (interface{}, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	task, ok := r.tasks[id]
	if !ok {
		return nil, ErrUnknownProcess
	}

	if len(key) == 0 {
		return task.metadata, nil
	}

	data, ok := task.metadata[key]
	if !ok {
		return nil, ErrMetadataKeyNotFound
	}

	return data, nil
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
		"group":     config.Domain,
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
		input.ID = r.Replace(input.ID, "reference", config.Reference, vars, config, "input")

		vars["inputid"] = input.ID

		input.Address = r.Replace(input.Address, "inputid", input.ID, vars, config, "input")
		input.Address = r.Replace(input.Address, "processid", config.ID, vars, config, "input")
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
		output.ID = r.Replace(output.ID, "reference", config.Reference, vars, config, "output")

		vars["outputid"] = output.ID

		output.Address = r.Replace(output.Address, "outputid", output.ID, vars, config, "output")
		output.Address = r.Replace(output.Address, "processid", config.ID, vars, config, "output")
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
			cleanup.Pattern = r.Replace(cleanup.Pattern, "reference", config.Reference, vars, config, "output")

			output.Cleanup[j] = cleanup
		}

		delete(vars, "outputid")

		config.Output[i] = output
	}
}

// resolveDynamicPlaceholder replaces placeholders in the config that should be replaced at process start.
// The config will be modified in place.
func resolveDynamicPlaceholder(config *app.Config, r replace.Replacer) {
	vars := map[string]string{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	for i, option := range config.Options {
		option = r.Replace(option, "date", "", vars, config, "global")

		config.Options[i] = option
	}

	for i, input := range config.Input {
		input.Address = r.Replace(input.Address, "date", "", vars, config, "input")

		for j, option := range input.Options {
			option = r.Replace(option, "date", "", vars, config, "input")

			input.Options[j] = option
		}

		config.Input[i] = input
	}

	for i, output := range config.Output {
		output.Address = r.Replace(output.Address, "date", "", vars, config, "output")

		for j, option := range output.Options {
			option = r.Replace(option, "date", "", vars, config, "output")

			output.Options[j] = option
		}

		for j, cleanup := range output.Cleanup {
			cleanup.Pattern = r.Replace(cleanup.Pattern, "date", "", vars, config, "output")

			output.Cleanup[j] = cleanup
		}

		config.Output[i] = output
	}
}
