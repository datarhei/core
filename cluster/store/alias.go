package store

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/datarhei/core/v16/alias"
)

// aliasError adds the matching store error to an error from the alias package such
// that it can be mapped to a status code.
func aliasError(err error) error {
	switch {
	case errors.Is(err, alias.ErrInvalid):
		return fmt.Errorf("%w%w", err, ErrBadRequest)
	case errors.Is(err, alias.ErrExists):
		return fmt.Errorf("%w%w", err, ErrConflict)
	case errors.Is(err, alias.ErrNotFound):
		return fmt.Errorf("%w%w", err, ErrNotFound)
	}

	return err
}

func (s *store) aliasIndex(pattern string) int {
	return slices.IndexFunc(s.data.Aliases.Rules, func(r alias.Rule) bool {
		return r.Pattern == pattern
	})
}

func (s *store) addAlias(cmd CommandAddAlias) error {
	if err := alias.Validate(cmd.Rule); err != nil {
		return aliasError(err)
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	if s.aliasIndex(cmd.Rule.Pattern) != -1 {
		return aliasError(alias.ErrExists)
	}

	s.data.Aliases.Rules = append(slices.Clone(s.data.Aliases.Rules), cmd.Rule)
	s.data.Aliases.UpdatedAt = time.Now()

	return nil
}

func (s *store) updateAlias(cmd CommandUpdateAlias) error {
	if err := alias.Validate(cmd.Rule); err != nil {
		return aliasError(err)
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	i := s.aliasIndex(cmd.Pattern)
	if i == -1 {
		return aliasError(alias.ErrNotFound)
	}

	if cmd.Rule.Pattern != cmd.Pattern && s.aliasIndex(cmd.Rule.Pattern) != -1 {
		return aliasError(alias.ErrExists)
	}

	rules := slices.Clone(s.data.Aliases.Rules)
	rules[i] = cmd.Rule

	s.data.Aliases.Rules = rules
	s.data.Aliases.UpdatedAt = time.Now()

	return nil
}

func (s *store) removeAlias(cmd CommandRemoveAlias) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	i := s.aliasIndex(cmd.Pattern)
	if i == -1 {
		return aliasError(alias.ErrNotFound)
	}

	s.data.Aliases.Rules = slices.Delete(slices.Clone(s.data.Aliases.Rules), i, i+1)
	s.data.Aliases.UpdatedAt = time.Now()

	return nil
}

func (s *store) setAliases(cmd CommandSetAliases) error {
	if _, err := alias.Compile(cmd.Rules); err != nil {
		return aliasError(err)
	}

	rules := slices.Clone(cmd.Rules)
	if rules == nil {
		rules = []alias.Rule{}
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	s.data.Aliases.Rules = rules
	s.data.Aliases.UpdatedAt = time.Now()

	return nil
}

func (s *store) AliasList() Aliases {
	s.lock.RLock()
	defer s.lock.RUnlock()

	return Aliases{
		UpdatedAt: s.data.Aliases.UpdatedAt,
		Rules:     slices.Clone(s.data.Aliases.Rules),
	}
}

func (s *store) AliasGet(pattern string) (alias.Rule, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	i := s.aliasIndex(pattern)
	if i == -1 {
		return alias.Rule{}, aliasError(alias.ErrNotFound)
	}

	return s.data.Aliases.Rules[i], nil
}
