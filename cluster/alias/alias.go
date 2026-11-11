// Package alias provides an alias manager that is backed by the cluster store.
package alias

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/datarhei/core/v16/alias"
	"github.com/datarhei/core/v16/cluster/store"
	"github.com/datarhei/core/v16/log"
)

// ErrClusterMode is returned for any modification. In cluster mode the aliases
// can only be modified through the cluster.
var ErrClusterMode = fmt.Errorf("%w: not available in cluster mode", alias.ErrReadOnly)

type manager struct {
	store  store.Store
	logger log.Logger

	dirty   atomic.Bool
	lock    sync.Mutex
	ruleset atomic.Pointer[alias.Ruleset]
}

// New returns an alias manager that reads the aliases from the cluster store.
// All modifications return ErrClusterMode.
func New(store store.Store, logger log.Logger) (alias.Manager, error) {
	if store == nil {
		return nil, fmt.Errorf("no store provided")
	}

	m := &manager{
		store:  store,
		logger: logger,
	}

	if m.logger == nil {
		m.logger = log.New("")
	}

	m.ruleset.Store(&alias.Ruleset{})
	m.dirty.Store(true)

	store.OnApply(m.apply)

	return m, nil
}

// apply is called with the store lock held, it must not access the store.
func (m *manager) apply(op store.Operation) {
	switch op {
	case store.OpAddAlias, store.OpUpdateAlias, store.OpRemoveAlias, store.OpSetAliases:
		m.dirty.Store(true)
	}
}

// reload compiles the aliases from the store if they have been changed.
func (m *manager) reload() *alias.Ruleset {
	if !m.dirty.Load() {
		return m.ruleset.Load()
	}

	m.lock.Lock()
	defer m.lock.Unlock()

	if !m.dirty.Swap(false) {
		return m.ruleset.Load()
	}

	rules := m.store.AliasList().Rules

	ruleset, err := alias.Compile(rules)
	if err != nil {
		// The store only accepts valid rules. Skip invalid rules anyways.
		m.logger.Warn().WithError(err).Log("Invalid aliases in the cluster store")

		valid := []alias.Rule{}
		for _, r := range rules {
			if _, err := alias.Compile(append(valid, r)); err != nil {
				m.logger.Warn().WithError(err).WithField("pattern", r.Pattern).Log("Ignoring alias")
				continue
			}

			valid = append(valid, r)
		}

		ruleset, _ = alias.Compile(valid)
	}

	m.ruleset.Store(ruleset)

	m.logger.Debug().WithField("rules", len(ruleset.Rules())).Log("Reloaded aliases")

	return ruleset
}

func (m *manager) Alias(path string) (string, bool) {
	return m.reload().Alias(path)
}

func (m *manager) Add(rule alias.Rule) error {
	return ErrClusterMode
}

func (m *manager) Update(pattern string, rule alias.Rule) error {
	return ErrClusterMode
}

func (m *manager) Remove(pattern string) error {
	return ErrClusterMode
}

func (m *manager) Set(rules []alias.Rule) error {
	return ErrClusterMode
}

func (m *manager) Get(pattern string) (alias.Rule, error) {
	r, err := m.store.AliasGet(pattern)
	if err != nil {
		return alias.Rule{}, alias.ErrNotFound
	}

	return r, nil
}

func (m *manager) List() []alias.Rule {
	return m.store.AliasList().Rules
}
