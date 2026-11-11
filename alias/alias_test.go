package alias

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/datarhei/core/v16/io/fs"

	"github.com/stretchr/testify/require"
)

func newMemFS(t *testing.T) fs.Filesystem {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	return memfs
}

func newManager(t *testing.T, memfs fs.Filesystem) Manager {
	m, err := New(Config{
		Filesystem: memfs,
		Filepath:   "/aliases.json",
	})
	require.NoError(t, err)

	return m
}

func TestNew(t *testing.T) {
	_, err := New(Config{Filepath: "/aliases.json"})
	require.Error(t, err)

	_, err = New(Config{Filesystem: newMemFS(t)})
	require.Error(t, err)

	m := newManager(t, newMemFS(t))
	require.Empty(t, m.List())
}

func TestAddGetList(t *testing.T) {
	m := newManager(t, newMemFS(t))

	require.NoError(t, m.Add(Rule{Pattern: "^/b/(.*)$", Target: "/a/$1"}))
	require.NoError(t, m.Add(Rule{Pattern: "^/c/(.*)$", Target: "/a/$1"}))
	require.NoError(t, m.Add(Rule{Pattern: "^/0/(.*)$", Target: "/a/$1"}))

	require.ErrorIs(t, m.Add(Rule{Pattern: "^/b/(.*)$", Target: "/x/$1"}), ErrExists)

	r, err := m.Get("^/c/(.*)$")
	require.NoError(t, err)
	require.Equal(t, Rule{Pattern: "^/c/(.*)$", Target: "/a/$1"}, r)

	_, err = m.Get("^/d/(.*)$")
	require.ErrorIs(t, err, ErrNotFound)

	require.Equal(t, []Rule{
		{Pattern: "^/b/(.*)$", Target: "/a/$1"},
		{Pattern: "^/c/(.*)$", Target: "/a/$1"},
		{Pattern: "^/0/(.*)$", Target: "/a/$1"},
	}, m.List())
}

func TestInvalid(t *testing.T) {
	m := newManager(t, newMemFS(t))

	require.ErrorIs(t, m.Add(Rule{Pattern: "", Target: "/a"}), ErrInvalid)
	require.ErrorIs(t, m.Add(Rule{Pattern: "^/b/(.*$", Target: "/a/$1"}), ErrInvalid)
	require.ErrorIs(t, m.Add(Rule{Pattern: "^/b/(.*)$", Target: "a/$1"}), ErrInvalid)

	require.NoError(t, m.Add(Rule{Pattern: "^/b/(.*)$", Target: "/a/$1"}))
	require.ErrorIs(t, m.Update("^/b/(.*)$", Rule{Pattern: "^/b/(.*)$", Target: ""}), ErrInvalid)

	require.Len(t, m.List(), 1)
}

func TestUpdate(t *testing.T) {
	m := newManager(t, newMemFS(t))

	require.NoError(t, m.Add(Rule{Pattern: "^/b/(.*)$", Target: "/a/$1"}))
	require.NoError(t, m.Add(Rule{Pattern: "^/c/(.*)$", Target: "/a/$1"}))
	require.NoError(t, m.Add(Rule{Pattern: "^/d/(.*)$", Target: "/a/$1"}))

	// Change the target, the position is kept
	require.NoError(t, m.Update("^/c/(.*)$", Rule{Pattern: "^/c/(.*)$", Target: "/x/$1"}))

	// Change the pattern, the position is kept
	require.NoError(t, m.Update("^/b/(.*)$", Rule{Pattern: "^/e/(.*)$", Target: "/y/$1"}))

	// Not found
	require.ErrorIs(t, m.Update("^/b/(.*)$", Rule{Pattern: "^/b/(.*)$", Target: "/y/$1"}), ErrNotFound)

	// Collision with another rule
	require.ErrorIs(t, m.Update("^/e/(.*)$", Rule{Pattern: "^/d/(.*)$", Target: "/y/$1"}), ErrExists)

	require.Equal(t, []Rule{
		{Pattern: "^/e/(.*)$", Target: "/y/$1"},
		{Pattern: "^/c/(.*)$", Target: "/x/$1"},
		{Pattern: "^/d/(.*)$", Target: "/a/$1"},
	}, m.List())
}

func TestRemove(t *testing.T) {
	m := newManager(t, newMemFS(t))

	require.NoError(t, m.Add(Rule{Pattern: "^/b/(.*)$", Target: "/a/$1"}))
	require.NoError(t, m.Add(Rule{Pattern: "^/c/(.*)$", Target: "/a/$1"}))

	require.NoError(t, m.Remove("^/b/(.*)$"))
	require.ErrorIs(t, m.Remove("^/b/(.*)$"), ErrNotFound)

	require.Equal(t, []Rule{
		{Pattern: "^/c/(.*)$", Target: "/a/$1"},
	}, m.List())

	_, ok := m.Alias("/b/x.txt")
	require.False(t, ok)
}

