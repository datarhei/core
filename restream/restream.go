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

	AddProcess(config *app.Config) error                                      // Add a new process
	GetProcessIDs(idpattern, refpattern, user, group string) []string         // Get a list of process IDs based on patterns for ID and reference
	DeleteProcess(id, user, group string) error                               // Delete a process
	UpdateProcess(id, user, group string, config *app.Config) error           // Update a process
	StartProcess(id, user, group string) error                                // Start a process
	StopProcess(id, user, group string) error                                 // Stop a process
	RestartProcess(id, user, group string) error                              // Restart a process
	ReloadProcess(id, user, group string) error                               // Reload a process
	GetProcess(id, user, group string) (*app.Process, error)                  // Get a process
	GetProcessState(id, user, group string) (*app.State, error)               // Get the state of a process
	GetProcessLog(id, user, group string) (*app.Log, error)                   // Get the logs of a process
	GetPlayout(id, user, group, inputid string) (string, error)               // Get the URL of the playout API for a process
	Probe(id, user, group string) app.Probe                                   // Probe a process
	ProbeWithTimeout(id, user, group string, timeout time.Duration) app.Probe // Probe a process with specific timeout
	SetProcessMetadata(id, user, group, key string, data interface{}) error   // Set metatdata to a process
	GetProcessMetadata(id, user, group, key string) (interface{}, error)      // Get previously set metadata from a process
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
	Logger       log.Logger
	IAM          iam.IAM
}

type task struct {
	valid     bool
	id        string // ID of the task/process
	owner     string
	group     string
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

func newTaskid(id, group string) string {
	return id + "~" + group
}

func (t *task) String() string {
	return newTaskid(t.id, t.group)
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
		list         []rfs.Filesystem
		diskfs       []rfs.Filesystem
		stopObserver context.CancelFunc
	}
	replace  replace.Replacer
	rewrite  rewrite.Rewriter
	tasks    map[string]*task
	logger   log.Logger
	metadata map[string]interface{}

	lock sync.RWMutex

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
		s, err := store.NewJSON(store.JSONConfig{
			Filesystem: dummyfs,
		})
		if err != nil {
			return nil, err
		}
		r.store = s
	}

	for _, fs := range config.Filesystems {
		fs := rfs.New(rfs.Config{
			FS:     fs,
			Logger: r.logger.WithComponent("Cleanup"),
		})

		r.fs.list = append(r.fs.list, fs)

		// Add the diskfs filesystems also to a separate array. We need it later for input and output validation
		if fs.Type() == "disk" {
			r.fs.diskfs = append(r.fs.diskfs, fs)
		}
	}

	if r.replace == nil {
		r.replace = replace.New()
	}

	r.ffmpeg = config.FFmpeg
	if r.ffmpeg == nil {
		return nil, fmt.Errorf("ffmpeg must be provided")
	}

	r.maxProc = config.MaxProcesses

	if err := r.load(); err != nil {
		return nil, fmt.Errorf("failed to load data from DB (%w)", err)
	}

	r.save()

	r.stopOnce.Do(func() {})

	return r, nil
}

func (r *restream) Start() {
	r.startOnce.Do(func() {
		r.lock.Lock()
		defer r.lock.Unlock()

		for id, t := range r.tasks {
			if t.process.Order == "start" {
				r.startProcess(id)
			}

			// The filesystem cleanup rules can be set
			r.setCleanup(id, t.config)
		}

		ctx, cancel := context.WithCancel(context.Background())
		r.fs.stopObserver = cancel

		for _, fs := range r.fs.list {
			fs.Start()

			if fs.Type() == "disk" {
				go r.observe(ctx, fs, 10*time.Second)
			}
		}

		r.stopOnce = sync.Once{}
	})
}

func (r *restream) Stop() {
	r.stopOnce.Do(func() {
		r.lock.Lock()
		defer r.lock.Unlock()

		// Stop the currently running processes without
		// altering their order such that on a subsequent
		// Start() they will get restarted.
		for id, t := range r.tasks {
			if t.ffmpeg != nil {
				t.ffmpeg.Stop(true)
			}

			r.unsetCleanup(id)
		}

		r.fs.stopObserver()

		// Stop the cleanup jobs
		for _, fs := range r.fs.list {
			fs.Stop()
		}

		r.startOnce = sync.Once{}
	})
}

