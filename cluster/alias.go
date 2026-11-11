package cluster

import (
	"errors"
	"fmt"

	"github.com/datarhei/core/v16/alias"
	clusteralias "github.com/datarhei/core/v16/cluster/alias"
	"github.com/datarhei/core/v16/cluster/store"
)

// Aliases returns an alias manager that is backed by the cluster store. The manager
// is read-only, use AliasAdd, AliasUpdate, AliasRemove, and AliasesSet for modifications.
func (c *cluster) Aliases() (alias.Manager, error) {
	m, err := clusteralias.New(c.store, c.logger.WithComponent("Alias"))
	if err != nil {
		return nil, fmt.Errorf("cluster aliases: %w", err)
	}

	return m, nil
}

func invalidAlias(err error) error {
	return fmt.Errorf("%w%w", err, store.ErrBadRequest)
}

func (c *cluster) AliasAdd(origin string, rule alias.Rule) error {
	if err := alias.Validate(rule); err != nil {
		return invalidAlias(err)
	}

	if !c.IsRaftLeader() {
		return c.forwarder.AliasAdd(origin, rule)
	}

	cmd := &store.Command{
		Operation: store.OpAddAlias,
		Data: &store.CommandAddAlias{
			Rule: rule,
		},
	}

	return c.applyCommand(cmd)
}

func (c *cluster) AliasUpdate(origin, pattern string, rule alias.Rule) error {
	if err := alias.Validate(rule); err != nil {
		return invalidAlias(err)
	}

	if !c.IsRaftLeader() {
		return c.forwarder.AliasUpdate(origin, pattern, rule)
	}

	cmd := &store.Command{
		Operation: store.OpUpdateAlias,
		Data: &store.CommandUpdateAlias{
			Pattern: pattern,
			Rule:    rule,
		},
	}

	return c.applyCommand(cmd)
}

func (c *cluster) AliasRemove(origin, pattern string) error {
	if !c.IsRaftLeader() {
		return c.forwarder.AliasRemove(origin, pattern)
	}

	cmd := &store.Command{
		Operation: store.OpRemoveAlias,
		Data: &store.CommandRemoveAlias{
			Pattern: pattern,
		},
	}

	return c.applyCommand(cmd)
}

func (c *cluster) AliasesSet(origin string, rules []alias.Rule) error {
	if _, err := alias.Compile(rules); err != nil {
		if errors.Is(err, alias.ErrExists) {
			return fmt.Errorf("%w%w", err, store.ErrConflict)
		}

		return invalidAlias(err)
	}

	if !c.IsRaftLeader() {
		return c.forwarder.AliasesSet(origin, rules)
	}

	cmd := &store.Command{
		Operation: store.OpSetAliases,
		Data: &store.CommandSetAliases{
			Rules: rules,
		},
	}

	return c.applyCommand(cmd)
}
