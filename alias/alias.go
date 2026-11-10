// Package alias manages alias rules. An alias rule maps paths that match a
// regular expression to a target path. The rules can be changed at runtime and
// are persisted as JSON on a filesystem.
package alias

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/io/fs"
	"github.com/datarhei/core/v16/log"
)

var (
	ErrNotFound = errors.New("alias not found")
	ErrExists   = errors.New("alias already exists")
	ErrInvalid  = errors.New("invalid alias")
)

// Rule is an alias rule. The pattern is the ID of the rule.
type Rule struct {
	// Pattern is a regular expression that is matched against the path.
	Pattern string `json:"pattern"`

	// Target is the path the matching path is replaced with. It can reference
	// capture groups of the pattern with $1, ${1}, or ${name}.
	Target string `json:"target"`
}

// Aliaser resolves aliases.
type Aliaser interface {
	// Alias returns the target path of the first rule that matches the path.
	// If no rule matches, false is returned.
	Alias(path string) (string, bool)
}

// Manager manages alias rules. The rules are evaluated in the order they have been added.
type Manager interface {
	Aliaser

	// Add appends a rule.
	Add(rule Rule) error

	// Update replaces the rule with the given pattern. The position of the rule is kept.
	Update(pattern string, rule Rule) error

	// Remove removes the rule with the given pattern.
	Remove(pattern string) error

	// Get returns the rule with the given pattern.
	Get(pattern string) (Rule, error)

	// List returns all rules in the order they are evaluated.
	List() []Rule
}

type Config struct {
	Filesystem fs.Filesystem
	Filepath   string // Path to the file on the filesystem
	Logger     log.Logger
}

type rule struct {
	Rule
	regexp *regexp.Regexp
}

type manager struct {
	fs       fs.Filesystem
	filepath string
	logger   log.Logger

	rules []rule
	lock  sync.RWMutex
}

const version = 1

type data struct {
	Version int    `json:"version"`
	Rules   []Rule `json:"rules"`
}

// New returns a new alias manager. The rules are loaded from the file, if it exists.
func New(config Config) (Manager, error) {
	m := &manager{
		fs:       config.Filesystem,
		filepath: config.Filepath,
		logger:   config.Logger,
		rules:    []rule{},
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

func compile(r Rule) (rule, error) {
	if len(r.Pattern) == 0 {
		return rule{}, fmt.Errorf("%w: the pattern is missing", ErrInvalid)
	}

	re, err := regexp.Compile(r.Pattern)
	if err != nil {
		return rule{}, fmt.Errorf("%w: the pattern is not a valid regular expression: %w", ErrInvalid, err)
	}

	if !strings.HasPrefix(r.Target, "/") {
		return rule{}, fmt.Errorf("%w: the target must be an absolute path", ErrInvalid)
	}

	return rule{
		Rule:   r,
		regexp: re,
	}, nil
}

func (m *manager) Alias(path string) (string, bool) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	for _, r := range m.rules {
		match := r.regexp.FindStringSubmatchIndex(path)
		if match == nil {
			continue
		}

		return string(r.regexp.ExpandString(nil, r.Target, path, match)), true
	}

	return "", false
}

func (m *manager) index(pattern string) int {
	return slices.IndexFunc(m.rules, func(r rule) bool {
		return r.Pattern == pattern
	})
}

func (m *manager) Add(r Rule) error {
	cr, err := compile(r)
	if err != nil {
		return err
	}

	m.lock.Lock()
	defer m.lock.Unlock()

	if m.index(r.Pattern) != -1 {
		return ErrExists
	}

	rules := append(slices.Clone(m.rules), cr)

	return m.commit(rules)
}

func (m *manager) Update(pattern string, r Rule) error {
	cr, err := compile(r)
	if err != nil {
		return err
	}

	m.lock.Lock()
	defer m.lock.Unlock()

	i := m.index(pattern)
	if i == -1 {
		return ErrNotFound
	}

	if r.Pattern != pattern && m.index(r.Pattern) != -1 {
		return ErrExists
	}

	rules := slices.Clone(m.rules)
	rules[i] = cr

	return m.commit(rules)
}

func (m *manager) Remove(pattern string) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	i := m.index(pattern)
	if i == -1 {
		return ErrNotFound
	}

	rules := slices.Delete(slices.Clone(m.rules), i, i+1)

	return m.commit(rules)
}

func (m *manager) Get(pattern string) (Rule, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	i := m.index(pattern)
	if i == -1 {
		return Rule{}, ErrNotFound
	}

	return m.rules[i].Rule, nil
}

func (m *manager) List() []Rule {
	m.lock.RLock()
	defer m.lock.RUnlock()

	rules := make([]Rule, 0, len(m.rules))
	for _, r := range m.rules {
		rules = append(rules, r.Rule)
	}

	return rules
}

// commit stores the rules and replaces the current rules only if storing was successful.
// The write lock must be held.
func (m *manager) commit(rules []rule) error {
	if err := m.save(rules); err != nil {
		return err
	}

	m.rules = rules

	return nil
}

func (m *manager) save(rules []rule) error {
	d := data{
		Version: version,
		Rules:   make([]Rule, 0, len(rules)),
	}

	for _, r := range rules {
		d.Rules = append(d.Rules, r.Rule)
	}

	jsondata, err := json.MarshalIndent(d, "", "    ")
	if err != nil {
		return err
	}

	if _, _, err := m.fs.WriteFileSafe(m.filepath, jsondata); err != nil {
		return fmt.Errorf("write aliases to %s: %w", m.filepath, err)
	}

	m.logger.Debug().WithField("path", m.filepath).Log("Stored aliases")

	return nil
}

func (m *manager) load() error {
	if _, err := m.fs.Stat(m.filepath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}

		return err
	}

	jsondata, err := m.fs.ReadFile(m.filepath)
	if err != nil {
		return fmt.Errorf("read aliases from %s: %w", m.filepath, err)
	}

	d := data{}

	if err := json.Unmarshal(jsondata, &d); err != nil {
		return fmt.Errorf("parse aliases from %s: %w", m.filepath, json.FormatError(jsondata, err))
	}

	if d.Version != version {
		return fmt.Errorf("unsupported version %d of aliases in %s, expected %d", d.Version, m.filepath, version)
	}

	rules := []rule{}

	for _, r := range d.Rules {
		cr, err := compile(r)
		if err != nil {
			m.logger.Warn().WithError(err).WithField("pattern", r.Pattern).Log("Ignoring alias")
			continue
		}

		if slices.ContainsFunc(rules, func(x rule) bool { return x.Pattern == r.Pattern }) {
			m.logger.Warn().WithField("pattern", r.Pattern).Log("Ignoring duplicate alias")
			continue
		}

		rules = append(rules, cr)
	}

	m.rules = rules

	m.logger.Debug().WithField("path", m.filepath).WithField("rules", len(rules)).Log("Loaded aliases")

	return nil
}
