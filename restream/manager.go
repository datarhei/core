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

func (m *Storage) Range(f func(key app.ProcessID, value *task) bool) {
	m.tasks.Range(f)
}

func (m *Storage) Store(id app.ProcessID, t *task) {
	m.tasks.Store(id, t)
}

func (m *Storage) LoadOrStore(id app.ProcessID, t *task) (*task, bool) {
	return m.tasks.LoadOrStore(id, t)
}

func (m *Storage) Has(id app.ProcessID) bool {
	_, hasTask := m.Load(id)

	return hasTask
}

func (m *Storage) Load(id app.ProcessID) (*task, bool) {
	return m.tasks.Load(id)
}

func (m *Storage) Delete(id app.ProcessID) bool {
	if t, ok := m.Load(id); ok {
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
