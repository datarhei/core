package session

import (
	"testing"
	"time"

	"github.com/datarhei/core/v16/io/fs"
	"github.com/stretchr/testify/require"
)

func createCollector(inactive, session time.Duration, sessionsCh chan<- Session) (*collector, error) {
	return newCollector("test", sessionsCh, nil, CollectorConfig{
		InactiveTimeout: inactive,
		SessionTimeout:  session,
	})
}

func TestRegisterSession(t *testing.T) {
	c, err := createCollector(time.Hour, time.Hour, nil)
	require.Equal(t, nil, err)

	b := c.IsKnownSession("foobar")
	require.Equal(t, false, b)

	c.Register("foobar", "", "", "")

	b = c.IsKnownSession("foobar")
	require.Equal(t, true, b)

	c.Unregister("foobar")

	time.Sleep(2 * time.Second)

	b = c.IsKnownSession("foobar")
	require.Equal(t, false, b)
}

func TestInactiveSession(t *testing.T) {
	c, err := createCollector(time.Second, time.Hour, nil)
	require.Equal(t, nil, err)

	b := c.IsKnownSession("foobar")
	require.Equal(t, false, b)

	c.Register("foobar", "", "", "")

	b = c.IsKnownSession("foobar")
	require.Equal(t, true, b)

	time.Sleep(3 * time.Second)

	b = c.IsKnownSession("foobar")
	require.Equal(t, false, b)
}

func TestActivateSession(t *testing.T) {
	c, err := createCollector(time.Second, time.Second, nil)
	require.Equal(t, nil, err)

	b := c.IsKnownSession("foobar")
	require.Equal(t, false, b)

	c.RegisterAndActivate("foobar", "", "", "")

	b = c.IsKnownSession("foobar")
	require.Equal(t, true, b)

	time.Sleep(3 * time.Second)

	b = c.IsKnownSession("foobar")
	require.Equal(t, false, b)
}

func TestIngress(t *testing.T) {
	c, err := createCollector(time.Second, time.Hour, nil)
	require.Equal(t, nil, err)

	c.RegisterAndActivate("foobar", "", "", "")

	c.Ingress("foobar", 1024)

	sessions := c.Active()

	require.Equal(t, 1, len(sessions))
	require.Equal(t, uint64(1024), sessions[0].RxBytes)

	c.stop()
}

func TestEgress(t *testing.T) {
	c, err := createCollector(time.Second, time.Hour, nil)
	require.Equal(t, nil, err)

	c.RegisterAndActivate("foobar", "", "", "")

	c.Egress("foobar", 1024)

	sessions := c.Active()

	require.Equal(t, 1, len(sessions))
	require.Equal(t, uint64(1024), sessions[0].TxBytes)

	c.stop()
}

func TestNbSessions(t *testing.T) {
	c, err := createCollector(time.Hour, time.Hour, nil)
	require.Equal(t, nil, err)

	nsessions := c.Sessions()
	require.Equal(t, uint64(0), nsessions)

	c.Register("foo", "", "", "")

	nsessions = c.Sessions()
	require.Equal(t, uint64(0), nsessions)

	c.Activate("foo")

	nsessions = c.Sessions()
	require.Equal(t, uint64(1), nsessions)

	c.RegisterAndActivate("bar", "", "", "")

	nsessions = c.Sessions()
	require.Equal(t, uint64(2), nsessions)

	c.stop()

	time.Sleep(2 * time.Second)

	nsessions = c.Sessions()
	require.Equal(t, uint64(0), nsessions)
}

func TestHistoryRestore(t *testing.T) {
	sessions := make(chan Session, 1)

	c, err := createCollector(time.Hour, time.Hour, sessions)
	require.Equal(t, nil, err)

	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	_, _, err = memfs.WriteFile("/foobar.json", []byte("{}"))
	require.NoError(t, err)

	snapshot, err := NewHistorySource(memfs, "/foobar.json")
	require.NoError(t, err)

	err = c.Restore(snapshot)
	require.NoError(t, err)

	c.RegisterAndActivate("foo", "", "", "")
	c.Close("foo")

	<-sessions
}

func TestHistoryRestoreInvalid(t *testing.T) {
	sessions := make(chan Session, 1)

	c, err := createCollector(time.Hour, time.Hour, sessions)
	require.Equal(t, nil, err)

	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	_, _, err = memfs.WriteFile("/foobar.json", []byte(""))
	require.NoError(t, err)

	snapshot, err := NewHistorySource(memfs, "/foobar.json")
	require.NoError(t, err)

	err = c.Restore(snapshot)
	require.Error(t, err)

	c.RegisterAndActivate("foo", "", "", "")
	c.Close("foo")

	<-sessions
}
