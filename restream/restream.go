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
	"github.com/datarhei/core/v16/io/fs"
	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/net"
	"github.com/datarhei/core/v16/net/url"
	"github.com/datarhei/core/v16/process"
	"github.com/datarhei/core/v16/restream/app"
	rfs "github.com/datarhei/core/v16/restream/fs"
	"github.com/datarhei/core/v16/restream/replace"
	"github.com/datarhei/core/v16/restream/store"
)

// The Restreamer interface
type Restreamer interface {
	ID() string                                                // ID of this instance
	Name() string                                              // Arbitrary name of this instance
	CreatedAt() time.Time                                      // time of when this instance has been created
	Start()                                                    // start all processes that have a "start" order
	Stop()                                                     // stop all running process but keep their "start" order
	AddProcess(config *app.Config) error                       // add a new process
	GetProcessIDs(idpattern, refpattern string) []string       // get a list of process IDs based on patterns for ID and reference
	DeleteProcess(id string) error                             // delete a process
	UpdateProcess(id string, config *app.Config) error         // update a process
	StartProcess(id string) error                              // start a process
	StopProcess(id string) error                               // stop a process
	RestartProcess(id string) error                            // restart a process
	ReloadProcess(id string) error                             // reload a process
	GetProcess(id string) (*app.Process, error)                // get a process
	GetProcessState(id string) (*app.State, error)             // get the state of a process
	GetProcessLog(id string) (*app.Log, error)                 // get the logs of a process
	GetPlayout(id, inputid string) (string, error)             // get the URL of the playout API for a process
	Probe(id string) app.Probe                                 // probe a process
	Skills() skills.Skills                                     // get the ffmpeg skills
	ReloadSkills() error                                       // reload the ffmpeg skills
	SetProcessMetadata(id, key string, data interface{}) error // set metatdata to a process
	GetProcessMetadata(id, key string) (interface{}, error)    // get previously set metadata from a process
	SetMetadata(key string, data interface{}) error            // set general metadata
	GetMetadata(key string) (interface{}, error)               // get previously set general metadata
}

// Config is the required configuration for a new restreamer instance.
type Config struct {
	ID           string
	Name         string
	Store        store.Store
	DiskFS       fs.Filesystem
	MemFS        fs.Filesystem
	Replace      replace.Replacer
	FFmpeg       ffmpeg.FFmpeg
	MaxProcesses int64
	Logger       log.Logger
}

type task struct {
	valid     bool
	id        string // ID of the task/process
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

type restream struct {
	id        string
	name      string
	createdAt time.Time
	store     store.Store
	ffmpeg    ffmpeg.FFmpeg
	maxProc   int64
	nProc     int64
	fs        struct {
		diskfs       rfs.Filesystem
		memfs        rfs.Filesystem
		stopObserver context.CancelFunc
	}
	replace  replace.Replacer
	tasks    map[string]*task
	logger   log.Logger
	metadata map[string]interface{}

	lock sync.RWMutex

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
		logger:    config.Logger,
	}

	if r.logger == nil {
		r.logger = log.New("")
	}

	if r.store == nil {
		r.store = store.NewDummyStore(store.DummyConfig{})
	}

	if config.DiskFS != nil {
		r.fs.diskfs = rfs.New(rfs.Config{
			FS:     config.DiskFS,
			Logger: r.logger.WithComponent("Cleanup").WithField("type", "diskfs"),
		})
	} else {
		r.fs.diskfs = rfs.New(rfs.Config{
			FS: fs.NewDummyFilesystem(),
		})
	}

	if config.MemFS != nil {
		r.fs.memfs = rfs.New(rfs.Config{
			FS:     config.MemFS,
			Logger: r.logger.WithComponent("Cleanup").WithField("type", "memfs"),
		})
	} else {
		r.fs.memfs = rfs.New(rfs.Config{
			FS: fs.NewDummyFilesystem(),
		})
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

		r.fs.diskfs.Start()
		r.fs.memfs.Start()

		ctx, cancel := context.WithCancel(context.Background())
		r.fs.stopObserver = cancel
		go r.observe(ctx, 10*time.Second)

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

		r.fs.diskfs.Stop()
		r.fs.memfs.Stop()

		r.startOnce = sync.Once{}
	})
}

