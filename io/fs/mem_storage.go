package fs

import (
	"sync"

	"github.com/datarhei/core/v16/mem"
	"github.com/dolthub/swiss"
	"github.com/puzpuzpuz/xsync/v3"
)

type memStorage interface {
	// Delete deletes a file from the storage.
	Delete(key string) (file *memFile, ok bool)

	// Store stores a file to the storage. If there's already a file with
	// the same key, that value will be returned and replaced with the
	// new file.
	Store(key string, file *memFile) (oldfile *memFile, ok bool)

	// Load loads a file from the storage. This is a references to the file,
	// i.e. all changes to the file will be reflected on the storage.
	Load(key string) (file *memFile, ok bool)

	// LoadAndCopy loads a file from the storage. This is a copy of file
	// metadata and content.
	LoadAndCopy(key string) (file *memFile, ok bool)

	// Has checks whether a file exists at path.
	Has(key string) bool

	// Range ranges over all files on the storage. The callback needs to return
	// false in order to stop the iteration.
	Range(f func(key string, file *memFile) bool)
}

type mapOfStorage struct {
	lock  *xsync.RBMutex
	files *xsync.MapOf[string, *memFile]
}

func newMapOfStorage() memStorage {
	m := &mapOfStorage{
		lock:  xsync.NewRBMutex(),
		files: xsync.NewMapOf[string, *memFile](),
	}

	return m
}

func (m *mapOfStorage) Delete(key string) (*memFile, bool) {
	m.lock.Lock()
	defer m.lock.Unlock()

	return m.files.LoadAndDelete(key)
}

func (m *mapOfStorage) Store(key string, value *memFile) (*memFile, bool) {
	m.lock.Lock()
	defer m.lock.Unlock()

	return m.files.LoadAndStore(key, value)
}

func (m *mapOfStorage) Load(key string) (*memFile, bool) {
	token := m.lock.RLock()
	defer m.lock.RUnlock(token)

	return m.files.Load(key)
}

func (m *mapOfStorage) LoadAndCopy(key string) (*memFile, bool) {
	token := m.lock.RLock()
	defer m.lock.RUnlock(token)

	file, ok := m.files.Load(key)
	if !ok {
		return nil, false
	}

	newFile := &memFile{
		memFileInfo: memFileInfo{
			name:    file.name,
			size:    file.size,
			dir:     file.dir,
			lastMod: file.lastMod,
			linkTo:  file.linkTo,
		},
		data: nil,
		r:    nil,
	}

	if file.data != nil {
		newFile.data = mem.Get()
		file.data.WriteTo(newFile.data)
	}

	return newFile, true
}

func (m *mapOfStorage) Has(key string) bool {
	token := m.lock.RLock()
	defer m.lock.RUnlock(token)

	_, ok := m.files.Load(key)

	return ok
}

func (m *mapOfStorage) Range(f func(key string, value *memFile) bool) {
	token := m.lock.RLock()
	defer m.lock.RUnlock(token)

	m.files.Range(f)
}

type mapStorage struct {
	lock  sync.RWMutex
	files map[string]*memFile
}

func newMapStorage() memStorage {
	m := &mapStorage{
		files: map[string]*memFile{},
	}

	return m
}

func (m *mapStorage) Delete(key string) (*memFile, bool) {
	m.lock.Lock()
	defer m.lock.Unlock()

	v, ok := m.files[key]
	delete(m.files, key)

	return v, ok
}

func (m *mapStorage) Store(key string, value *memFile) (*memFile, bool) {
	m.lock.Lock()
	defer m.lock.Unlock()

	v, ok := m.files[key]
	m.files[key] = value

	return v, ok
}

func (m *mapStorage) Load(key string) (*memFile, bool) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	v, ok := m.files[key]

	return v, ok
}

func (m *mapStorage) LoadAndCopy(key string) (*memFile, bool) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	v, ok := m.files[key]
	if !ok {
		return nil, false
	}

	newFile := &memFile{
		memFileInfo: memFileInfo{
			name:    v.name,
			size:    v.size,
			dir:     v.dir,
			lastMod: v.lastMod,
			linkTo:  v.linkTo,
		},
		data: nil,
		r:    nil,
	}

	if v.data != nil {
		newFile.data = mem.Get()
		v.data.WriteTo(newFile.data)
	}

	return newFile, true
}

func (m *mapStorage) Has(key string) bool {
	m.lock.RLock()
	defer m.lock.RUnlock()

	_, ok := m.files[key]

	return ok
}

func (m *mapStorage) Range(f func(key string, value *memFile) bool) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	for k, v := range m.files {
		if !f(k, v) {
			break
		}
	}
}

type swissMapStorage struct {
	lock  *xsync.RBMutex
	files *swiss.Map[string, *memFile]
}

func newSwissMapStorage() memStorage {
	m := &swissMapStorage{
		lock:  xsync.NewRBMutex(),
		files: swiss.NewMap[string, *memFile](128),
	}

	return m
}

func (m *swissMapStorage) Delete(key string) (*memFile, bool) {
	m.lock.Lock()
	defer m.lock.Unlock()

	file, hasFile := m.files.Get(key)
	if !hasFile {
		return nil, false
	}

	m.files.Delete(key)

	return file, true
}

func (m *swissMapStorage) Store(key string, value *memFile) (*memFile, bool) {
	m.lock.Lock()
	defer m.lock.Unlock()

	file, hasFile := m.files.Get(key)
	m.files.Put(key, value)

	return file, hasFile
}

func (m *swissMapStorage) Load(key string) (*memFile, bool) {
	token := m.lock.RLock()
	defer m.lock.RUnlock(token)

	return m.files.Get(key)
}

func (m *swissMapStorage) LoadAndCopy(key string) (*memFile, bool) {
	token := m.lock.RLock()
	defer m.lock.RUnlock(token)

	file, ok := m.files.Get(key)
	if !ok {
		return nil, false
	}

	newFile := &memFile{
		memFileInfo: memFileInfo{
			name:    file.name,
			size:    file.size,
			dir:     file.dir,
			lastMod: file.lastMod,
			linkTo:  file.linkTo,
		},
		data: nil,
		r:    nil,
	}

	if file.data != nil {
		newFile.data = mem.Get()
		file.data.WriteTo(newFile.data)
	}

	return newFile, true
}

func (m *swissMapStorage) Has(key string) bool {
	token := m.lock.RLock()
	defer m.lock.RUnlock(token)

	return m.files.Has(key)
}

func (m *swissMapStorage) Range(f func(key string, value *memFile) bool) {
	token := m.lock.RLock()
	defer m.lock.RUnlock(token)

	m.files.Iter(func(key string, value *memFile) bool {
		return !f(key, value)
	})
}
