// Package zone manages zones. A zone is a named quota for the resources that the
// processes assigned to it may claim in total. The claimed resources of a process
// are the limits from its config, regardless of whether the process is running.
//
// The zone with the ID "$default" is the default zone. It always exists and can't be
// removed. Processes without a zone belong to the default zone. A limit of 0 means
// unlimited.
package zone

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/io/fs"
	"github.com/datarhei/core/v16/log"
	"github.com/datarhei/core/v16/restream/app"
)

var (
	ErrNotFound = errors.New("zone not found")
	ErrExists   = errors.New("zone already exists")
	ErrInvalid  = errors.New("invalid zone")
	ErrInUse    = errors.New("zone is in use")
	ErrExceeded = errors.New("zone limits exceeded")
	ErrReadOnly = errors.New("zones are read-only")
)

// Resources are amounts of resources, either the limits of a zone, the usage of a zone,
// or the resources a process claims.
type Resources struct {
	CPU        float64 `json:"cpu_usage"`        // percent 0-100*ncpu
	Memory     uint64  `json:"memory_bytes"`     // bytes
	GPUUsage   float64 `json:"gpu_usage"`        // percent 0-100
	GPUEncoder float64 `json:"gpu_encoder"`      // percent 0-100
	GPUDecoder float64 `json:"gpu_decoder"`      // percent 0-100
	GPUMemory  uint64  `json:"gpu_memory_bytes"` // bytes
}

// ResourcesFromConfig returns the resources a process with this config claims.
func ResourcesFromConfig(config *app.Config) Resources {
	return Resources{
		CPU:        config.LimitCPU,
		Memory:     config.LimitMemory,
		GPUUsage:   config.LimitGPU.Usage,
		GPUEncoder: config.LimitGPU.Encoder,
		GPUDecoder: config.LimitGPU.Decoder,
		GPUMemory:  config.LimitGPU.Memory,
	}
}

// Add returns the sum of both resources.
func (r Resources) Add(a Resources) Resources {
	return Resources{
		CPU:        r.CPU + a.CPU,
		Memory:     r.Memory + a.Memory,
		GPUUsage:   r.GPUUsage + a.GPUUsage,
		GPUEncoder: r.GPUEncoder + a.GPUEncoder,
		GPUDecoder: r.GPUDecoder + a.GPUDecoder,
		GPUMemory:  r.GPUMemory + a.GPUMemory,
	}
}

// Sub returns the difference of both resources. The values don't drop below 0.
func (r Resources) Sub(a Resources) Resources {
	return Resources{
		CPU:        max(r.CPU-a.CPU, 0),
		Memory:     r.Memory - min(r.Memory, a.Memory),
		GPUUsage:   max(r.GPUUsage-a.GPUUsage, 0),
		GPUEncoder: max(r.GPUEncoder-a.GPUEncoder, 0),
		GPUDecoder: max(r.GPUDecoder-a.GPUDecoder, 0),
		GPUMemory:  r.GPUMemory - min(r.GPUMemory, a.GPUMemory),
	}
}

// resource describes one of the resources, for checking it and for error messages.
type resource struct {
	name  string // name as in the API
	value func(r Resources) float64
	unit  float64 // divisor for the error message, e.g. bytes to megabytes
}

var resourceList = []resource{
	{"cpu_usage", func(r Resources) float64 { return r.CPU }, 1},
	{"memory_mbytes", func(r Resources) float64 { return float64(r.Memory) }, 1024 * 1024},
	{"gpu_usage", func(r Resources) float64 { return r.GPUUsage }, 1},
	{"gpu_encoder", func(r Resources) float64 { return r.GPUEncoder }, 1},
	{"gpu_decoder", func(r Resources) float64 { return r.GPUDecoder }, 1},
	{"gpu_memory_mbytes", func(r Resources) float64 { return float64(r.GPUMemory) }, 1024 * 1024},
}

// Zone is a zone with its limits. The ID is the name of the zone.
type Zone struct {
	ID     string    `json:"id"`
	Limits Resources `json:"limits"`
}

// DefaultID is the ID of the default zone. It contains a character that is not allowed
// in the IDs of other zones.
const DefaultID = "$default"

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// normalizeID returns the ID of the default zone for an empty ID, e.g. of a process
// without a zone.
func normalizeID(id string) string {
	if len(id) == 0 {
		return DefaultID
	}

	return id
}

// Validate checks whether the zone is valid.
func Validate(z Zone) error {
	if z.ID != DefaultID && !idPattern.MatchString(z.ID) {
		return fmt.Errorf("%w: the ID must not be empty and may only contain the characters a-z, A-Z, 0-9, _ and -", ErrInvalid)
	}

	for _, r := range resourceList {
		v := r.value(z.Limits)
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return fmt.Errorf("%w: the limit %s must be a number of at least 0", ErrInvalid, r.name)
		}
	}

	return nil
}

