package store

import (
	"fmt"
	"time"

	"github.com/datarhei/core/v16/restream/app"
)

func (s *store) addProcess(cmd CommandAddProcess) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	id := cmd.Config.ProcessID().String()

	if cmd.Config.LimitCPU <= 0 || cmd.Config.LimitMemory <= 0 {
		return fmt.Errorf("the process with the ID '%s' must have limits defined", id)
	}

	_, ok := s.data.Process[id]
	if ok {
		return fmt.Errorf("the process with the ID '%s' already exists", id)
	}

	order := "stop"
	if cmd.Config.Autostart {
		order = "start"
		cmd.Config.Autostart = false
	}

	now := time.Now()
	s.data.Process[id] = Process{
		CreatedAt: now,
		UpdatedAt: now,
		Config:    cmd.Config,
		Order:     order,
		Metadata:  map[string]interface{}{},
	}

	return nil
}

func (s *store) removeProcess(cmd CommandRemoveProcess) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	id := cmd.ID.String()

	_, ok := s.data.Process[id]
	if !ok {
		return fmt.Errorf("the process with the ID '%s' doesn't exist", id)
	}

	delete(s.data.Process, id)

	return nil
}

func (s *store) updateProcess(cmd CommandUpdateProcess) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	srcid := cmd.ID.String()
	dstid := cmd.Config.ProcessID().String()

	if cmd.Config.LimitCPU <= 0 || cmd.Config.LimitMemory <= 0 {
		return fmt.Errorf("the process with the ID '%s' must have limits defined", dstid)
	}

	p, ok := s.data.Process[srcid]
	if !ok {
		return fmt.Errorf("the process with the ID '%s' doesn't exists", srcid)
	}

	if p.Config.Equal(cmd.Config) {
		return nil
	}

	if srcid == dstid {
		p.UpdatedAt = time.Now()
		p.Config = cmd.Config

		s.data.Process[srcid] = p

		return nil
	}

	_, ok = s.data.Process[dstid]
	if ok {
		return fmt.Errorf("the process with the ID '%s' already exists", dstid)
	}

	now := time.Now()

	p.CreatedAt = now
	p.UpdatedAt = now
	p.Config = cmd.Config

	delete(s.data.Process, srcid)
	s.data.Process[dstid] = p

	return nil
}

func (s *store) setProcessOrder(cmd CommandSetProcessOrder) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	id := cmd.ID.String()

	p, ok := s.data.Process[id]
	if !ok {
		return fmt.Errorf("the process with the ID '%s' doesn't exists", cmd.ID)
	}

	p.Order = cmd.Order
	p.UpdatedAt = time.Now()

	s.data.Process[id] = p

	return nil
}

func (s *store) setProcessMetadata(cmd CommandSetProcessMetadata) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	id := cmd.ID.String()

	p, ok := s.data.Process[id]
	if !ok {
		return fmt.Errorf("the process with the ID '%s' doesn't exists", cmd.ID)
	}

	if p.Metadata == nil {
		p.Metadata = map[string]interface{}{}
	}

	if cmd.Data == nil {
		delete(p.Metadata, cmd.Key)
	} else {
		p.Metadata[cmd.Key] = cmd.Data
	}
	p.UpdatedAt = time.Now()

	s.data.Process[id] = p

	return nil
}

func (s *store) setProcessError(cmd CommandSetProcessError) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	id := cmd.ID.String()

	p, ok := s.data.Process[id]
	if !ok {
		return fmt.Errorf("the process with the ID '%s' doesn't exists", cmd.ID)
	}

	p.Error = cmd.Error

	s.data.Process[id] = p

	return nil
}

func (s *store) setProcessNodeMap(cmd CommandSetProcessNodeMap) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.data.ProcessNodeMap = cmd.Map

	return nil
}

func (s *store) ListProcesses() []Process {
	s.lock.RLock()
	defer s.lock.RUnlock()

	processes := []Process{}

	for _, p := range s.data.Process {
		processes = append(processes, Process{
			CreatedAt: p.CreatedAt,
			UpdatedAt: p.UpdatedAt,
			Config:    p.Config.Clone(),
			Order:     p.Order,
			Metadata:  p.Metadata,
			Error:     p.Error,
		})
	}

	return processes
}

func (s *store) GetProcess(id app.ProcessID) (Process, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	process, ok := s.data.Process[id.String()]
	if !ok {
		return Process{}, fmt.Errorf("not found")
	}

	return Process{
		CreatedAt: process.CreatedAt,
		UpdatedAt: process.UpdatedAt,
		Config:    process.Config.Clone(),
		Order:     process.Order,
		Metadata:  process.Metadata,
		Error:     process.Error,
	}, nil
}

func (s *store) GetProcessNodeMap() map[string]string {
	s.lock.RLock()
	defer s.lock.RUnlock()

	m := map[string]string{}

	for key, value := range s.data.ProcessNodeMap {
		m[key] = value
	}

	return m
}