func TestAlias(t *testing.T) {
	m := newManager(t, newMemFS(t))

	require.NoError(t, m.Add(Rule{Pattern: `^/memfs/b/x\.txt$`, Target: "/memfs/first.txt"}))
	require.NoError(t, m.Add(Rule{Pattern: "^/memfs/b/(.*)$", Target: "/memfs/a/$1"}))
	require.NoError(t, m.Add(Rule{Pattern: "^/memfs/(?P<user>[^/]+)/shared/(.*)$", Target: "/memfs/${user}-public/$2"}))

	target, ok := m.Alias("/memfs/b/x.txt")
	require.True(t, ok)
	require.Equal(t, "/memfs/first.txt", target)

	target, ok = m.Alias("/memfs/b/live/stream.m3u8")
	require.True(t, ok)
	require.Equal(t, "/memfs/a/live/stream.m3u8", target)

	target, ok = m.Alias("/memfs/c/shared/x.txt")
	require.True(t, ok)
	require.Equal(t, "/memfs/c-public/x.txt", target)

	_, ok = m.Alias("/memfs/c/x.txt")
	require.False(t, ok)
}

func TestPersistence(t *testing.T) {
	memfs := newMemFS(t)

	m := newManager(t, memfs)

	require.NoError(t, m.Add(Rule{Pattern: "^/c/(.*)$", Target: "/a/$1"}))
	require.NoError(t, m.Add(Rule{Pattern: "^/b/(.*)$", Target: "/a/$1"}))
	require.NoError(t, m.Add(Rule{Pattern: "^/d/(.*)$", Target: "/a/$1"}))
	require.NoError(t, m.Remove("^/d/(.*)$"))

	m = newManager(t, memfs)

	require.Equal(t, []Rule{
		{Pattern: "^/c/(.*)$", Target: "/a/$1"},
		{Pattern: "^/b/(.*)$", Target: "/a/$1"},
	}, m.List())

	target, ok := m.Alias("/b/x.txt")
	require.True(t, ok)
	require.Equal(t, "/a/x.txt", target)
}

func TestLoadInvalid(t *testing.T) {
	memfs := newMemFS(t)

	_, _, err := memfs.WriteFile("/aliases.json", []byte(`{"version":1,"rules":[`))
	require.NoError(t, err)

	_, err = New(Config{Filesystem: memfs, Filepath: "/aliases.json"})
	require.Error(t, err)

	_, _, err = memfs.WriteFile("/aliases.json", []byte(`{"version":2,"rules":[]}`))
	require.NoError(t, err)

	_, err = New(Config{Filesystem: memfs, Filepath: "/aliases.json"})
	require.Error(t, err)

	// Invalid and duplicate rules are skipped
	_, _, err = memfs.WriteFile("/aliases.json", []byte(`{"version":1,"rules":[
		{"pattern":"^/b/(.*$","target":"/a/$1"},
		{"pattern":"^/c/(.*)$","target":"a/$1"},
		{"pattern":"^/d/(.*)$","target":"/a/$1"},
		{"pattern":"^/d/(.*)$","target":"/x/$1"}
	]}`))
	require.NoError(t, err)

	m := newManager(t, memfs)
	require.Equal(t, []Rule{
		{Pattern: "^/d/(.*)$", Target: "/a/$1"},
	}, m.List())
}

type failingFS struct {
	fs.Filesystem
	fail bool
}

func (f *failingFS) WriteFileSafe(path string, data []byte) (int64, bool, error) {
	if f.fail {
		return -1, false, errors.New("write failed")
	}

	return f.Filesystem.WriteFileSafe(path, data)
}

func TestSaveFailure(t *testing.T) {
	ffs := &failingFS{Filesystem: newMemFS(t)}

	m := newManager(t, ffs)

	require.NoError(t, m.Add(Rule{Pattern: "^/b/(.*)$", Target: "/a/$1"}))

	ffs.fail = true

	// None of the changes are applied if they can't be stored
	require.Error(t, m.Add(Rule{Pattern: "^/c/(.*)$", Target: "/a/$1"}))
	require.Error(t, m.Update("^/b/(.*)$", Rule{Pattern: "^/b/(.*)$", Target: "/x/$1"}))
	require.Error(t, m.Remove("^/b/(.*)$"))

	require.Equal(t, []Rule{
		{Pattern: "^/b/(.*)$", Target: "/a/$1"},
	}, m.List())
}

