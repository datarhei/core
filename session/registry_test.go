package session

import (
	"testing"
	"time"

	"github.com/datarhei/core/v16/io/fs"
	"github.com/lestrrat-go/strftime"
	"github.com/stretchr/testify/require"
)

func TestRegister(t *testing.T) {
	r, err := New(Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		r.Close()
	})

	_, err = r.Register("", CollectorConfig{})
	require.Error(t, err)

	_, err = r.Register("../foo/bar", CollectorConfig{})
	require.Error(t, err)

	_, err = r.Register("foobar", CollectorConfig{})
	require.NoError(t, err)

	_, err = r.Register("foobar", CollectorConfig{})
	require.Error(t, err)
}

func TestUnregister(t *testing.T) {
	r, err := New(Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		r.Close()
	})

	_, err = r.Register("foobar", CollectorConfig{})
	require.NoError(t, err)

	err = r.Unregister("foobar")
	require.NoError(t, err)

	err = r.Unregister("foobar")
	require.Error(t, err)
}

func TestCollectors(t *testing.T) {
	r, err := New(Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		r.Close()
	})

	c := r.Collectors()
	require.Equal(t, []string{}, c)

	_, err = r.Register("foobar", CollectorConfig{})
	require.NoError(t, err)

	c = r.Collectors()
	require.Equal(t, []string{"foobar"}, c)

	err = r.Unregister("foobar")
	require.NoError(t, err)

	c = r.Collectors()
	require.Equal(t, []string{}, c)
}

func TestGetCollector(t *testing.T) {
	r, err := New(Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		r.Close()
	})

	c := r.Collector("foobar")
	require.Nil(t, c)

	_, err = r.Register("foobar", CollectorConfig{})
	require.NoError(t, err)

	c = r.Collector("foobar")
	require.NotNil(t, c)

	err = r.Unregister("foobar")
	require.NoError(t, err)

	c = r.Collector("foobar")
	require.Nil(t, c)
}

func TestUnregisterAll(t *testing.T) {
	r, err := New(Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		r.Close()
	})

	_, err = r.Register("foo", CollectorConfig{})
	require.NoError(t, err)

	_, err = r.Register("bar", CollectorConfig{})
	require.NoError(t, err)

	c := r.Collectors()
	require.ElementsMatch(t, []string{"foo", "bar"}, c)

	r.UnregisterAll()

	c = r.Collectors()
	require.Equal(t, []string{}, c)
}

func TestPersistHistory(t *testing.T) {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	r, err := New(Config{
		PersistFS: memfs,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		r.Close()
	})

	c, err := r.Register("foobar", CollectorConfig{})
	require.NoError(t, err)

	c.RegisterAndActivate("foo", "ref", "location", "peer")
	c.Egress("foo", 42)

	err = r.Unregister("foobar")
	require.NoError(t, err)

	_, err = memfs.Stat("/foobar.json")
	require.NoError(t, err)

	c, err = r.Register("foobar", CollectorConfig{})
	require.NoError(t, err)

	cc := c.(*collector)
	totals, ok := cc.history.Sessions["location:peer:ref"]
	require.True(t, ok)
	require.Equal(t, "location", totals.Location)
	require.Equal(t, "ref", totals.Reference)
	require.Equal(t, "peer", totals.Peer)
	require.Equal(t, uint64(42), totals.TotalTxBytes)
}

func TestPeriodicPersistHistory(t *testing.T) {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	r, err := New(Config{
		PersistFS:       memfs,
		PersistInterval: 5 * time.Second,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		r.Close()
	})

	c, err := r.Register("foobar", CollectorConfig{
		SessionTimeout: time.Second,
	})
	require.NoError(t, err)

	c.RegisterAndActivate("foo", "ref", "location", "peer")
	c.Egress("foo", 42)

	require.Eventually(t, func() bool {
		_, err = memfs.Stat("/foobar.json")
		return err == nil
	}, 10*time.Second, time.Second)

	err = r.Unregister("foobar")
	require.NoError(t, err)

	_, err = memfs.Stat("/foobar.json")
	require.NoError(t, err)
}

func TestPersistSession(t *testing.T) {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	pattern := "/log/%Y-%m-%d.log"

	r, err := New(Config{
		PersistFS:  memfs,
		LogPattern: pattern,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		r.Close()
	})

	c, err := r.Register("foobar", CollectorConfig{
		SessionTimeout: 3 * time.Second,
	})
	require.NoError(t, err)

	c.RegisterAndActivate("foo", "ref", "location", "peer")
	c.Egress("foo", 42)

	err = r.Unregister("foobar")
	require.NoError(t, err)

	r.Close()

	path, err := strftime.Format(pattern, time.Now())
	require.NoError(t, err)

	info, err := memfs.Stat(path)
	require.NoError(t, err)
	require.Greater(t, info.Size(), int64(0))
}

func TestPersistSessionSlpit(t *testing.T) {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	pattern := "/log/%Y-%m-%d-%H:%M:%S.log"

	r, err := New(Config{
		PersistFS:  memfs,
		LogPattern: pattern,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		r.Close()
	})

	c, err := r.Register("foobar", CollectorConfig{
		SessionTimeout: 3 * time.Second,
	})
	require.NoError(t, err)

	c.RegisterAndActivate("foo", "ref", "location", "peer")
	c.Egress("foo", 42)

	time.Sleep(3 * time.Second)

	c.RegisterAndActivate("bar", "ref", "location", "peer")
	c.Egress("bar", 24)

	err = r.Unregister("foobar")
	require.NoError(t, err)

	r.Close()

	require.Equal(t, int64(2), memfs.Files())
}

func TestPersistSessionBuffer(t *testing.T) {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	pattern := "/log/%Y-%m-%d.log"

	r, err := New(Config{
		PersistFS:         memfs,
		LogPattern:        pattern,
		LogBufferDuration: 5 * time.Second,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		r.Close()
	})

	c, err := r.Register("foobar", CollectorConfig{
		SessionTimeout: 3 * time.Second,
	})
	require.NoError(t, err)

	c.RegisterAndActivate("foo", "ref", "location", "peer")
	c.Egress("foo", 42)

	require.Eventually(t, func() bool {
		path, err := strftime.Format(pattern, time.Now())
		if err != nil {
			return false
		}
		_, err = memfs.Stat(path)
		return err == nil
	}, 10*time.Second, time.Second)

	err = r.Unregister("foobar")
	require.NoError(t, err)

	r.Close()

	path, err := strftime.Format(pattern, time.Now())
	require.NoError(t, err)

	info, err := memfs.Stat(path)
	require.NoError(t, err)
	require.Greater(t, info.Size(), int64(0))
}
