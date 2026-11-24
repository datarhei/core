package restream

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/datarhei/core/v16/restream/app"
	"github.com/datarhei/core/v16/restream/replace"

	"github.com/stretchr/testify/require"
)

// getAccessRestreamer returns a restreamer whose memfs placeholder contains the current value of password
func getAccessRestreamer(t *testing.T, password *atomic.Value) *restream {
	replacer := replace.New()

	replacer.RegisterReplaceFunc("memfs", func(params map[string]string, config *app.Config, section string) string {
		return "http://" + config.Owner + ":" + password.Load().(string) + "@localhost/memfs"
	}, nil)

	rs, err := getDummyRestreamer(nil, nil, nil, replacer, false)
	require.NoError(t, err)

	return rs.(*restream)
}

// requireCommandContains waits until the command of the running process contains s
func requireCommandContains(t *testing.T, rs *restream, id string, s string) {
	t.Helper()

	require.Eventually(t, func() bool {
		state, err := rs.GetProcessState(app.ProcessID{ID: id})
		if err != nil || state.State != "running" {
			return false
		}

		return strings.Contains(strings.Join(state.Command, " "), s)
	}, 10*time.Second, 50*time.Millisecond, "the command of '%s' doesn't contain '%s'", id, s)
}

func TestRestartResolvesAccessPlaceholders(t *testing.T) {
	password := &atomic.Value{}
	password.Store("pw1")

	rs := getAccessRestreamer(t, password)

	process := getDummyProcess()
	process.Owner = "bob"
	process.Output[0].Options = append(process.Output[0].Options, "-metadata", "url={memfs}/out.txt")

	require.NoError(t, rs.AddProcess(process))

	task, ok := rs.tasks.LoadUnsafe(app.ProcessID{ID: process.ID})
	require.True(t, ok)
	require.Equal(t, "url={memfs}/out.txt", task.config.Output[0].Options[len(task.config.Output[0].Options)-1])

	require.NoError(t, rs.StartProcess(app.ProcessID{ID: process.ID}))
	requireCommandContains(t, rs, process.ID, "url=http://bob:pw1@localhost/memfs/out.txt")

	// A restart uses the new credentials
	password.Store("pw2")

	require.NoError(t, rs.RestartProcess(app.ProcessID{ID: process.ID}))
	requireCommandContains(t, rs, process.ID, "url=http://bob:pw2@localhost/memfs/out.txt")

	require.NoError(t, rs.StopProcess(app.ProcessID{ID: process.ID}))
}

func TestRestartResolvesReferences(t *testing.T) {
	password := &atomic.Value{}
	password.Store("pw1")

	rs := getAccessRestreamer(t, password)

	processA := getDummyProcess()
	processA.ID = "a"
	processA.Owner = "alice"
	processA.Output[0].Address = "{memfs}/a.txt"

	require.NoError(t, rs.AddProcess(processA))

	// Unknown references are rejected when the process is added
	processB := getDummyProcess()
	processB.ID = "b"
	processB.Owner = "bob"
	processB.Input[0].Address = "#x:output=out"

	require.Error(t, rs.AddProcess(processB))

	processB.Input[0].Address = "#a:output=out"

	require.NoError(t, rs.AddProcess(processB))

	task, ok := rs.tasks.LoadUnsafe(app.ProcessID{ID: "b"})
	require.True(t, ok)
	require.Equal(t, "#a:output=out", task.config.Input[0].Address)

	// The reference has the credentials of the owner of the referenced process
	require.NoError(t, rs.StartProcess(app.ProcessID{ID: "b"}))
	requireCommandContains(t, rs, "b", "http://alice:pw1@localhost/memfs/a.txt")

	// A restart uses the new credentials
	password.Store("pw2")

	require.NoError(t, rs.RestartProcess(app.ProcessID{ID: "b"}))
	requireCommandContains(t, rs, "b", "http://alice:pw2@localhost/memfs/a.txt")

	require.NoError(t, rs.StopProcess(app.ProcessID{ID: "b"}))
}