func (r *restream) observe(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			size, limit := r.fs.diskfs.Size()
			isFull := false
			if limit > 0 && size >= limit {
				isFull = true
			}

			if isFull {
				// Stop all tasks that write to disk
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

					r.logger.Warn().Log("Shutting down because disk is full")
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

	for id, process := range data.Process {
		t := &task{
			id:        id,
			reference: process.Reference,
			process:   process,
			config:    process.Config.Clone(),
			logger:    r.logger.WithField("id", id),
		}

		// Replace all placeholders in the config
		t.config.ResolvePlaceholders(r.replace)

		tasks[id] = t
	}

	for id, userdata := range data.Metadata.Process {
		t, ok := tasks[id]
		if !ok {
			continue
		}

		t.metadata = userdata
	}

	// Now that all tasks are defined and all placeholders are
	// replaced, we can resolve references and validate the
	// inputs and outputs.
	for _, t := range tasks {
		err := r.resolveAddresses(tasks, t.config)
		if err != nil {
			r.logger.Warn().WithField("id", t.id).WithError(err).Log("Ignoring")
			continue
		}

		t.usesDisk, err = r.validateConfig(t.config)
		if err != nil {
			r.logger.Warn().WithField("id", t.id).WithError(err).Log("Ignoring")
			continue
		}

		err = r.setPlayoutPorts(t)
		if err != nil {
			r.logger.Warn().WithField("id", t.id).WithError(err).Log("Ignoring")
			continue
		}

		t.command = t.config.CreateCommand()
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
	}

	r.tasks = tasks
	r.metadata = data.Metadata.System

	return nil
}

func (r *restream) save() {
	data := store.NewStoreData()

	for id, t := range r.tasks {
		data.Process[id] = t.process
		data.Metadata.System = r.metadata
		data.Metadata.Process[id] = t.metadata
	}

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
var ErrProcessExists = errors.New("process already exists")

func (r *restream) AddProcess(config *app.Config) error {
	r.lock.RLock()
	t, err := r.createTask(config)
	r.lock.RUnlock()

	if err != nil {
		return err
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	_, ok := r.tasks[t.id]
	if ok {
		return ErrProcessExists
	}

	r.tasks[t.id] = t

	// set filesystem cleanup rules
	r.setCleanup(t.id, t.config)

	if t.process.Order == "start" {
		err := r.startProcess(t.id)
		if err != nil {
			delete(r.tasks, t.id)
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

	process := &app.Process{
		ID:        config.ID,
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
		reference: process.Reference,
		process:   process,
		config:    process.Config.Clone(),
		logger:    r.logger.WithField("id", process.ID),
	}

	t.config.ResolvePlaceholders(r.replace)

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
		return nil, err
	}

	t.ffmpeg = ffmpeg
	t.valid = true

	return t, nil
}

func (r *restream) setCleanup(id string, config *app.Config) {
	for _, output := range config.Output {
		for _, c := range output.Cleanup {
			if strings.HasPrefix(c.Pattern, "memfs:") {
				r.fs.memfs.SetCleanup(id, []rfs.Pattern{
					{
						Pattern:       strings.TrimPrefix(c.Pattern, "memfs:"),
						MaxFiles:      c.MaxFiles,
						MaxFileAge:    time.Duration(c.MaxFileAge) * time.Second,
						PurgeOnDelete: c.PurgeOnDelete,
					},
				})
			} else if strings.HasPrefix(c.Pattern, "diskfs:") {
				r.fs.diskfs.SetCleanup(id, []rfs.Pattern{
					{
						Pattern:       strings.TrimPrefix(c.Pattern, "diskfs:"),
						MaxFiles:      c.MaxFiles,
						MaxFileAge:    time.Duration(c.MaxFileAge) * time.Second,
						PurgeOnDelete: c.PurgeOnDelete,
					},
				})
			}
		}
	}
}

func (r *restream) unsetCleanup(id string) {
	r.fs.diskfs.UnsetCleanup(id)
	r.fs.memfs.UnsetCleanup(id)
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

		io.Address, err = r.validateInputAddress(io.Address, r.fs.diskfs.Base())
		if err != nil {
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

		isFile := false

		io.Address, isFile, err = r.validateOutputAddress(io.Address, r.fs.diskfs.Base())
		if err != nil {
			return false, fmt.Errorf("the address for output '#%s:%s' is invalid: %w", config.ID, io.ID, err)
		}

		if isFile {
			hasFiles = true
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
	re := regexp.MustCompile(`^#(.+):output=(.+)`)

	if len(address) == 0 {
		return address, fmt.Errorf("empty address")
	}

	if address[0] != '#' {
		return address, nil
	}

	matches := re.FindStringSubmatch(address)
	if matches == nil {
		return address, fmt.Errorf("invalid format (%s)", address)
	}

	if matches[1] == id {
		return address, fmt.Errorf("self-reference not possible (%s)", address)
	}

	task, ok := tasks[matches[1]]
	if !ok {
		return address, fmt.Errorf("unknown process '%s' (%s)", matches[1], address)
	}

	for _, x := range task.config.Output {
		if x.ID == matches[2] {
			return x.Address, nil
		}
	}

	return address, fmt.Errorf("the process '%s' has no outputs with the ID '%s' (%s)", matches[1], matches[2], address)
}

func (r *restream) UpdateProcess(id string, config *app.Config) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	t, err := r.createTask(config)
	if err != nil {
		return err
	}

	task, ok := r.tasks[id]
	if !ok {
		return ErrUnknownProcess
	}

	t.process.Order = task.process.Order

	if id != t.id {
		_, ok := r.tasks[t.id]
		if ok {
			return ErrProcessExists
		}
	}

	if err := r.stopProcess(id); err != nil {
		return err
	}

	if err := r.deleteProcess(id); err != nil {
		return err
	}

	r.tasks[t.id] = t

	// set filesystem cleanup rules
	r.setCleanup(t.id, t.config)

	if t.process.Order == "start" {
		r.startProcess(t.id)
	}

	r.save()

	return nil
}

func (r *restream) GetProcessIDs(idpattern, refpattern string) []string {
	r.lock.RLock()
	defer r.lock.RUnlock()

	if len(idpattern) == 0 && len(refpattern) == 0 {
		ids := make([]string, len(r.tasks))
		i := 0

		for id := range r.tasks {
			ids[i] = id
			i++
		}

		return ids
	}

	idmap := map[string]int{}
	count := 0

	if len(idpattern) != 0 {
		for id := range r.tasks {
			match, err := glob.Match(idpattern, id)
			if err != nil {
				return nil
			}

			if !match {
				continue
			}

			idmap[id]++
		}

		count++
	}

	if len(refpattern) != 0 {
		for _, t := range r.tasks {
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

func (r *restream) GetProcess(id string) (*app.Process, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	task, ok := r.tasks[id]
	if !ok {
		return &app.Process{}, ErrUnknownProcess
	}

	process := task.process.Clone()

	return process, nil
}

func (r *restream) DeleteProcess(id string) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	err := r.deleteProcess(id)
	if err != nil {
		return err
	}

	r.save()

	return nil
}

func (r *restream) deleteProcess(id string) error {
	task, ok := r.tasks[id]
	if !ok {
		return ErrUnknownProcess
	}

	if task.process.Order != "stop" {
		return fmt.Errorf("the process with the ID '%s' is still running", id)
	}

	r.unsetPlayoutPorts(task)
	r.unsetCleanup(id)

	delete(r.tasks, id)

	return nil
}

func (r *restream) StartProcess(id string) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	err := r.startProcess(id)
	if err != nil {
		return err
	}

	r.save()

	return nil
}

func (r *restream) startProcess(id string) error {
	task, ok := r.tasks[id]
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

func (r *restream) StopProcess(id string) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	err := r.stopProcess(id)
	if err != nil {
		return err
	}

	r.save()

	return nil
}

func (r *restream) stopProcess(id string) error {
	task, ok := r.tasks[id]
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

func (r *restream) RestartProcess(id string) error {
	r.lock.RLock()
	defer r.lock.RUnlock()

	return r.restartProcess(id)
}

func (r *restream) restartProcess(id string) error {
	task, ok := r.tasks[id]
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

func (r *restream) ReloadProcess(id string) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	err := r.reloadProcess(id)
	if err != nil {
		return err
	}

	r.save()

	return nil
}

func (r *restream) reloadProcess(id string) error {
	t, ok := r.tasks[id]
	if !ok {
		return ErrUnknownProcess
	}

	t.valid = false

	t.config = t.process.Config.Clone()

	t.config.ResolvePlaceholders(r.replace)

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
		r.stopProcess(id)
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
		r.startProcess(id)
	}

	return nil
}

func (r *restream) GetProcessState(id string) (*app.State, error) {
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

func (r *restream) GetProcessLog(id string) (*app.Log, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	task, ok := r.tasks[id]
	if !ok {
		return &app.Log{}, ErrUnknownProcess
	}

	if !task.valid {
		return &app.Log{}, nil
	}

	log := &app.Log{}

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

func (r *restream) Probe(id string) app.Probe {
	r.lock.RLock()

	appprobe := app.Probe{}

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
		StaleTimeout:   20 * time.Second,
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

func (r *restream) GetPlayout(id, inputid string) (string, error) {
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

func (r *restream) SetProcessMetadata(id, key string, data interface{}) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	if len(key) == 0 {
		return fmt.Errorf("a key for storing the data has to be provided")
	}

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

func (r *restream) GetProcessMetadata(id, key string) (interface{}, error) {
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
	if ok {
		return data, nil
	}

	return nil, nil
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
	if ok {
		return data, nil
	}

	return nil, nil
}
