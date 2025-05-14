package restream

import (
	"github.com/datarhei/core/v16/restream/app"
	"github.com/puzpuzpuz/xsync/v3"
)

type Storage struct {
	tasks *xsync.MapOf[app.ProcessID, *task]
}

func NewStorage() *Storage {
	m := &Storage{
		tasks: xsync.NewMapOf[app.ProcessID, *task](),
	}

	return m
}

func (m *Storage) Range(onlyValid bool, f func(key app.ProcessID, value *task, token string) bool) {
	m.tasks.Range(func(id app.ProcessID, task *task) bool {
		token := task.RLock()
		if onlyValid && !task.IsValid() {
			task.Release(token)
			return true
		}
		return f(id, task, token)
	})
}

func (m *Storage) Store(id app.ProcessID, t *task) {
	t, ok := m.tasks.LoadAndStore(id, t)
	if ok {
		t.Destroy()
	}
}

func (m *Storage) LoadOrStore(id app.ProcessID, t *task) (*task, bool) {
	return m.tasks.LoadOrStore(id, t)
}

func (m *Storage) Has(id app.ProcessID) bool {
	_, hasTask := m.tasks.Load(id)

	return hasTask
}

func (m *Storage) Load(id app.ProcessID) (*task, string, bool) {
	task, ok := m.tasks.Load(id)
	if !ok {
		return nil, "", false
	}

	token := task.RLock()
	if !task.IsValid() {
		task.Release(token)
		return nil, "", false
	}
	return task, token, true
}

func (m *Storage) LoadAndLock(id app.ProcessID) (*task, bool) {
	task, ok := m.tasks.Load(id)
	if !ok {
		return nil, false
	}

	task.Lock()
	if !task.IsValid() {
		task.Unlock()
		return nil, false
	}
	return task, true
}

func (m *Storage) Delete(id app.ProcessID) bool {
	if t, ok := m.tasks.Load(id); ok {
		m.tasks.Delete(id)
		t.Destroy()
		return true
	}

	return false
}

func (m *Storage) Size() int {
	return m.tasks.Size()
}

func (m *Storage) Clear() {
	m.tasks.Range(func(_ app.ProcessID, t *task) bool {
		t.Destroy()

		return true
	})

	m.tasks.Clear()
}
