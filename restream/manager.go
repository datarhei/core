package restream

import (
	"sync"

	"github.com/datarhei/core/v16/restream/app"
	"github.com/puzpuzpuz/xsync/v3"
)

type metatask struct {
	task *task
	lock sync.RWMutex
}

type Storage struct {
	lock  *xsync.RBMutex
	tasks *xsync.MapOf[app.ProcessID, *metatask]
}

func NewStorage() *Storage {
	m := &Storage{
		lock:  xsync.NewRBMutex(),
		tasks: xsync.NewMapOf[app.ProcessID, *metatask](),
	}

	return m
}

// Range calls f sequentially for each key and value present in the map. If f returns false, range stops the iteration.
//
// Range does not necessarily correspond to any consistent snapshot of the Map's contents: no key will be visited more than once,
// but if the value for any key is stored or deleted concurrently, Range may reflect any mapping for that key from any point during the Range call.
//
// It is safe to modify the map while iterating it, including entry creation, modification and deletion. However,
// the concurrent modification rule apply, i.e. the changes may be not reflected in the subsequently iterated entries.
func (m *Storage) Range(f func(key app.ProcessID, value *task) bool) {
	m.tasks.Range(func(id app.ProcessID, mt *metatask) bool {
		return f(id, mt.task)
	})
}

// Store sets the value for a key.
func (m *Storage) LoadAndStore(id app.ProcessID, t *task) (*task, bool) {
	m.lock.Lock()
	defer m.lock.Unlock()

	mt, ok := m.tasks.Load(id)
	if ok {
		old := mt.task
		mt.task = t

		return old, true
	}
	mt = &metatask{
		task: t,
	}
	m.tasks.Store(id, mt)

	return t, false
}

// LoadOrStore returns the existing value for the key if present. Otherwise, it stores and returns the given value.
// The loaded result is true if the value was loaded, false if stored.
func (m *Storage) LoadOrStore(id app.ProcessID, t *task) (*task, bool) {
	m.lock.Lock()
	defer m.lock.Unlock()

	mt, ok := m.tasks.Load(id)
	if ok {
		return mt.task, true
	}
	mt = &metatask{
		task: t,
	}
	m.tasks.Store(id, mt)

	return t, false
}

// Has returns whether a value is stored in the map.
func (m *Storage) Has(id app.ProcessID) bool {
	_, hasTask := m.tasks.Load(id)

	return hasTask
}

// LoadUnsafe returns the value stored in the map for a key, or zero value of type V if no value is present.
// The ok result indicates whether value was found in the map.
func (m *Storage) LoadUnsafe(id app.ProcessID) (*task, bool) {
	mt, ok := m.tasks.Load(id)
	if !ok {
		return nil, false
	}

	return mt.task, true
}

func (m *Storage) LoadAndLock(id app.ProcessID) (*task, bool) {
	mt, ok := m.tasks.Load(id)
	if !ok {
		return nil, false
	}

	mt.lock.Lock()

	return mt.task, true
}

func (m *Storage) Unlock(id app.ProcessID) {
	mt, ok := m.tasks.Load(id)
	if !ok {
		return
	}

	mt.lock.Unlock()
}

// Delete deletes the value for a key.
func (m *Storage) LoadAndDelete(id app.ProcessID) (*task, bool) {
	m.lock.Lock()
	defer m.lock.Unlock()

	if mt, ok := m.tasks.LoadAndDelete(id); ok {
		return mt.task, true
	}

	return nil, false
}

// Size returns current size of the map.
func (m *Storage) Size() int {
	return m.tasks.Size()
}

// Clear deletes all keys and values currently stored in the map.
func (m *Storage) Clear(f func(key app.ProcessID, value *task) bool) {
	m.tasks.Range(func(id app.ProcessID, mt *metatask) bool {
		return f(id, mt.task)
	})

	m.tasks.Clear()
}