// Check returns an error if the zone with the given current usage can't afford that a
// process claims next instead of prev. prev are the resources the process already claims
// in this zone, they are part of the usage. A limit of 0 is unlimited. A resource that
// doesn't increase is always allowed, even if the usage is already above the limit.
func Check(z Zone, usage, prev, next Resources) error {
	total := usage.Sub(prev).Add(next)

	exceeded := []string{}

	for _, r := range resourceList {
		limit := r.value(z.Limits)
		if limit == 0 {
			continue
		}

		if r.value(next) <= r.value(prev) {
			continue
		}

		if r.value(total) > limit {
			exceeded = append(exceeded, fmt.Sprintf("%s %s of %s", r.name, formatValue(r.value(total)/r.unit), formatValue(limit/r.unit)))
		}
	}

	if len(exceeded) != 0 {
		return fmt.Errorf("%w: zone '%s': %s", ErrExceeded, z.ID, strings.Join(exceeded, ", "))
	}

	return nil
}

func formatValue(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// Claim are the resources a process claims in a zone.
type Claim struct {
	Zone      string // ID of the zone, an empty ID is the default zone
	Resources Resources
}

// ClaimFromConfig returns the claim of a process with this config.
func ClaimFromConfig(config *app.Config) *Claim {
	return &Claim{
		Zone:      config.Zone,
		Resources: ResourcesFromConfig(config),
	}
}

// Info is a zone with its current usage.
type Info struct {
	Zone
	Usage     Resources
	Processes int // Number of processes in the zone
}

// Manager manages zones and the resources the processes claim in them.
type Manager interface {
	// List returns all zones, sorted by their ID.
	List() []Info

	// Get returns the zone with the given ID.
	Get(id string) (Info, error)

	// Add adds a zone.
	Add(z Zone) error

	// Update replaces the limits of a zone.
	Update(z Zone) error

	// Remove removes a zone. A zone with processes or the default zone can't be removed.
	Remove(id string) error

	// Acquire claims the resources of next in its zone, if the zone can afford them.
	// prev are the resources the process claimed so far, they are released. For a new
	// process, prev is nil.
	Acquire(prev, next *Claim) error

	// Release releases the resources of a process, e.g. after it has been removed.
	Release(c *Claim)

	// Register claims the resources without checking the limits, e.g. for existing
	// processes when they are loaded.
	Register(c *Claim)
}

type Config struct {
	Filesystem fs.Filesystem
	Filepath   string // Path to the file on the filesystem
	Logger     log.Logger
}

type usage struct {
	resources Resources
	processes int
}

type manager struct {
	fs       fs.Filesystem
	filepath string
	logger   log.Logger

	zones map[string]Zone
	usage map[string]usage
	lock  sync.RWMutex
}

const version = 1

type data struct {
	Version int    `json:"version"`
	Zones   []Zone `json:"zones"`
}

// New returns a new zone manager. The zones are loaded from the file, if it exists.
func New(config Config) (Manager, error) {
	m := &manager{
		fs:       config.Filesystem,
		filepath: config.Filepath,
		logger:   config.Logger,
		zones:    map[string]Zone{},
		usage:    map[string]usage{},
	}

	if m.fs == nil {
		return nil, fmt.Errorf("no filesystem provided")
	}

	if len(m.filepath) == 0 {
		return nil, fmt.Errorf("no file path provided")
	}

	if m.logger == nil {
		m.logger = log.New("")
	}

	if err := m.load(); err != nil {
		return nil, err
	}

	return m, nil
}

func (m *manager) info(z Zone) Info {
	u := m.usage[z.ID]

	return Info{
		Zone:      z,
		Usage:     u.resources,
		Processes: u.processes,
	}
}

func (m *manager) List() []Info {
	m.lock.RLock()
	defer m.lock.RUnlock()

	list := make([]Info, 0, len(m.zones))
	for _, z := range m.zones {
		list = append(list, m.info(z))
	}

	slices.SortFunc(list, func(a, b Info) int {
		return strings.Compare(a.ID, b.ID)
	})

	return list
}

func (m *manager) Get(id string) (Info, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	z, ok := m.zones[id]
	if !ok {
		return Info{}, ErrNotFound
	}

	return m.info(z), nil
}

func (m *manager) Add(z Zone) error {
	if err := Validate(z); err != nil {
		return err
	}

	m.lock.Lock()
	defer m.lock.Unlock()

	if _, ok := m.zones[z.ID]; ok {
		return ErrExists
	}

	return m.commit(func(zones map[string]Zone) {
		zones[z.ID] = z
	})
}

func (m *manager) Update(z Zone) error {
	if err := Validate(z); err != nil {
		return err
	}

	m.lock.Lock()
	defer m.lock.Unlock()

	if _, ok := m.zones[z.ID]; !ok {
		return ErrNotFound
	}

	return m.commit(func(zones map[string]Zone) {
		zones[z.ID] = z
	})
}

func (m *manager) Remove(id string) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	if _, ok := m.zones[id]; !ok {
		return ErrNotFound
	}

	if id == DefaultID {
		return fmt.Errorf("%w: the default zone can't be removed", ErrInvalid)
	}

	if n := m.usage[id].processes; n != 0 {
		return fmt.Errorf("%w: the zone '%s' has %d processes", ErrInUse, id, n)
	}

	return m.commit(func(zones map[string]Zone) {
		delete(zones, id)
	})
}