func (r *restream) observe(ctx context.Context, fs fs.Filesystem, interval time.Duration) {
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

func (r *restream) load() error {
	data, err := r.store.Load()
	if err != nil {
		return err
	}

	tasks := make(map[string]*task)

	skills := r.ffmpeg.Skills()
	ffversion := skills.FFmpeg.Version
	if v, err := semver.NewVersion(ffversion); err == nil {
		// Remove the patch level for the constraint
		ffversion = fmt.Sprintf("%d.%d.0", v.Major(), v.Minor())
	}

	for _, process := range data.Process {
		if len(process.Config.FFVersion) == 0 {
			process.Config.FFVersion = "^" + ffversion
		}

		t := &task{
			id:        process.ID,
			owner:     process.Owner,
			group:     process.Group,
			reference: process.Reference,
			process:   process,
			config:    process.Config.Clone(),
			logger: r.logger.WithFields(log.Fields{
				"id":    process.ID,
				"owner": process.Owner,
				"group": process.Group,
			}),
		}

		// Replace all placeholders in the config
		resolvePlaceholders(t.config, r.replace)

		tasks[t.String()] = t
	}

	for tid, userdata := range data.Metadata.Process {
		t, ok := tasks[tid]
		if !ok {
			continue
		}

		t.metadata = userdata
	}

	// Now that all tasks are defined and all placeholders are
	// replaced, we can resolve references and validate the
	// inputs and outputs.
	for tid, t := range tasks {
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

		if !r.enforce(t.owner, t.group, t.id, "CREATE") {
			t.logger.Warn().WithError(fmt.Errorf("forbidden")).Log("Ignoring")
			continue
		}

		err := r.resolveAddresses(tasks, t.config)
		if err != nil {
			t.logger.Warn().WithError(err).Log("Ignoring")
			continue
		}

		t.usesDisk, err = r.validateConfig(t.config)
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
		t.parser = r.ffmpeg.NewProcessParser(t.logger, tid, t.reference)

		ffmpeg, err := r.ffmpeg.New(ffmpeg.ProcessConfig{
			Reconnect:      t.config.Reconnect,
			ReconnectDelay: time.Duration(t.config.ReconnectDelay) * time.Second,
			StaleTimeout:   time.Duration(t.config.StaleTimeout) * time.Second,
			Command:        t.command,
			Parser:         t.parser,
			Logger:         t.logger,
		})
		if err != nil {
			return err
		}

		t.ffmpeg = ffmpeg
		t.valid = true
	}

	r.tasks = tasks
	r.metadata = data.Metadata.System

	return nil
}

func (r *restream) save() {
	data := store.NewStoreData()

	for tid, t := range r.tasks {
		data.Process[tid] = t.process
		data.Metadata.System = r.metadata
		data.Metadata.Process[tid] = t.metadata
	}

	r.store.Store(data)
}

func (r *restream) enforce(name, group, processid, action string) bool {
	if len(name) == 0 {
		// This is for backwards compatibility. Existing processes don't have an owner.
		// All processes that will be added later will have an owner ($anon, ...).
		name = r.iam.GetDefaultVerifier().Name()
	}

	if len(group) == 0 {
		group = "$none"
	}

	return r.iam.Enforce(name, group, "process:"+processid, action)
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
	if !r.enforce(config.Owner, config.Group, config.ID, "CREATE") {
		return ErrForbidden
	}

	r.lock.RLock()
	t, err := r.createTask(config)
	r.lock.RUnlock()

	if err != nil {
		return err
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	tid := t.String()

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
		Group:     config.Group,
		Reference: config.Reference,
		Config:    config.Clone(),
		Order:     "stop",
		CreatedAt: time.Now().Unix(),
	}

	if config.Autostart {
		process.Order = "start"
	}

	t := &task{
		id:        config.ID,
		group:     config.Group,
		reference: process.Reference,
		process:   process,
		config:    process.Config.Clone(),
		logger: r.logger.WithFields(log.Fields{
			"id":    process.ID,
			"group": process.Group,
		}),
	}

	resolvePlaceholders(t.config, r.replace)

	err := r.resolveAddresses(r.tasks, t.config)
	if err != nil {
		return nil, err
	}

	t.usesDisk, err = r.validateConfig(t.config)
	if err != nil {
		return nil, err
	}

	err = r.setPlayoutPorts(t)
	if err != nil {
		return nil, err
	}

	t.command = t.config.CreateCommand()
	t.parser = r.ffmpeg.NewProcessParser(t.logger, t.String(), t.reference)

	ffmpeg, err := r.ffmpeg.New(ffmpeg.ProcessConfig{
		Reconnect:      t.config.Reconnect,
		ReconnectDelay: time.Duration(t.config.ReconnectDelay) * time.Second,
		StaleTimeout:   time.Duration(t.config.StaleTimeout) * time.Second,
		Command:        t.command,
		Parser:         t.parser,
		Logger:         t.logger,
	})
	if err != nil {
		return nil, err
	}

	t.ffmpeg = ffmpeg
	t.valid = true

	return t, nil
}

func (r *restream) setCleanup(id string, config *app.Config) {
	rePrefix := regexp.MustCompile(`^([a-z]+):`)

	for _, output := range config.Output {
		for _, c := range output.Cleanup {
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

				fs.SetCleanup(id, []rfs.Pattern{
					pattern,
				})

				break
			}
		}
	}
}