func TestConcurrency(t *testing.T) {
	m := newManager(t, newMemFS(t))

	wg := sync.WaitGroup{}

	for i := range 4 {
		wg.Go(func() {
			for j := range 50 {
				pattern := fmt.Sprintf("^/%d/%d/(.*)$", i, j)
				require.NoError(t, m.Add(Rule{Pattern: pattern, Target: "/a/$1"}))
				require.NoError(t, m.Update(pattern, Rule{Pattern: pattern, Target: "/b/$1"}))
			}
		})

		wg.Go(func() {
			for range 200 {
				m.Alias("/1/1/x.txt")
				m.List()
			}
		})
	}

	wg.Wait()

	require.Len(t, m.List(), 200)

	target, ok := m.Alias("/1/1/x.txt")
	require.True(t, ok)
	require.Equal(t, "/b/x.txt", target)
}

func TestSet(t *testing.T) {
	memfs := newMemFS(t)
	m := newManager(t, memfs)

	require.NoError(t, m.Add(Rule{Pattern: "^/b/(.*)$", Target: "/a/$1"}))

	require.NoError(t, m.Set([]Rule{
		{Pattern: "^/d/(.*)$", Target: "/x/$1"},
		{Pattern: "^/c/(.*)$", Target: "/y/$1"},
	}))

	expected := []Rule{
		{Pattern: "^/d/(.*)$", Target: "/x/$1"},
		{Pattern: "^/c/(.*)$", Target: "/y/$1"},
	}

	require.Equal(t, expected, m.List())

	_, ok := m.Alias("/b/x.txt")
	require.False(t, ok)

	// Invalid rules or duplicates don't change anything
	require.ErrorIs(t, m.Set([]Rule{
		{Pattern: "^/e/(.*)$", Target: "/x/$1"},
		{Pattern: "^/f/(.*$", Target: "/y/$1"},
	}), ErrInvalid)

	require.ErrorIs(t, m.Set([]Rule{
		{Pattern: "^/e/(.*)$", Target: "/x/$1"},
		{Pattern: "^/e/(.*)$", Target: "/y/$1"},
	}), ErrExists)

	require.Equal(t, expected, m.List())

	// The rules are persisted
	m = newManager(t, memfs)
	require.Equal(t, expected, m.List())

	// Remove all rules
	require.NoError(t, m.Set(nil))
	require.Empty(t, m.List())
}

func TestSetFailure(t *testing.T) {
	ffs := &failingFS{Filesystem: newMemFS(t)}
	m := newManager(t, ffs)

	require.NoError(t, m.Add(Rule{Pattern: "^/b/(.*)$", Target: "/a/$1"}))

	ffs.fail = true

	require.Error(t, m.Set([]Rule{{Pattern: "^/c/(.*)$", Target: "/a/$1"}}))
	require.Equal(t, []Rule{{Pattern: "^/b/(.*)$", Target: "/a/$1"}}, m.List())
}

func TestValidate(t *testing.T) {
	require.NoError(t, Validate(Rule{Pattern: "^/memfs/b/(.+)$", Target: "/memfs/a/$1"}))
	require.ErrorIs(t, Validate(Rule{Pattern: "", Target: "/memfs/a"}), ErrInvalid)
	require.ErrorIs(t, Validate(Rule{Pattern: "^/memfs/b/(.+$", Target: "/memfs/a/$1"}), ErrInvalid)
	require.ErrorIs(t, Validate(Rule{Pattern: "^/memfs/b/(.+)$", Target: "memfs/a/$1"}), ErrInvalid)
}

func TestCompile(t *testing.T) {
	rules := []Rule{
		{Pattern: "^/memfs/b/(.+)$", Target: "/memfs/a/$1"},
		{Pattern: "^/memfs/(.+)$", Target: "/memfs/x/$1"},
	}

	s, err := Compile(rules)
	require.NoError(t, err)
	require.Equal(t, rules, s.Rules())

	target, ok := s.Alias("/memfs/b/x.txt")
	require.True(t, ok)
	require.Equal(t, "/memfs/a/x.txt", target)

	target, ok = s.Alias("/memfs/c/x.txt")
	require.True(t, ok)
	require.Equal(t, "/memfs/x/c/x.txt", target)

	_, ok = s.Alias("/disk/x.txt")
	require.False(t, ok)

	s, err = Compile(nil)
	require.NoError(t, err)
	require.Equal(t, []Rule{}, s.Rules())

	_, err = Compile([]Rule{rules[0], {Pattern: "(", Target: "/x"}})
	require.ErrorIs(t, err, ErrInvalid)

	_, err = Compile([]Rule{rules[0], rules[1], rules[0]})
	require.ErrorIs(t, err, ErrExists)

	var nilset *Ruleset
	_, ok = nilset.Alias("/memfs/b/x.txt")
	require.False(t, ok)
	require.Equal(t, []Rule{}, nilset.Rules())
}
