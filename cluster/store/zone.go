package store

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/datarhei/core/v16/restream/app"
	"github.com/datarhei/core/v16/zone"
)

// zoneError adds the matching store error to an error from the zone package such
// that it can be mapped to a status code.
func zoneError(err error) error {
	switch {
	case errors.Is(err, zone.ErrInvalid), errors.Is(err, zone.ErrExceeded):
		return fmt.Errorf("%w%w", err, ErrBadRequest)
	case errors.Is(err, zone.ErrExists), errors.Is(err, zone.ErrInUse):
		return fmt.Errorf("%w%w", err, ErrConflict)
	case errors.Is(err, zone.ErrNotFound):
		return fmt.Errorf("%w%w", err, ErrNotFound)
	}

	return err
}

// zoneGet returns the zone with the given ID. The default zone always exists, it is
// unlimited as long as it hasn't been updated. The lock must be held.
func (s *store) zoneGet(id string) (zone.Zone, bool) {
	id = zone.NormalizeID(id)

	z, ok := s.data.Zones[id]
	if !ok && id == zone.DefaultID {
		return zone.Zone{ID: zone.DefaultID}, true
	}

	return z, ok
}

// zoneConfigs returns the configs of all processes, sorted by their ID. The order is
// deterministic, such that the sum of the claimed resources is the same on every node.
// The lock must be held.
func (s *store) zoneConfigs() []*app.Config {
	ids := slices.Sorted(maps.Keys(s.data.Process))

	configs := make([]*app.Config, 0, len(ids))
	for _, id := range ids {
		configs = append(configs, s.data.Process[id].Config)
	}

	return configs
}

// zoneCheck returns an error if the zone of next doesn't exist or can't afford that a process
// claims next instead of prev. prev is nil for a new process. The lock must be held.
func (s *store) zoneCheck(prev, next *app.Config) error {
	id := zone.NormalizeID(next.Zone)

	z, ok := s.zoneGet(id)
	if !ok {
		return fmt.Errorf("the zone '%s' doesn't exist: %w%w", id, zone.ErrNotFound, ErrBadRequest)
	}

	usage, _ := zone.Usage(id, s.zoneConfigs())

	claimed := zone.Resources{}
	if prev != nil && zone.NormalizeID(prev.Zone) == id {
		claimed = zone.ResourcesFromConfig(prev)
	}

	if err := zone.Check(z, usage, claimed, zone.ResourcesFromConfig(next)); err != nil {
		return zoneError(err)
	}

	return nil
}

func (s *store) addZone(cmd CommandAddZone) error {
	if err := zone.Validate(cmd.Zone); err != nil {
		return zoneError(err)
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	if _, ok := s.zoneGet(cmd.Zone.ID); ok {
		return zoneError(zone.ErrExists)
	}

	s.data.Zones[cmd.Zone.ID] = cmd.Zone

	return nil
}

func (s *store) updateZone(cmd CommandUpdateZone) error {
	if err := zone.Validate(cmd.Zone); err != nil {
		return zoneError(err)
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	if _, ok := s.zoneGet(cmd.Zone.ID); !ok {
		return zoneError(zone.ErrNotFound)
	}

	s.data.Zones[cmd.Zone.ID] = cmd.Zone

	return nil
}

func (s *store) removeZone(cmd CommandRemoveZone) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if _, ok := s.zoneGet(cmd.ID); !ok || len(cmd.ID) == 0 {
		return zoneError(zone.ErrNotFound)
	}

	if cmd.ID == zone.DefaultID {
		return zoneError(fmt.Errorf("%w: the default zone can't be removed", zone.ErrInvalid))
	}

	if _, n := zone.Usage(cmd.ID, s.zoneConfigs()); n != 0 {
		return zoneError(fmt.Errorf("%w: the zone '%s' has %d processes", zone.ErrInUse, cmd.ID, n))
	}

	delete(s.data.Zones, cmd.ID)

	return nil
}

// zoneInfo returns the zone with its usage. The lock must be held.
func (s *store) zoneInfo(z zone.Zone, configs []*app.Config) zone.Info {
	usage, n := zone.Usage(z.ID, configs)

	return zone.Info{
		Zone:      z,
		Usage:     usage,
		Processes: n,
	}
}

func (s *store) ZoneList() []zone.Info {
	s.lock.RLock()
	defer s.lock.RUnlock()

	configs := s.zoneConfigs()

	ids := slices.Collect(maps.Keys(s.data.Zones))
	if !slices.Contains(ids, zone.DefaultID) {
		ids = append(ids, zone.DefaultID)
	}

	slices.Sort(ids)

	list := make([]zone.Info, 0, len(ids))
	for _, id := range ids {
		z, _ := s.zoneGet(id)
		list = append(list, s.zoneInfo(z, configs))
	}

	return list
}

func (s *store) ZoneGet(id string) (zone.Info, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	if len(id) == 0 {
		return zone.Info{}, zone.ErrNotFound
	}

	z, ok := s.zoneGet(id)
	if !ok {
		return zone.Info{}, zone.ErrNotFound
	}

	return s.zoneInfo(z, s.zoneConfigs()), nil
}