func (r *restream) unsetCleanup(id string) {
	for _, fs := range r.fs.list {
		fs.UnsetCleanup(id)
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

func (r *restream) validateConfig(config *app.Config) (bool, error) {
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

		if len(r.fs.diskfs) != 0 {
			maxFails := 0
			for _, fs := range r.fs.diskfs {
				io.Address, err = r.validateInputAddress(io.Address, fs.Metadata("base"))
				if err != nil {
					maxFails++
				}
			}

			if maxFails == len(r.fs.diskfs) {
				return false, fmt.Errorf("the address for input '#%s:%s' (%s) is invalid: %w", config.ID, io.ID, io.Address, err)
			}
		} else {
			io.Address, err = r.validateInputAddress(io.Address, "/")
			if err != nil {
				return false, fmt.Errorf("the address for input '#%s:%s' (%s) is invalid: %w", config.ID, io.ID, io.Address, err)
			}
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

		if len(r.fs.diskfs) != 0 {
			maxFails := 0
			for _, fs := range r.fs.diskfs {
				isFile := false
				io.Address, isFile, err = r.validateOutputAddress(io.Address, fs.Metadata("base"))
				if err != nil {
					maxFails++
				}

				if isFile {
					hasFiles = true
				}
			}

			if maxFails == len(r.fs.diskfs) {
				return false, fmt.Errorf("the address for output '#%s:%s' is invalid: %w", config.ID, io.ID, err)
			}
		} else {
			isFile := false
			io.Address, isFile, err = r.validateOutputAddress(io.Address, "/")
			if err != nil {
				return false, fmt.Errorf("the address for output '#%s:%s' is invalid: %w", config.ID, io.ID, err)
			}

			if isFile {
				hasFiles = true
			}
		}
	}

	return hasFiles, nil
}

func (r *restream) validateInputAddress(address, basedir string) (string, error) {
	if ok := url.HasScheme(address); ok {
		if err := url.Validate(address); err != nil {
			return address, err
		}
	}

	if !r.ffmpeg.ValidateInputAddress(address) {
		return address, fmt.Errorf("address is not allowed")
	}

	return address, nil
}

func (r *restream) validateOutputAddress(address, basedir string) (string, bool, error) {
	// If the address contains a "|" or it starts with a "[", then assume that it
	// is an address for the tee muxer.
	if strings.Contains(address, "|") || strings.HasPrefix(address, "[") {
		addresses := strings.Split(address, "|")

		isFile := false

		teeOptions := regexp.MustCompile(`^\[[^\]]*\]`)

		for i, a := range addresses {
			options := teeOptions.FindString(a)
			a = teeOptions.ReplaceAllString(a, "")

			va, file, err := r.validateOutputAddress(a, basedir)
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

		if !r.ffmpeg.ValidateOutputAddress(address) {
			return address, false, fmt.Errorf("address is not allowed")
		}

		return address, false, nil
	}

	if address == "-" {
		return "pipe:", false, nil
	}

	address, err := filepath.Abs(address)
	if err != nil {
		return address, false, fmt.Errorf("not a valid path (%w)", err)
	}

	if strings.HasPrefix(address, "/dev/") {
		if !r.ffmpeg.ValidateOutputAddress("file:" + address) {
			return address, false, fmt.Errorf("address is not allowed")
		}

		return "file:" + address, false, nil
	}

	if !strings.HasPrefix(address, basedir) {
		return address, false, fmt.Errorf("%s is not inside of %s", address, basedir)
	}

	if !r.ffmpeg.ValidateOutputAddress("file:" + address) {
		return address, false, fmt.Errorf("address is not allowed")
	}

	return "file:" + address, true, nil
}

func (r *restream) resolveAddresses(tasks map[string]*task, config *app.Config) error {
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

func (r *restream) resolveAddress(tasks map[string]*task, id, address string) (string, error) {
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
		if tsk.id == matches["id"] && tsk.group == matches["group"] {
			t = tsk
			break
		}
	}

	if t == nil {
		return address, fmt.Errorf("unknown process '%s' in group '%s' (%s)", matches["id"], matches["group"], address)
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

	re := regexp.MustCompile(`:(output|group|source)=(.+)`)

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

func (r *restream) UpdateProcess(id, user, group string, config *app.Config) error {
	if !r.enforce(user, group, id, "UPDATE") {
		return ErrForbidden
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	t, err := r.createTask(config)
	if err != nil {
		return err
	}

	tid := newTaskid(id, group)

	task, ok := r.tasks[tid]
	if !ok {
		return ErrUnknownProcess
	}

	t.process.Order = task.process.Order

	if tid != t.String() {
		_, ok := r.tasks[t.String()]
		if ok {
			return ErrProcessExists
		}
	}

	if err := r.stopProcess(tid); err != nil {
		return err
	}

	if err := r.deleteProcess(tid); err != nil {
		return err
	}

	tid = t.String()

	r.tasks[tid] = t

	// set filesystem cleanup rules
	r.setCleanup(tid, t.config)

	if t.process.Order == "start" {
		r.startProcess(tid)
	}

	r.save()

	return nil
}

func (r *restream) GetProcessIDs(idpattern, refpattern, user, group string) []string {
	r.lock.RLock()
	defer r.lock.RUnlock()

	if len(idpattern) == 0 && len(refpattern) == 0 {
		ids := []string{}

		for _, t := range r.tasks {
			if t.group != group {
				continue
			}

			if !r.enforce(user, group, t.id, "GET") {
				continue
			}

			ids = append(ids, t.id)
		}

		return ids
	}

	idmap := map[string]int{}
	count := 0

	if len(idpattern) != 0 {
		for _, t := range r.tasks {
			if t.group != group {
				continue
			}

			if !r.enforce(user, group, t.id, "GET") {
				continue
			}

			match, err := glob.Match(idpattern, t.id)
			if err != nil {
				return nil
			}

			if !match {
				continue
			}

			idmap[t.id]++
		}

		count++
	}

	if len(refpattern) != 0 {
		for _, t := range r.tasks {
			if t.group != group {
				continue
			}

			if !r.enforce(user, group, t.id, "GET") {
				continue
			}

			match, err := glob.Match(refpattern, t.reference)
			if err != nil {
				return nil
			}

			if !match {
				continue
			}

			idmap[t.id]++
		}

		count++
	}

	ids := []string{}

	for id, n := range idmap {
		if n != count {
			continue
		}

		ids = append(ids, id)
	}

	return ids
}

func (r *restream) GetProcess(id, user, group string) (*app.Process, error) {
	if !r.enforce(user, group, id, "GET") {
		return nil, ErrForbidden
	}

	tid := newTaskid(id, group)

	r.lock.RLock()
	defer r.lock.RUnlock()

	task, ok := r.tasks[tid]
	if !ok {
		return &app.Process{}, ErrUnknownProcess
	}

	process := task.process.Clone()

	return process, nil
}

func (r *restream) DeleteProcess(id, user, group string) error {
	if !r.enforce(user, group, id, "DELETE") {
		return ErrForbidden
	}

	tid := newTaskid(id, group)

	r.lock.Lock()
	defer r.lock.Unlock()

	err := r.deleteProcess(tid)
	if err != nil {
		return err
	}

	r.save()

	return nil
}

func (r *restream) deleteProcess(tid string) error {
	task, ok := r.tasks[tid]
	if !ok {
		return ErrUnknownProcess
	}

	if task.process.Order != "stop" {
		return fmt.Errorf("the process with the ID '%s' is still running", task.id)
	}

	r.unsetPlayoutPorts(task)
	r.unsetCleanup(tid)

	delete(r.tasks, tid)

	return nil
}

func (r *restream) StartProcess(id, user, group string) error {
	if !r.enforce(user, group, id, "COMMAND") {
		return ErrForbidden
	}

	tid := newTaskid(id, group)

	r.lock.Lock()
	defer r.lock.Unlock()

	err := r.startProcess(tid)
	if err != nil {
		return err
	}

	r.save()

	return nil
}

func (r *restream) startProcess(tid string) error {
	task, ok := r.tasks[tid]
	if !ok {
		return ErrUnknownProcess
	}

	if !task.valid {
		return fmt.Errorf("invalid process definition")
	}

	status := task.ffmpeg.Status()

	if task.process.Order == "start" && status.Order == "start" {
		return nil
	}

	if r.maxProc > 0 && r.nProc >= r.maxProc {
		return fmt.Errorf("max. number of running processes (%d) reached", r.maxProc)
	}

	task.process.Order = "start"

	task.ffmpeg.Start()

	r.nProc++

	return nil
}

func (r *restream) StopProcess(id, user, group string) error {
	if !r.enforce(user, group, id, "COMMAND") {
		return ErrForbidden
	}

	tid := newTaskid(id, group)

	r.lock.Lock()
	defer r.lock.Unlock()

	err := r.stopProcess(tid)
	if err != nil {
		return err
	}

	r.save()

	return nil
}

func (r *restream) stopProcess(tid string) error {
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

func (r *restream) RestartProcess(id, user, group string) error {
	if !r.enforce(user, group, id, "COMMAND") {
		return ErrForbidden
	}

	tid := newTaskid(id, group)

	r.lock.RLock()
	defer r.lock.RUnlock()

	return r.restartProcess(tid)
}

func (r *restream) restartProcess(tid string) error {
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

	task.ffmpeg.Kill(true)

	return nil
}

func (r *restream) ReloadProcess(id, user, group string) error {
	if !r.enforce(user, group, id, "COMMAND") {
		return ErrForbidden
	}

	tid := newTaskid(id, group)

	r.lock.Lock()
	defer r.lock.Unlock()

	err := r.reloadProcess(tid)
	if err != nil {
		return err
	}

	r.save()

	return nil
}

func (r *restream) reloadProcess(tid string) error {
	t, ok := r.tasks[tid]
	if !ok {
		return ErrUnknownProcess
	}

	t.valid = false

	t.config = t.process.Config.Clone()

	resolvePlaceholders(t.config, r.replace)

	err := r.resolveAddresses(r.tasks, t.config)
	if err != nil {
		return err
	}

	t.usesDisk, err = r.validateConfig(t.config)
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

	t.parser = r.ffmpeg.NewProcessParser(t.logger, t.id, t.reference)

	ffmpeg, err := r.ffmpeg.New(ffmpeg.ProcessConfig{
		Reconnect:      t.config.Reconnect,
		ReconnectDelay: time.Duration(t.config.ReconnectDelay) * time.Second,
		StaleTimeout:   time.Duration(t.config.StaleTimeout) * time.Second,
		Command:        t.command,
		Parser:         t.parser,
		Logger:         t.logger,
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

func (r *restream) GetProcessState(id, user, group string) (*app.State, error) {
	state := &app.State{}

	if !r.enforce(user, group, id, "GET") {
		return state, ErrForbidden
	}

	tid := newTaskid(id, group)

	r.lock.RLock()
	defer r.lock.RUnlock()

	task, ok := r.tasks[tid]
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
	state.Memory = status.Memory
	state.CPU = status.CPU
	state.Duration = status.Duration.Round(10 * time.Millisecond).Seconds()
	state.Reconnect = -1
	state.Command = make([]string, len(task.command))
	copy(state.Command, task.command)

	if state.Order == "start" && !task.ffmpeg.IsRunning() && task.config.Reconnect {
		state.Reconnect = float64(task.config.ReconnectDelay) - state.Duration

		if state.Reconnect < 0 {
			state.Reconnect = 0
		}
	}

	state.Progress = task.parser.Progress()

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

	report := task.parser.Report()

	if len(report.Log) != 0 {
		state.LastLog = report.Log[len(report.Log)-1].Data
	}

	return state, nil
}

func (r *restream) GetProcessLog(id, user, group string) (*app.Log, error) {
	log := &app.Log{}

	if !r.enforce(user, group, id, "GET") {
		return log, ErrForbidden
	}

	tid := newTaskid(id, group)

	r.lock.RLock()
	defer r.lock.RUnlock()

	task, ok := r.tasks[tid]
	if !ok {
		return log, ErrUnknownProcess
	}

	if !task.valid {
		return log, nil
	}

	current := task.parser.Report()

	log.CreatedAt = current.CreatedAt
	log.Prelude = current.Prelude
	log.Log = make([]app.LogEntry, len(current.Log))
	for i, line := range current.Log {
		log.Log[i] = app.LogEntry{
			Timestamp: line.Timestamp,
			Data:      line.Data,
		}
	}

	history := task.parser.ReportHistory()

	for _, h := range history {
		e := app.LogHistoryEntry{
			CreatedAt: h.CreatedAt,
			Prelude:   h.Prelude,
		}

		e.Log = make([]app.LogEntry, len(h.Log))
		for i, line := range h.Log {
			e.Log[i] = app.LogEntry{
				Timestamp: line.Timestamp,
				Data:      line.Data,
			}
		}

		log.History = append(log.History, e)
	}

	return log, nil
}

func (r *restream) Probe(id, user, group string) app.Probe {
	return r.ProbeWithTimeout(id, user, group, 20*time.Second)
}

func (r *restream) ProbeWithTimeout(id, user, group string, timeout time.Duration) app.Probe {
	appprobe := app.Probe{}

	if !r.enforce(user, group, id, "PROBE") {
		appprobe.Log = append(appprobe.Log, ErrForbidden.Error())
		return appprobe
	}

	tid := newTaskid(id, group)

	r.lock.RLock()

	task, ok := r.tasks[tid]
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
		Command:        command,
		Parser:         prober,
		Logger:         task.logger,
		OnExit: func() {
			wg.Done()
		},
	})

	if err != nil {
		appprobe.Log = append(appprobe.Log, err.Error())
		return appprobe
	}

	ffmpeg.Start()

	wg.Wait()

	appprobe = prober.Probe()

	return appprobe
}

func (r *restream) Skills() skills.Skills {
	return r.ffmpeg.Skills()
}

func (r *restream) ReloadSkills() error {
	return r.ffmpeg.ReloadSkills()
}

func (r *restream) GetPlayout(id, user, group, inputid string) (string, error) {
	if !r.enforce(user, group, id, "PLAYOUT") {
		return "", ErrForbidden
	}

	tid := newTaskid(id, group)

	r.lock.RLock()
	defer r.lock.RUnlock()

	task, ok := r.tasks[tid]
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

func (r *restream) SetProcessMetadata(id, user, group, key string, data interface{}) error {
	if !r.enforce(user, group, id, "METADATA") {
		return ErrForbidden
	}

	if len(key) == 0 {
		return fmt.Errorf("a key for storing the data has to be provided")
	}

	tid := newTaskid(id, group)

	r.lock.Lock()
	defer r.lock.Unlock()

	task, ok := r.tasks[tid]
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

func (r *restream) GetProcessMetadata(id, user, group, key string) (interface{}, error) {
	if !r.enforce(user, group, id, "METADATA") {
		return nil, ErrForbidden
	}

	tid := newTaskid(id, group)

	r.lock.RLock()
	defer r.lock.RUnlock()

	task, ok := r.tasks[tid]
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
		return r.metadata, nil
	}

	data, ok := r.metadata[key]
	if !ok {
		return nil, ErrMetadataKeyNotFound
	}

	return data, nil
}

// resolvePlaceholders replaces all placeholders in the config. The config
// will be modified in place.
func resolvePlaceholders(config *app.Config, r replace.Replacer) {
	vars := map[string]string{
		"processid": config.ID,
		"owner":     config.Owner,
		"reference": config.Reference,
		"group":     config.Group,
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
		input.ID = r.Replace(input.ID, "processid", config.ID, nil, nil, "input")
		input.ID = r.Replace(input.ID, "reference", config.Reference, nil, nil, "input")

		vars["inputid"] = input.ID

		input.Address = r.Replace(input.Address, "inputid", input.ID, nil, nil, "input")
		input.Address = r.Replace(input.Address, "processid", config.ID, nil, nil, "input")
		input.Address = r.Replace(input.Address, "reference", config.Reference, nil, nil, "input")
		input.Address = r.Replace(input.Address, "diskfs", "", vars, config, "input")
		input.Address = r.Replace(input.Address, "memfs", "", vars, config, "input")
		input.Address = r.Replace(input.Address, "fs:*", "", vars, config, "input")
		input.Address = r.Replace(input.Address, "rtmp", "", vars, config, "input")
		input.Address = r.Replace(input.Address, "srt", "", vars, config, "input")

		for j, option := range input.Options {
			// Replace any known placeholders
			option = r.Replace(option, "inputid", input.ID, nil, nil, "input")
			option = r.Replace(option, "processid", config.ID, nil, nil, "input")
			option = r.Replace(option, "reference", config.Reference, nil, nil, "input")
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
		output.ID = r.Replace(output.ID, "processid", config.ID, nil, nil, "output")
		output.ID = r.Replace(output.ID, "reference", config.Reference, nil, nil, "output")

		vars["outputid"] = output.ID

		output.Address = r.Replace(output.Address, "outputid", output.ID, nil, nil, "output")
		output.Address = r.Replace(output.Address, "processid", config.ID, nil, nil, "output")
		output.Address = r.Replace(output.Address, "reference", config.Reference, nil, nil, "output")
		output.Address = r.Replace(output.Address, "diskfs", "", vars, config, "output")
		output.Address = r.Replace(output.Address, "memfs", "", vars, config, "output")
		output.Address = r.Replace(output.Address, "fs:*", "", vars, config, "output")
		output.Address = r.Replace(output.Address, "rtmp", "", vars, config, "output")
		output.Address = r.Replace(output.Address, "srt", "", vars, config, "output")

		for j, option := range output.Options {
			// Replace any known placeholders
			option = r.Replace(option, "outputid", output.ID, nil, nil, "output")
			option = r.Replace(option, "processid", config.ID, nil, nil, "output")
			option = r.Replace(option, "reference", config.Reference, nil, nil, "output")
			option = r.Replace(option, "diskfs", "", vars, config, "output")
			option = r.Replace(option, "memfs", "", vars, config, "output")
			option = r.Replace(option, "fs:*", "", vars, config, "output")

			output.Options[j] = option
		}

		for j, cleanup := range output.Cleanup {
			// Replace any known placeholders
			cleanup.Pattern = r.Replace(cleanup.Pattern, "outputid", output.ID, nil, nil, "output")
			cleanup.Pattern = r.Replace(cleanup.Pattern, "processid", config.ID, nil, nil, "output")
			cleanup.Pattern = r.Replace(cleanup.Pattern, "reference", config.Reference, nil, nil, "output")

			output.Cleanup[j] = cleanup
		}

		delete(vars, "outputid")

		config.Output[i] = output
	}
}