func (m *manager) Acquire(prev, next *Claim) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	id := normalizeID(next.Zone)

	z, ok := m.zones[id]
	if !ok {
		return fmt.Errorf("the zone '%s' doesn't exist: %w", id, ErrNotFound)
	}

	claimed := Resources{}
	if prev != nil && normalizeID(prev.Zone) == id {
		claimed = prev.Resources
	}

	if err := Check(z, m.usage[id].resources, claimed, next.Resources); err != nil {
		return err
	}

	if prev != nil {
		m.release(prev)
	}

	m.register(next)

	return nil
}

func (m *manager) Release(c *Claim) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.release(c)
}

func (m *manager) Register(c *Claim) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.register(c)
}

// register adds the claim to the usage of its zone. The write lock must be held.
func (m *manager) register(c *Claim) {
	id := normalizeID(c.Zone)

	u := m.usage[id]
	u.resources = u.resources.Add(c.Resources)
	u.processes++
	m.usage[id] = u
}

// release removes the claim from the usage of its zone. The write lock must be held.
func (m *manager) release(c *Claim) {
	id := normalizeID(c.Zone)

	u, ok := m.usage[id]
	if !ok {
		return
	}

	u.resources = u.resources.Sub(c.Resources)
	u.processes--

	// Remove the entry without processes, such that no rounding errors remain.
	if u.processes <= 0 {
		delete(m.usage, id)
		return
	}

	m.usage[id] = u
}

// commit applies the change to a copy of the zones, stores them, and replaces the current
// zones only if storing was successful. The write lock must be held.
func (m *manager) commit(change func(zones map[string]Zone)) error {
	zones := maps.Clone(m.zones)

	change(zones)

	if err := m.save(zones); err != nil {
		return err
	}

	m.zones = zones

	return nil
}

func (m *manager) save(zones map[string]Zone) error {
	d := data{
		Version: version,
		Zones:   make([]Zone, 0, len(zones)),
	}

	for _, z := range zones {
		d.Zones = append(d.Zones, z)
	}

	slices.SortFunc(d.Zones, func(a, b Zone) int {
		return strings.Compare(a.ID, b.ID)
	})

	jsondata, err := json.MarshalIndent(d, "", "    ")
	if err != nil {
		return err
	}

	if _, _, err := m.fs.WriteFileSafe(m.filepath, jsondata); err != nil {
		return fmt.Errorf("write zones to %s: %w", m.filepath, err)
	}

	m.logger.Debug().WithField("path", m.filepath).Log("Stored zones")

	return nil
}

func (m *manager) load() error {
	// The default zone always exists.
	m.zones[DefaultID] = Zone{ID: DefaultID}

	if _, err := m.fs.Stat(m.filepath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}

		return err
	}

	jsondata, err := m.fs.ReadFile(m.filepath)
	if err != nil {
		return fmt.Errorf("read zones from %s: %w", m.filepath, err)
	}

	d := data{}

	if err := json.Unmarshal(jsondata, &d); err != nil {
		return fmt.Errorf("parse zones from %s: %w", m.filepath, json.FormatError(jsondata, err))
	}

	if d.Version != version {
		return fmt.Errorf("unsupported version %d of zones in %s, expected %d", d.Version, m.filepath, version)
	}

	seen := map[string]struct{}{}

	for _, z := range d.Zones {
		if err := Validate(z); err != nil {
			m.logger.Warn().WithError(err).WithField("id", z.ID).Log("Ignoring zone")
			continue
		}

		if _, ok := seen[z.ID]; ok {
			m.logger.Warn().WithField("id", z.ID).Log("Ignoring duplicate zone")
			continue
		}

		seen[z.ID] = struct{}{}
		m.zones[z.ID] = z
	}

	m.logger.Debug().WithField("path", m.filepath).WithField("zones", len(m.zones)).Log("Loaded zones")

	return nil
}
