package restream

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/datarhei/core/v16/ffmpeg"
	"github.com/datarhei/core/v16/iam"
	iamidentity "github.com/datarhei/core/v16/iam/identity"
	"github.com/datarhei/core/v16/iam/policy"
	mock "github.com/datarhei/core/v16/internal/mock/resources"
	"github.com/datarhei/core/v16/internal/testhelper"
	"github.com/datarhei/core/v16/io/fs"
	"github.com/datarhei/core/v16/net"
	"github.com/datarhei/core/v16/resources"
	"github.com/datarhei/core/v16/restream/app"
	rfs "github.com/datarhei/core/v16/restream/fs"
	"github.com/datarhei/core/v16/restream/replace"
	"github.com/datarhei/core/v16/restream/rewrite"
	"github.com/lestrrat-go/strftime"

	"github.com/stretchr/testify/require"
)

func getDummyRestreamer(portrange net.Portranger, validatorIn, validatorOut ffmpeg.Validator, replacer replace.Replacer, limits bool) (Restreamer, error) {
	binary, err := testhelper.BuildBinary("ffmpeg")
	if err != nil {
		return nil, fmt.Errorf("failed to build helper program: %w", err)
	}

	var res resources.Resources
	if limits {
		res = mock.NewWithLimits()
	} else {
		res = mock.New()
	}

	ffmpeg, err := ffmpeg.New(ffmpeg.Config{
		Binary:           binary,
		LogHistoryLength: 3,
		Portrange:        portrange,
		ValidatorInput:   validatorIn,
		ValidatorOutput:  validatorOut,
		Resource:         res,
	})
	if err != nil {
		return nil, err
	}

	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	if err != nil {
		return nil, err
	}

	policyAdapter, err := policy.NewJSONAdapter(memfs, "./policy.json", nil)
	if err != nil {
		return nil, err
	}

	identityAdapter, err := iamidentity.NewJSONAdapter(memfs, "./users.json", nil)
	if err != nil {
		return nil, err
	}

	iam, err := iam.New(iam.Config{
		PolicyAdapter:   policyAdapter,
		IdentityAdapter: identityAdapter,
		Superuser: iamidentity.User{
			Name: "foobar",
		},
		JWTRealm:  "",
		JWTSecret: "",
		Logger:    nil,
	})
	if err != nil {
		return nil, err
	}

	iam.AddPolicy("$anon", "$none", []string{"process"}, "*", []string{"CREATE", "GET", "DELETE", "UPDATE", "COMMAND", "PROBE", "METADATA", "PLAYOUT"})

	rewriter, err := rewrite.New(rewrite.Config{
		IAM: iam,
	})
	if err != nil {
		return nil, err
	}

	rs, err := New(Config{
		FFmpeg:      ffmpeg,
		Replace:     replacer,
		Filesystems: []fs.Filesystem{memfs},
		Rewrite:     rewriter,
		Resources:   res,
	})
	if err != nil {
		return nil, err
	}

	return rs, nil
}

func getDummyProcess() *app.Config {
	return &app.Config{
		ID: "process",
		Input: []app.ConfigIO{
			{
				ID:      "in",
				Address: "testsrc=size=1280x720:rate=25",
				Options: []string{
					"-f",
					"lavfi",
					"-re",
				},
			},
		},
		Output: []app.ConfigIO{
			{
				ID:      "out",
				Address: "-",
				Options: []string{
					"-codec",
					"copy",
					"-f",
					"null",
				},
			},
		},
		Options: []string{
			"-loglevel",
			"info",
		},
		Reconnect:      true,
		ReconnectDelay: 10,
		Autostart:      false,
		StaleTimeout:   0,
	}
}

func TestAddProcess(t *testing.T) {
	rs, err := getDummyRestreamer(nil, nil, nil, nil, false)
	require.NoError(t, err)

	process := getDummyProcess()
	require.NotNil(t, process)

	tid := app.ProcessID{ID: process.ID}

	_, err = rs.GetProcess(tid)
	require.Equal(t, ErrUnknownProcess, err)

	err = rs.AddProcess(process)
	require.Equal(t, nil, err, "Failed to add process (%s)", err)

	_, err = rs.GetProcess(tid)
	require.Equal(t, nil, err, "Set process not found (%s)", process.ID)

	state, _ := rs.GetProcessState(tid)
	require.Equal(t, "stop", state.Order, "Process should be stopped")
}

func TestAutostartProcess(t *testing.T) {
	rs, err := getDummyRestreamer(nil, nil, nil, nil, false)
	require.NoError(t, err)

	process := getDummyProcess()
	process.Autostart = true

	tid := app.ProcessID{ID: process.ID}

	rs.AddProcess(process)

	state, _ := rs.GetProcessState(tid)
	require.Equal(t, "start", state.Order, "Process should be started")

	rs.StopProcess(tid)
}

func TestAddInvalidProcess(t *testing.T) {
	rs, err := getDummyRestreamer(nil, nil, nil, nil, false)
	require.NoError(t, err)

	// Invalid process ID
	process := getDummyProcess()
	process.ID = ""

	err = rs.AddProcess(process)
	require.NotEqual(t, nil, err, "Succeeded to add process without ID")

	// Invalid input ID
	process = getDummyProcess()
	process.Input[0].ID = ""

	err = rs.AddProcess(process)
	require.NotEqual(t, nil, err, "Succeeded to add process input without ID")

	// Invalid input address
	process = getDummyProcess()
	process.Input[0].Address = ""

	err = rs.AddProcess(process)
	require.NotEqual(t, nil, err, "Succeeded to add process input without address")

	// Duplicate input ID
	process = getDummyProcess()
	process.Input = append(process.Input, process.Input[0])

	err = rs.AddProcess(process)
	require.NotEqual(t, nil, err, "Succeeded to add process input with duplicate ID")

	// No inputs
	process = getDummyProcess()
	process.Input = nil

	err = rs.AddProcess(process)
	require.NotEqual(t, nil, err, "Succeeded to add process without inputs")

	// Invalid output ID
	process = getDummyProcess()
	process.Output[0].ID = ""

	err = rs.AddProcess(process)
	require.NotEqual(t, nil, err, "Succeeded to add process output without ID")

	// Invalid output address
	process = getDummyProcess()
	process.Output[0].Address = ""

	err = rs.AddProcess(process)
	require.NotEqual(t, nil, err, "Succeeded to add process output without address")

	// Duplicate output ID
	process = getDummyProcess()
	process.Output = append(process.Output, process.Output[0])

	err = rs.AddProcess(process)
	require.NotEqual(t, nil, err, "Succeeded to add process output with duplicate ID")

	// No outputs
	process = getDummyProcess()
	process.Output = nil

	err = rs.AddProcess(process)
	require.NotEqual(t, nil, err, "Succeeded to add process without outputs")
}

func TestRemoveProcess(t *testing.T) {
	rs, err := getDummyRestreamer(nil, nil, nil, nil, false)
	require.NoError(t, err)

	process := getDummyProcess()
	tid := app.ProcessID{ID: process.ID}

	err = rs.AddProcess(process)
	require.Equal(t, nil, err, "Failed to add process (%s)", err)

	err = rs.DeleteProcess(tid, true)
	require.Equal(t, nil, err, "Set process not found (%s)", process.ID)

	_, err = rs.GetProcess(tid)
	require.NotEqual(t, nil, err, "Unset process found (%s)", process.ID)
}

func TestUpdateProcess(t *testing.T) {
	rs, err := getDummyRestreamer(nil, nil, nil, nil, false)
	require.NoError(t, err)

	process1 := getDummyProcess()
	require.NotNil(t, process1)
	process1.ID = "process1"
	tid1 := app.ProcessID{ID: process1.ID}

	process2 := getDummyProcess()
	require.NotNil(t, process2)
	process2.ID = "process2"
	tid2 := app.ProcessID{ID: process2.ID}

	err = rs.AddProcess(process1)
	require.Equal(t, nil, err)

	err = rs.AddProcess(process2)
	require.Equal(t, nil, err)

	process, err := rs.GetProcess(tid2)
	require.NoError(t, err)

	createdAt := process.CreatedAt
	updatedAt := process.UpdatedAt

	time.Sleep(2 * time.Second)

	process3 := getDummyProcess()
	require.NotNil(t, process3)
	process3.ID = "process2"
	tid3 := app.ProcessID{ID: process3.ID}

	err = rs.UpdateProcess(tid1, process3, false)
	require.Error(t, err)

	process3.ID = "process3"
	tid3.ID = process3.ID
	err = rs.UpdateProcess(tid1, process3, false)
	require.NoError(t, err)

	_, err = rs.GetProcess(tid1)
	require.Error(t, err)

	process, err = rs.GetProcess(tid3)
	require.NoError(t, err)

	require.NotEqual(t, createdAt, process.CreatedAt) // this should be equal, but will require a major version jump
	require.NotEqual(t, updatedAt, process.UpdatedAt)
}

func TestUpdateSameHashProcess(t *testing.T) {
	rs, err := getDummyRestreamer(nil, nil, nil, nil, false)
	require.NoError(t, err)

	config := getDummyProcess()
	require.NotNil(t, config)
	tid := app.ProcessID{ID: config.ID}

	err = rs.AddProcess(config)
	require.Equal(t, nil, err)

	process, err := rs.GetProcess(tid)
	require.NoError(t, err)

	createdAt := process.CreatedAt
	updatedAt := process.UpdatedAt

	time.Sleep(2 * time.Second)

	err = rs.UpdateProcess(tid, config, false)
	require.NoError(t, err)

	process, err = rs.GetProcess(tid)
	require.NoError(t, err)

	require.Equal(t, createdAt, process.CreatedAt)
	require.Equal(t, updatedAt, process.UpdatedAt)
}

func TestUpdateProcessLogHistoryTransfer(t *testing.T) {
	rs, err := getDummyRestreamer(nil, nil, nil, nil, false)
	require.NoError(t, err)

	p := getDummyProcess()
	require.NotNil(t, p)
	p.ID = "process1"

	tid1 := app.ProcessID{ID: p.ID}

	err = rs.AddProcess(p)
	require.Equal(t, nil, err)

	rs.StartProcess(tid1)

	require.Eventually(t, func() bool {
		state, _ := rs.GetProcessState(tid1)
		return state.State == "running"
	}, 10*time.Second, time.Second)

	log, err := rs.GetProcessReport(tid1)
	require.NoError(t, err)

	require.Equal(t, 0, len(log.History))

	p = getDummyProcess()
	require.NotNil(t, p)

	p.ID = "process2"
	err = rs.UpdateProcess(tid1, p, false)
	require.NoError(t, err)

	tid2 := app.ProcessID{ID: p.ID}

	_, err = rs.GetProcess(tid2)
	require.NoError(t, err)

	state, _ := rs.GetProcessState(tid2)
	require.Equal(t, "start", state.Order)

	require.Eventually(t, func() bool {
		state, _ := rs.GetProcessState(tid2)
		return state.State == "running"
	}, 10*time.Second, time.Second)

	log, err = rs.GetProcessReport(tid2)
	require.NoError(t, err)

	require.Equal(t, 1, len(log.History))

	err = rs.StopProcess(tid2)
	require.NoError(t, err)
}

func TestUpdateProcessMetadataTransfer(t *testing.T) {
	rs, err := getDummyRestreamer(nil, nil, nil, nil, false)
	require.NoError(t, err)

	p := getDummyProcess()
	require.NotNil(t, p)
	p.ID = "process1"

	tid1 := app.ProcessID{ID: p.ID}

	err = rs.AddProcess(p)
	require.Equal(t, nil, err)

	err = rs.SetProcessMetadata(tid1, "foo", "bar")
	require.Equal(t, nil, err)

	p = getDummyProcess()
	require.NotNil(t, p)

	p.ID = "process2"
	err = rs.UpdateProcess(tid1, p, false)
	require.NoError(t, err)

	tid2 := app.ProcessID{ID: p.ID}

	_, err = rs.GetProcess(tid2)
	require.NoError(t, err)

	metadata, err := rs.GetProcessMetadata(tid2, "")
	require.NoError(t, err)
	require.Equal(t, map[string]interface{}{
		"foo": "bar",
	}, metadata)
}

func TestGetProcess(t *testing.T) {
	rs, err := getDummyRestreamer(nil, nil, nil, nil, false)
	require.NoError(t, err)

	process1 := getDummyProcess()
	process1.ID = "foo_aaa_1"
	process1.Reference = "foo_aaa_1"
	tid1 := app.ProcessID{ID: process1.ID}
	process2 := getDummyProcess()
	process2.ID = "bar_bbb_2"
	process2.Reference = "bar_bbb_2"
	tid2 := app.ProcessID{ID: process2.ID}
	process3 := getDummyProcess()
	process3.ID = "foo_ccc_3"
	process3.Reference = "foo_ccc_3"
	tid3 := app.ProcessID{ID: process3.ID}
	process4 := getDummyProcess()
	process4.ID = "bar_ddd_4"
	process4.Reference = "bar_ddd_4"
	tid4 := app.ProcessID{ID: process4.ID}

	rs.AddProcess(process1)
	rs.AddProcess(process2)
	rs.AddProcess(process3)
	rs.AddProcess(process4)

	_, err = rs.GetProcess(tid1)
	require.Equal(t, nil, err)

	_, err = rs.GetProcess(tid2)
	require.Equal(t, nil, err)

	_, err = rs.GetProcess(tid3)
	require.Equal(t, nil, err)

	_, err = rs.GetProcess(tid4)
	require.Equal(t, nil, err)

	list := rs.GetProcessIDs("", "", "", "")
	require.Len(t, list, 4)
	require.ElementsMatch(t, []app.ProcessID{{ID: "foo_aaa_1"}, {ID: "bar_bbb_2"}, {ID: "foo_ccc_3"}, {ID: "bar_ddd_4"}}, list)

	list = rs.GetProcessIDs("foo_*", "", "", "")
	require.Len(t, list, 2)
	require.ElementsMatch(t, []app.ProcessID{{ID: "foo_aaa_1"}, {ID: "foo_ccc_3"}}, list)

	list = rs.GetProcessIDs("bar_*", "", "", "")
	require.Len(t, list, 2)
	require.ElementsMatch(t, []app.ProcessID{{ID: "bar_bbb_2"}, {ID: "bar_ddd_4"}}, list)

	list = rs.GetProcessIDs("*_bbb_*", "", "", "")
	require.Len(t, list, 1)
	require.ElementsMatch(t, []app.ProcessID{{ID: "bar_bbb_2"}}, list)

	list = rs.GetProcessIDs("", "foo_*", "", "")
	require.Len(t, list, 2)
	require.ElementsMatch(t, []app.ProcessID{{ID: "foo_aaa_1"}, {ID: "foo_ccc_3"}}, list)

	list = rs.GetProcessIDs("", "bar_*", "", "")
	require.Len(t, list, 2)
	require.ElementsMatch(t, []app.ProcessID{{ID: "bar_bbb_2"}, {ID: "bar_ddd_4"}}, list)

	list = rs.GetProcessIDs("", "*_bbb_*", "", "")
	require.Len(t, list, 1)
	require.ElementsMatch(t, []app.ProcessID{{ID: "bar_bbb_2"}}, list)
}

func TestStartProcess(t *testing.T) {
	rs, err := getDummyRestreamer(nil, nil, nil, nil, false)
	require.NoError(t, err)

	process := getDummyProcess()
	tid := app.ProcessID{ID: process.ID}

	rs.AddProcess(process)

	err = rs.StartProcess(app.ProcessID{ID: "foobar"})
	require.NotEqual(t, nil, err, "shouldn't be able to start non-existing process")

	err = rs.StartProcess(tid)
	require.Equal(t, nil, err, "should be able to start existing process")

	state, _ := rs.GetProcessState(tid)
	require.Equal(t, "start", state.Order, "Process should be started")

	err = rs.StartProcess(tid)
	require.Equal(t, nil, err, "should be able to start already running process")

	state, _ = rs.GetProcessState(tid)
	require.Equal(t, "start", state.Order, "Process should be started")

	rs.StopProcess(tid)
}

func TestStartProcessWithLimits(t *testing.T) {
	rs, err := getDummyRestreamer(nil, nil, nil, nil, true)
	require.NoError(t, err)

	process := getDummyProcess()
	process.LimitCPU = 1
	process.LimitMemory = 1
	process.LimitGPU = app.ConfigLimitGPU{
		Usage:   1,
		Encoder: 1,
		Decoder: 1,
		Memory:  1,
	}
	process.Options = append(process.Options, "-hwdevice", "{hwdevice}")
	tid := app.ProcessID{ID: process.ID}

	rs.AddProcess(process)

	err = rs.StartProcess(tid)
	require.Equal(t, nil, err, "should be able to start existing process")

	state, _ := rs.GetProcessState(tid)
	require.Equal(t, "start", state.Order, "Process should be started")

	require.Equal(t, []string{
		"-loglevel", "info", "-hwdevice", "0", "-f", "lavfi", "-re", "-i", "testsrc=size=1280x720:rate=25", "-codec", "copy", "-f", "null", "-",
	}, state.Command)

	rs.StopProcess(tid)
}

func TestProcessResources(t *testing.T) {
	rs, err := getDummyRestreamer(nil, nil, nil, nil, false)
	require.NoError(t, err)

	process := getDummyProcess()
	tid := app.ProcessID{ID: process.ID}

	rs.AddProcess(process)

	err = rs.StartProcess(tid)
	require.Equal(t, nil, err, "should be able to start existing process")

	time.Sleep(2 * time.Second)

	state, _ := rs.GetProcessState(tid)
	require.Equal(t, "start", state.Order, "Process should be started")

	require.Equal(t, app.ProcessUsage{
		CPU: app.ProcessUsageCPU{
			NCPU:         2,
			Current:      12,
			Average:      12,
			Max:          12,
			Limit:        0,
			IsThrottling: false,
		},
		Memory: app.ProcessUsageMemory{
			Current: 42,
			Average: 42,
			Max:     42,
			Limit:   0,
		},
		GPU: app.ProcessUsageGPU{
			Index: 0,
			Usage: app.ProcessUsageGPUUsage{
				Current: 5,
				Average: 5,
				Max:     5,
				Limit:   0,
			},
			Encoder: app.ProcessUsageGPUUsage{
				Current: 9,
				Average: 9,
				Max:     9,
				Limit:   0,
			},
			Decoder: app.ProcessUsageGPUUsage{
				Current: 11,
				Average: 11,
				Max:     11,
				Limit:   0,
			},
			Memory: app.ProcessUsageGPUMemory{
				Current: 42,
				Average: 42,
				Max:     42,
				Limit:   0,
			},
		},
	}, state.Resources)
}

func TestStopProcess(t *testing.T) {
	rs, err := getDummyRestreamer(nil, nil, nil, nil, false)
	require.NoError(t, err)

	process := getDummyProcess()
	tid := app.ProcessID{ID: process.ID}

	rs.AddProcess(process)
	rs.StartProcess(tid)

	err = rs.StopProcess(app.ProcessID{ID: "foobar"})
	require.NotEqual(t, nil, err, "shouldn't be able to stop non-existing process")

	err = rs.StopProcess(tid)
	require.Equal(t, nil, err, "should be able to stop existing running process")

	state, _ := rs.GetProcessState(tid)
	require.Equal(t, "stop", state.Order, "Process should be stopped")

	err = rs.StopProcess(tid)
	require.Equal(t, nil, err, "should be able to stop already stopped process")

	state, _ = rs.GetProcessState(tid)
	require.Equal(t, "stop", state.Order, "Process should be stopped")
}

func TestRestartProcess(t *testing.T) {
	rs, err := getDummyRestreamer(nil, nil, nil, nil, false)
	require.NoError(t, err)

	process := getDummyProcess()
	tid := app.ProcessID{ID: process.ID}

	rs.AddProcess(process)

	err = rs.RestartProcess(app.ProcessID{ID: "foobar"})
	require.NotEqual(t, nil, err, "shouldn't be able to restart non-existing process")

	err = rs.RestartProcess(tid)
	require.Equal(t, nil, err, "should be able to restart existing stopped process")

	state, _ := rs.GetProcessState(tid)
	require.Equal(t, "stop", state.Order, "Process should be stopped")

	rs.StartProcess(tid)

	state, _ = rs.GetProcessState(tid)
	require.Equal(t, "start", state.Order, "Process should be started")

	rs.StopProcess(tid)
}

func TestReloadProcess(t *testing.T) {
	rs, err := getDummyRestreamer(nil, nil, nil, nil, false)
	require.NoError(t, err)

	process := getDummyProcess()
	tid := app.ProcessID{ID: process.ID}

	rs.AddProcess(process)

	err = rs.ReloadProcess(app.ProcessID{ID: "foobar"})
	require.NotEqual(t, nil, err, "shouldn't be able to reload non-existing process")

	err = rs.ReloadProcess(tid)
	require.Equal(t, nil, err, "should be able to reload existing stopped process")

	state, _ := rs.GetProcessState(tid)
	require.Equal(t, "stop", state.Order, "Process should be stopped")

	rs.StartProcess(tid)

	state, _ = rs.GetProcessState(tid)
	require.Equal(t, "start", state.Order, "Process should be started")

	err = rs.ReloadProcess(tid)
	require.Equal(t, nil, err, "should be able to reload existing process")

	state, _ = rs.GetProcessState(tid)
	require.Equal(t, "start", state.Order, "Process should be started")

	rs.StopProcess(tid)
}

func TestParseProcessPattern(t *testing.T) {
	rs, err := getDummyRestreamer(nil, nil, nil, nil, false)
	require.NoError(t, err)

	process := getDummyProcess()
	process.LogPatterns = []string{"libx264"}

	tid := app.ProcessID{ID: process.ID}

	rs.AddProcess(process)
	rs.StartProcess(tid)

	time.Sleep(3 * time.Second)

	rs.StopProcess(tid)

	log, err := rs.GetProcessReport(tid)
	require.NoError(t, err)

	require.Equal(t, 1, len(log.History))
	require.Equal(t, 8, len(log.History[0].Matches))
}

func TestProbeProcess(t *testing.T) {
	rs, err := getDummyRestreamer(nil, nil, nil, nil, false)
	require.NoError(t, err)

	process := getDummyProcess()
	probe := rs.Probe(process, 5*time.Second)

	require.Equal(t, 3, len(probe.Streams))
}

func TestProbeProcessWithReference(t *testing.T) {
	rs, err := getDummyRestreamer(nil, nil, nil, nil, false)
	require.NoError(t, err)

	process := getDummyProcess()

	rs.AddProcess(process)

	process = getDummyProcess()
	process.ID = ""
	process.Input[0].Address = "#process:output=out"
	probe := rs.Probe(process, 5*time.Second)

	require.Equal(t, 3, len(probe.Streams))
}

func TestProcessMetadata(t *testing.T) {
	rs, err := getDummyRestreamer(nil, nil, nil, nil, false)
	require.NoError(t, err)

	process := getDummyProcess()
	tid := app.ProcessID{ID: process.ID}

	rs.AddProcess(process)

	data, err := rs.GetProcessMetadata(tid, "foobar")
	require.Equal(t, ErrMetadataKeyNotFound, err)
	require.Equal(t, nil, data, "nothing should be stored under the key")

	err = rs.SetProcessMetadata(tid, "foobar", process)
	require.NoError(t, err)

	data, err = rs.GetProcessMetadata(tid, "foobar")
	require.NoError(t, err)
	require.NotEqual(t, nil, data, "there should be something stored under the key")

	p := data.(*app.Config)

	require.Equal(t, process.ID, p.ID, "failed to retrieve stored data")
}

func TestLog(t *testing.T) {
	rs, err := getDummyRestreamer(nil, nil, nil, nil, false)
	require.NoError(t, err)

	process := getDummyProcess()
	tid := app.ProcessID{ID: process.ID}

	rs.AddProcess(process)

	_, err = rs.GetProcessReport(app.ProcessID{ID: "foobar"})
	require.Error(t, err)

	log, err := rs.GetProcessReport(tid)
	require.NoError(t, err)
	require.Equal(t, 0, len(log.Prelude))
	require.Equal(t, 0, len(log.Log))
	require.Equal(t, 0, len(log.Matches))
	require.Equal(t, 0, len(log.History))

	rs.StartProcess(tid)

	time.Sleep(3 * time.Second)

	log, _ = rs.GetProcessReport(tid)

	require.NotEqual(t, 0, len(log.Prelude))
	require.NotEqual(t, 0, len(log.Log))
	require.Equal(t, 0, len(log.Matches))
	require.Equal(t, 0, len(log.History))

	rs.StopProcess(tid)

	log, _ = rs.GetProcessReport(tid)

	require.Equal(t, 0, len(log.Prelude))
	require.Equal(t, 0, len(log.Log))
	require.Equal(t, 1, len(log.History))
}

func TestLogTransfer(t *testing.T) {
	rs, err := getDummyRestreamer(nil, nil, nil, nil, false)
	require.NoError(t, err)

	process := getDummyProcess()
	tid := app.ProcessID{ID: process.ID}

	err = rs.AddProcess(process)
	require.NoError(t, err)

	rs.StartProcess(tid)
	time.Sleep(3 * time.Second)
	rs.StopProcess(tid)

	log, _ := rs.GetProcessReport(tid)

	require.Equal(t, 1, len(log.History))

	err = rs.UpdateProcess(tid, process, false)
	require.NoError(t, err)

	log, _ = rs.GetProcessReport(tid)

	require.Equal(t, 1, len(log.History))
}

func TestPlayoutNoRange(t *testing.T) {
	rs, err := getDummyRestreamer(nil, nil, nil, nil, false)
	require.NoError(t, err)

	process := getDummyProcess()
	tid := app.ProcessID{ID: process.ID}

	process.Input[0].Address = "playout:" + process.Input[0].Address

	rs.AddProcess(process)

	_, err = rs.GetPlayout(app.ProcessID{ID: "foobar"}, process.Input[0].ID)
	require.Equal(t, ErrUnknownProcess, err)

	_, err = rs.GetPlayout(tid, "foobar")
	require.NotEqual(t, nil, err, "playout of non-existing input should error")

	addr, _ := rs.GetPlayout(tid, process.Input[0].ID)
	require.Equal(t, 0, len(addr), "the playout address should be empty if no port range is given")
}

func TestPlayoutRange(t *testing.T) {
	portrange, err := net.NewPortrange(3000, 3001)
	require.NoError(t, err)

	rs, err := getDummyRestreamer(portrange, nil, nil, nil, false)
	require.NoError(t, err)

	process := getDummyProcess()
	tid := app.ProcessID{ID: process.ID}

	process.Input[0].Address = "playout:" + process.Input[0].Address

	rs.AddProcess(process)

	_, err = rs.GetPlayout(app.ProcessID{ID: "foobar"}, process.Input[0].ID)
	require.Equal(t, ErrUnknownProcess, err)

	_, err = rs.GetPlayout(tid, "foobar")
	require.NotEqual(t, nil, err, "playout of non-existing input should error")

	addr, err := rs.GetPlayout(tid, process.Input[0].ID)
	require.NoError(t, err)
	require.NotEqual(t, 0, len(addr), "the playout address should not be empty if a port range is given")
	require.Equal(t, "127.0.0.1:3000", addr, "the playout address should be 127.0.0.1:3000")
}

func TestParseAddressReference(t *testing.T) {
	matches, err := parseAddressReference("foobar")
	require.NoError(t, err)
	require.Equal(t, "foobar", matches["address"])

	_, err = parseAddressReference("#foobar")
	require.Error(t, err)

	_, err = parseAddressReference("#foobar:nothing=foo")
	require.Error(t, err)

	matches, err = parseAddressReference("#foobar:output=foo")
	require.NoError(t, err)
	require.Equal(t, "foobar", matches["id"])
	require.Equal(t, "foo", matches["output"])

	matches, err = parseAddressReference("#foobar:domain=foo")
	require.NoError(t, err)
	require.Equal(t, "foobar", matches["id"])
	require.Equal(t, "foo", matches["domain"])

	matches, err = parseAddressReference("#foobar:nothing=foo:output=bar")
	require.NoError(t, err)
	require.Equal(t, "foobar:nothing=foo", matches["id"])
	require.Equal(t, "bar", matches["output"])

	matches, err = parseAddressReference("#foobar:output=foo:domain=bar")
	require.NoError(t, err)
	require.Equal(t, "foobar", matches["id"])
	require.Equal(t, "foo", matches["output"])
	require.Equal(t, "bar", matches["domain"])
}

func TestAddressReference(t *testing.T) {
	rs, err := getDummyRestreamer(nil, nil, nil, nil, false)
	require.NoError(t, err)

	process1 := getDummyProcess()
	process2 := getDummyProcess()

	rs.AddProcess(process1)

	process2.ID = "process2"
	process2.Input[0].Address = "#process:foobar=out"

	err = rs.AddProcess(process2)
	require.NotEqual(t, nil, err, "shouldn't resolve invalid reference")

	process2.Input[0].Address = "#process2:output=out"

	err = rs.AddProcess(process2)
	require.NotEqual(t, nil, err, "shouldn't resolve invalid reference")

	process2.Input[0].Address = "#process:output=foobar"

	err = rs.AddProcess(process2)
	require.NotEqual(t, nil, err, "shouldn't resolve invalid reference")

	process2.Input[0].Address = "#process:output=out"

	err = rs.AddProcess(process2)
	require.Equal(t, nil, err, "should resolve reference")
}

func TestTeeAddressReference(t *testing.T) {
	rs, err := getDummyRestreamer(nil, nil, nil, nil, false)
	require.NoError(t, err)

	process1 := getDummyProcess()
	process2 := getDummyProcess()
	process3 := getDummyProcess()
	process4 := getDummyProcess()

	process1.Output[0].Address = "[f=hls]http://example.com/live.m3u8|[f=flv]rtmp://example.com/live.stream?token=123"
	process2.ID = "process2"
	process3.ID = "process3"
	process4.ID = "process4"

	rs.AddProcess(process1)

	process2.Input[0].Address = "#process:output=out"

	err = rs.AddProcess(process2)
	require.Equal(t, nil, err, "should resolve reference")

	process3.Input[0].Address = "#process:output=out:source=hls"

	err = rs.AddProcess(process3)
	require.Equal(t, nil, err, "should resolve reference")

	process4.Input[0].Address = "#process:output=out:source=rtmp"

	err = rs.AddProcess(process4)
	require.Equal(t, nil, err, "should resolve reference")

	r := rs.(*restream)

	task, ok := r.tasks.LoadUnsafe(app.ProcessID{ID: "process2"})
	require.True(t, ok)
	require.Equal(t, "http://example.com/live.m3u8", task.config.Input[0].Address)

	task, ok = r.tasks.LoadUnsafe(app.ProcessID{ID: "process3"})
	require.True(t, ok)
	require.Equal(t, "http://example.com/live.m3u8", task.config.Input[0].Address)

	task, ok = r.tasks.LoadUnsafe(app.ProcessID{ID: "process4"})
	require.True(t, ok)
	require.Equal(t, "rtmp://example.com/live.stream?token=123", task.config.Input[0].Address)
}

func TestConfigValidation(t *testing.T) {
	rsi, err := getDummyRestreamer(nil, nil, nil, nil, false)
	require.NoError(t, err)

	rs := rsi.(*restream)

	config := getDummyProcess()

	hasfiles, err := validateConfig(config, rs.fs.list, rs.ffmpeg)
	require.NoError(t, err)
	require.False(t, hasfiles)

	config.Input = []app.ConfigIO{}
	hasfiles, err = validateConfig(config, rs.fs.list, rs.ffmpeg)
	require.Error(t, err)
	require.False(t, hasfiles)

	config = getDummyProcess()
	config.Input[0].ID = ""
	hasfiles, err = validateConfig(config, rs.fs.list, rs.ffmpeg)
	require.Error(t, err)
	require.False(t, hasfiles)

	config = getDummyProcess()
	config.Input[0].Address = ""
	hasfiles, err = validateConfig(config, rs.fs.list, rs.ffmpeg)
	require.Error(t, err)
	require.False(t, hasfiles)

	config = getDummyProcess()
	config.Output = []app.ConfigIO{}
	hasfiles, err = validateConfig(config, rs.fs.list, rs.ffmpeg)
	require.Error(t, err)
	require.False(t, hasfiles)

	config = getDummyProcess()
	config.Output[0].ID = ""
	hasfiles, err = validateConfig(config, rs.fs.list, rs.ffmpeg)
	require.Error(t, err)
	require.False(t, hasfiles)

	config = getDummyProcess()
	config.Output[0].Address = ""
	hasfiles, err = validateConfig(config, rs.fs.list, rs.ffmpeg)
	require.Error(t, err)
	require.False(t, hasfiles)
}

func TestConfigValidationWithMkdir(t *testing.T) {
	rsi, err := getDummyRestreamer(nil, nil, nil, nil, false)
	require.NoError(t, err)

	rs := rsi.(*restream)

	config := getDummyProcess()
	config.Output[0].Address = "/path/to/a/file/image.jpg"
	hasfiles, err := validateConfig(config, rs.fs.list, rs.ffmpeg)
	require.NoError(t, err)
	require.False(t, hasfiles)

	info, err := rs.fs.list[0].Stat("/path/to/a/file")
	require.NoError(t, err)
	require.True(t, info.IsDir())

	diskfs, err := fs.NewRootedDiskFilesystem(fs.RootedDiskConfig{
		Root: "./testing",
	})
	require.NoError(t, err)

	diskrfs, err := rfs.New(rfs.Config{
		FS:       diskfs,
		Interval: time.Second,
	})
	require.NoError(t, err)

	hasfiles, err = validateConfig(config, []rfs.Filesystem{diskrfs}, rs.ffmpeg)
	require.NoError(t, err)
	require.True(t, hasfiles)

	info, err = diskfs.Stat("/path/to/a/file")
	require.NoError(t, err)
	require.True(t, info.IsDir())

	os.RemoveAll("./testing")
}

func TestConfigValidationFFmpeg(t *testing.T) {
	valIn, err := ffmpeg.NewValidator([]string{"^https?://"}, nil)
	require.NoError(t, err)

	valOut, err := ffmpeg.NewValidator([]string{"^https?://", "^rtmp://"}, nil)
	require.NoError(t, err)

	rsi, err := getDummyRestreamer(nil, valIn, valOut, nil, false)
	require.NoError(t, err)

	rs := rsi.(*restream)

	config := getDummyProcess()

	_, err = validateConfig(config, rs.fs.list, rs.ffmpeg)
	require.Error(t, err)

	config.Input[0].Address = "http://stream.example.com/master.m3u8"
	config.Output[0].Address = "http://stream.example.com/master2.m3u8"

	_, err = validateConfig(config, rs.fs.list, rs.ffmpeg)
	require.NoError(t, err)

	config.Output[0].Address = "[f=flv]http://stream.example.com/master2.m3u8"
	_, err = validateConfig(config, rs.fs.list, rs.ffmpeg)
	require.NoError(t, err)

	config.Output[0].Address = "[f=hls]http://stream.example.com/master2.m3u8|[f=flv]rtmp://stream.example.com/stream"
	_, err = validateConfig(config, rs.fs.list, rs.ffmpeg)
	require.NoError(t, err)
}

func TestOutputAddressValidation(t *testing.T) {
	rsi, err := getDummyRestreamer(nil, nil, nil, nil, false)
	require.NoError(t, err)

	rs := rsi.(*restream)

	type res struct {
		path string
		err  bool
	}

	paths := map[string]res{
		"/dev/null":                            {"file:/dev/null", false},
		"/dev/../etc/passwd":                   {"/etc/passwd", true},
		"/dev/fb0":                             {"file:/dev/fb0", false},
		"/etc/passwd":                          {"/etc/passwd", true},
		"/core/data/../../etc/passwd":          {"/etc/passwd", true},
		"/core/data/./etc/passwd":              {"file:/core/data/etc/passwd", false},
		"file:/core/data/foobar":               {"file:/core/data/foobar", false},
		"http://example.com":                   {"http://example.com", false},
		"-":                                    {"pipe:", false},
		"/core/data/foobar|http://example.com": {"file:/core/data/foobar|http://example.com", false},
		"/core/data/foobar|/etc/passwd":        {"/core/data/foobar|/etc/passwd", true},
		"[f=mpegts]udp://10.0.1.255:1234/":     {"[f=mpegts]udp://10.0.1.255:1234/", false},
		"[f=null]-|[f=null]-":                  {"[f=null]pipe:|[f=null]pipe:", false},
		"[onfail=ignore]/core/data/archive-20121107.mkv|[f=mpegts]udp://10.0.1.255:1234/": {"[onfail=ignore]file:/core/data/archive-20121107.mkv|[f=mpegts]udp://10.0.1.255:1234/", false},
	}

	for path, r := range paths {
		path, _, err := validateOutputAddress(path, "/core/data", rs.ffmpeg)

		if r.err {
			require.Error(t, err, path)
		} else {
			require.NoError(t, err)
		}

		require.Equal(t, r.path, path)
	}
}

func TestMetadata(t *testing.T) {
	rs, err := getDummyRestreamer(nil, nil, nil, nil, false)
	require.NoError(t, err)

	process := getDummyProcess()

	data, _ := rs.GetMetadata("foobar")
	require.Equal(t, nil, data, "nothing should be stored under the key")

	rs.SetMetadata("foobar", process)

	data, _ = rs.GetMetadata("foobar")
	require.NotEqual(t, nil, data, "there should be something stored under the key")

	p := data.(*app.Config)

	require.Equal(t, process.ID, p.ID, "failed to retrieve stored data")
}

func TestReplacer(t *testing.T) {
	replacer := replace.New()

	replacer.RegisterReplaceFunc("date", func(params map[string]string, config *app.Config, section string) string {
		t, err := time.Parse(time.RFC3339, params["timestamp"])
		if err != nil {
			return ""
		}

		s, err := strftime.Format(params["format"], t)
		if err != nil {
			return ""
		}

		return s
	}, map[string]string{
		"format":    "%Y-%m-%d_%H-%M-%S",
		"timestamp": "2019-10-12T07:20:50.52Z",
	})

	replacer.RegisterReplaceFunc("diskfs", func(params map[string]string, config *app.Config, section string) string {
		return "/mnt/diskfs"
	}, nil)

	replacer.RegisterReplaceFunc("fs:disk", func(params map[string]string, config *app.Config, section string) string {
		return "/mnt/diskfs"
	}, nil)

	replacer.RegisterReplaceFunc("memfs", func(params map[string]string, config *app.Config, section string) string {
		return "http://localhost/mnt/memfs"
	}, nil)

	replacer.RegisterReplaceFunc("fs:mem", func(params map[string]string, config *app.Config, section string) string {
		return "http://localhost/mnt/memfs"
	}, nil)

	replacer.RegisterReplaceFunc("rtmp", func(params map[string]string, config *app.Config, section string) string {
		return "rtmp://localhost/app/" + params["name"] + "?token=foobar"
	}, nil)

	replacer.RegisterReplaceFunc("srt", func(params map[string]string, config *app.Config, section string) string {
		template := "srt://localhost:6000?mode=caller&transtype=live&latency=" + params["latency"] + "&streamid=" + params["name"]
		if section == "output" {
			template += ",mode:publish"
		} else {
			template += ",mode:request"
		}
		template += ",token:abcfoobar&passphrase=secret"

		return template
	}, map[string]string{
		"name":    "",
		"latency": "20000", // 20 milliseconds, FFmpeg requires microseconds
	})

	process := &app.Config{
		ID:        "314159265359",
		Reference: "refref",
		FFVersion: "^4.0.2",
		Input: []app.ConfigIO{
			{
				ID:      "in_{processid}_{reference}",
				Address: "input:{inputid}_process:{processid}_reference:{reference}_diskfs:{diskfs}/disk.txt_memfs:{memfs}/mem.txt_fsdisk:{fs:disk}/fsdisk.txt_fsmem:{fs:mem}/fsmem.txt_rtmp:{rtmp,name=pmtr}_srt:{srt,name=trs}_rtmp:{rtmp,name=$inputid}_hwdevice:{hwdevice}",
				Options: []string{
					"-f",
					"lavfi",
					"-re",
					"input:{inputid}",
					"process:{processid}",
					"reference:{reference}",
					"diskfs:{diskfs}/disk.txt",
					"memfs:{memfs}/mem.txt",
					"fsdisk:{fs:disk}/fsdisk_{date,format=%Y%m%d_%H%M%S}.txt",
					"fsmem:{fs:mem}/$inputid.txt",
					"hwdevice:{hwdevice}",
				},
			},
		},
		Output: []app.ConfigIO{
			{
				ID:      "out_{processid}_{reference}",
				Address: "output:{outputid}_process:{processid}_reference:{reference}_diskfs:{diskfs}/disk.txt_memfs:{memfs}/mem.txt_fsdisk:{fs:disk}/fsdisk.txt_fsmem:{fs:mem}/fsmem.txt_rtmp:{rtmp,name=$processid}_srt:{srt,name=$reference,latency=42}_rtmp:{rtmp,name=$outputid}",
				Options: []string{
					"-codec",
					"copy",
					"-f",
					"null",
					"output:{outputid}",
					"process:{processid}",
					"reference:{reference}",
					"diskfs:{diskfs}/disk.txt",
					"memfs:{memfs}/mem.txt",
					"fsdisk:{fs:disk}/fsdisk.txt",
					"fsmem:{fs:mem}/$outputid.txt",
				},
				Cleanup: []app.ConfigIOCleanup{
					{
						Pattern:       "pattern_{outputid}_{processid}_{reference}_{rtmp,name=$outputid}",
						MaxFiles:      0,
						MaxFileAge:    0,
						PurgeOnDelete: false,
					},
				},
			},
		},
		Options: []string{
			"-loglevel",
			"info",
			"{diskfs}/foobar_on_disk.txt",
			"{memfs}/foobar_in_mem.txt",
			"{fs:disk}/foobar_on_disk_aswell.txt",
			"{fs:mem}/foobar_in_mem_aswell.txt",
			"hwdevice:{hwdevice}",
		},
		Reconnect:      true,
		ReconnectDelay: 10,
		Autostart:      false,
		StaleTimeout:   0,
	}

	resolveStaticPlaceholders(process, replacer)

	wantprocess := &app.Config{
		ID:        "314159265359",
		Reference: "refref",
		FFVersion: "^4.0.2",
		Input: []app.ConfigIO{
			{
				ID:      "in_314159265359_refref",
				Address: "input:in_314159265359_refref_process:314159265359_reference:refref_diskfs:/mnt/diskfs/disk.txt_memfs:http://localhost/mnt/memfs/mem.txt_fsdisk:/mnt/diskfs/fsdisk.txt_fsmem:http://localhost/mnt/memfs/fsmem.txt_rtmp:rtmp://localhost/app/pmtr?token=foobar_srt:srt://localhost:6000?mode=caller&transtype=live&latency=20000&streamid=trs,mode:request,token:abcfoobar&passphrase=secret_rtmp:rtmp://localhost/app/in_314159265359_refref?token=foobar_hwdevice:{hwdevice}",
				Options: []string{
					"-f",
					"lavfi",
					"-re",
					"input:in_314159265359_refref",
					"process:314159265359",
					"reference:refref",
					"diskfs:/mnt/diskfs/disk.txt",
					"memfs:http://localhost/mnt/memfs/mem.txt",
					"fsdisk:/mnt/diskfs/fsdisk_{date,format=%Y%m%d_%H%M%S}.txt",
					"fsmem:http://localhost/mnt/memfs/$inputid.txt",
					"hwdevice:{hwdevice}",
				},
			},
		},
		Output: []app.ConfigIO{
			{
				ID:      "out_314159265359_refref",
				Address: "output:out_314159265359_refref_process:314159265359_reference:refref_diskfs:/mnt/diskfs/disk.txt_memfs:http://localhost/mnt/memfs/mem.txt_fsdisk:/mnt/diskfs/fsdisk.txt_fsmem:http://localhost/mnt/memfs/fsmem.txt_rtmp:rtmp://localhost/app/314159265359?token=foobar_srt:srt://localhost:6000?mode=caller&transtype=live&latency=42&streamid=refref,mode:publish,token:abcfoobar&passphrase=secret_rtmp:rtmp://localhost/app/out_314159265359_refref?token=foobar",
				Options: []string{
					"-codec",
					"copy",
					"-f",
					"null",
					"output:out_314159265359_refref",
					"process:314159265359",
					"reference:refref",
					"diskfs:/mnt/diskfs/disk.txt",
					"memfs:http://localhost/mnt/memfs/mem.txt",
					"fsdisk:/mnt/diskfs/fsdisk.txt",
					"fsmem:http://localhost/mnt/memfs/$outputid.txt",
				},
				Cleanup: []app.ConfigIOCleanup{
					{
						Pattern:       "pattern_out_314159265359_refref_314159265359_refref_{rtmp,name=$outputid}",
						MaxFiles:      0,
						MaxFileAge:    0,
						PurgeOnDelete: false,
					},
				},
			},
		},
		Options: []string{
			"-loglevel",
			"info",
			"/mnt/diskfs/foobar_on_disk.txt",
			"{memfs}/foobar_in_mem.txt",
			"/mnt/diskfs/foobar_on_disk_aswell.txt",
			"http://localhost/mnt/memfs/foobar_in_mem_aswell.txt",
			"hwdevice:{hwdevice}",
		},
		Reconnect:      true,
		ReconnectDelay: 10,
		Autostart:      false,
		StaleTimeout:   0,
	}

	require.Equal(t, wantprocess, process)

	resolveDynamicPlaceholder(process, replacer, map[string]string{
		"hwdevice": fmt.Sprintf("%d", -1),
	}, nil)

	wantprocess.Options = []string{
		"-loglevel",
		"info",
		"/mnt/diskfs/foobar_on_disk.txt",
		"{memfs}/foobar_in_mem.txt",
		"/mnt/diskfs/foobar_on_disk_aswell.txt",
		"http://localhost/mnt/memfs/foobar_in_mem_aswell.txt",
		"hwdevice:-1",
	}
	wantprocess.Input = []app.ConfigIO{
		{
			ID:      "in_314159265359_refref",
			Address: "input:in_314159265359_refref_process:314159265359_reference:refref_diskfs:/mnt/diskfs/disk.txt_memfs:http://localhost/mnt/memfs/mem.txt_fsdisk:/mnt/diskfs/fsdisk.txt_fsmem:http://localhost/mnt/memfs/fsmem.txt_rtmp:rtmp://localhost/app/pmtr?token=foobar_srt:srt://localhost:6000?mode=caller&transtype=live&latency=20000&streamid=trs,mode:request,token:abcfoobar&passphrase=secret_rtmp:rtmp://localhost/app/in_314159265359_refref?token=foobar_hwdevice:-1",
			Options: []string{
				"-f",
				"lavfi",
				"-re",
				"input:in_314159265359_refref",
				"process:314159265359",
				"reference:refref",
				"diskfs:/mnt/diskfs/disk.txt",
				"memfs:http://localhost/mnt/memfs/mem.txt",
				"fsdisk:/mnt/diskfs/fsdisk_20191012_072050.txt",
				"fsmem:http://localhost/mnt/memfs/$inputid.txt",
				"hwdevice:-1",
			},
		},
	}

	require.Equal(t, wantprocess, process)
}

func TestProcessReplacer(t *testing.T) {
	replacer := replace.New()

	replacer.RegisterReplaceFunc("date", func(params map[string]string, config *app.Config, section string) string {
		t, err := time.Parse(time.RFC3339, params["timestamp"])
		if err != nil {
			return ""
		}

		s, err := strftime.Format(params["format"], t)
		if err != nil {
			return ""
		}

		return s
	}, map[string]string{
		"format":    "%Y-%m-%d_%H-%M-%S",
		"timestamp": "2019-10-12T07:20:50.52Z",
	})

	replacer.RegisterReplaceFunc("diskfs", func(params map[string]string, config *app.Config, section string) string {
		return "/mnt/diskfs"
	}, nil)

	replacer.RegisterReplaceFunc("fs:disk", func(params map[string]string, config *app.Config, section string) string {
		return "/mnt/diskfs"
	}, nil)

	replacer.RegisterReplaceFunc("memfs", func(params map[string]string, config *app.Config, section string) string {
		return "http://localhost/mnt/memfs"
	}, nil)

	replacer.RegisterReplaceFunc("fs:mem", func(params map[string]string, config *app.Config, section string) string {
		return "http://localhost/mnt/memfs"
	}, nil)

	replacer.RegisterReplaceFunc("rtmp", func(params map[string]string, config *app.Config, section string) string {
		return "rtmp://localhost/app/" + params["name"] + "?token=foobar"
	}, nil)

	replacer.RegisterReplaceFunc("srt", func(params map[string]string, config *app.Config, section string) string {
		template := "srt://localhost:6000?mode=caller&transtype=live&latency=" + params["latency"] + "&streamid=" + params["name"]
		if section == "output" {
			template += ",mode:publish"
		} else {
			template += ",mode:request"
		}
		template += ",token:abcfoobar&passphrase=secret"

		return template
	}, map[string]string{
		"name":    "",
		"latency": "20000", // 20 milliseconds, FFmpeg requires microseconds
	})

	rsi, err := getDummyRestreamer(nil, nil, nil, replacer, false)
	require.NoError(t, err)

	process := &app.Config{
		ID:        "314159265359",
		Reference: "refref",
		Input: []app.ConfigIO{
			{
				ID:      "in_{processid}_{reference}",
				Address: "input:{inputid}_process:{processid}_reference:{reference}_diskfs:{diskfs}/disk.txt_memfs:{memfs}/mem.txt_fsdisk:{fs:disk}/fsdisk.txt_fsmem:{fs:mem}/fsmem.txt_rtmp:{rtmp,name=pmtr}_srt:{srt,name=trs}_rtmp:{rtmp,name=$inputid}",
				Options: []string{
					"-f",
					"lavfi",
					"-re",
					"input:{inputid}",
					"process:{processid}",
					"reference:{reference}",
					"diskfs:{diskfs}/disk.txt",
					"memfs:{memfs}/mem.txt",
					"fsdisk:{fs:disk}/fsdisk_{date,format=%Y%m%d_%H%M%S}.txt",
					"fsmem:{fs:mem}/$inputid.txt",
					"hwdevice:{hwdevice}",
				},
			},
		},
		Output: []app.ConfigIO{
			{
				ID:      "out_{processid}_{reference}",
				Address: "output:{outputid}_process:{processid}_reference:{reference}_diskfs:{diskfs}/disk.txt_memfs:{memfs}/mem.txt_fsdisk:{fs:disk}/fsdisk.txt_fsmem:{fs:mem}/fsmem.txt_rtmp:{rtmp,name=$processid}_srt:{srt,name=$reference,latency=42}_rtmp:{rtmp,name=$outputid}",
				Options: []string{
					"-codec",
					"copy",
					"-f",
					"null",
					"output:{outputid}",
					"process:{processid}",
					"reference:{reference}",
					"diskfs:{diskfs}/disk.txt",
					"memfs:{memfs}/mem.txt",
					"fsdisk:{fs:disk}/fsdisk.txt",
					"fsmem:{fs:mem}/$outputid.txt",
				},
				Cleanup: []app.ConfigIOCleanup{
					{
						Pattern:       "pattern_{outputid}_{processid}_{reference}_{rtmp,name=$outputid}",
						MaxFiles:      0,
						MaxFileAge:    0,
						PurgeOnDelete: false,
					},
				},
			},
		},
		Options: []string{
			"-loglevel",
			"info",
			"{diskfs}/foobar_on_disk.txt",
			"{memfs}/foobar_in_mem.txt",
			"{fs:disk}/foobar_on_disk_aswell.txt",
			"{fs:mem}/foobar_in_mem_aswell.txt",
			"hwdevice:{hwdevice}",
		},
		Reconnect:      true,
		ReconnectDelay: 10,
		Autostart:      false,
		StaleTimeout:   0,
	}

	err = rsi.AddProcess(process)
	require.NoError(t, err)

	rs := rsi.(*restream)

	process = &app.Config{
		ID:        "314159265359",
		Reference: "refref",
		FFVersion: "^7.1.1",
		Input: []app.ConfigIO{
			{
				ID:      "in_314159265359_refref",
				Address: "input:in_314159265359_refref_process:314159265359_reference:refref_diskfs:/mnt/diskfs/disk.txt_memfs:http://localhost/mnt/memfs/mem.txt_fsdisk:/mnt/diskfs/fsdisk.txt_fsmem:http://localhost/mnt/memfs/fsmem.txt_rtmp:rtmp://localhost/app/pmtr?token=foobar_srt:srt://localhost:6000?mode=caller&transtype=live&latency=20000&streamid=trs,mode:request,token:abcfoobar&passphrase=secret_rtmp:rtmp://localhost/app/in_314159265359_refref?token=foobar",
				Options: []string{
					"-f",
					"lavfi",
					"-re",
					"input:in_314159265359_refref",
					"process:314159265359",
					"reference:refref",
					"diskfs:/mnt/diskfs/disk.txt",
					"memfs:http://localhost/mnt/memfs/mem.txt",
					"fsdisk:/mnt/diskfs/fsdisk_{date,format=%Y%m%d_%H%M%S}.txt",
					"fsmem:http://localhost/mnt/memfs/$inputid.txt",
					"hwdevice:{hwdevice}",
				},
				Cleanup: []app.ConfigIOCleanup{},
			},
		},
		Output: []app.ConfigIO{
			{
				ID:      "out_314159265359_refref",
				Address: "output:out_314159265359_refref_process:314159265359_reference:refref_diskfs:/mnt/diskfs/disk.txt_memfs:http://localhost/mnt/memfs/mem.txt_fsdisk:/mnt/diskfs/fsdisk.txt_fsmem:http://localhost/mnt/memfs/fsmem.txt_rtmp:rtmp://localhost/app/314159265359?token=foobar_srt:srt://localhost:6000?mode=caller&transtype=live&latency=42&streamid=refref,mode:publish,token:abcfoobar&passphrase=secret_rtmp:rtmp://localhost/app/out_314159265359_refref?token=foobar",
				Options: []string{
					"-codec",
					"copy",
					"-f",
					"null",
					"output:out_314159265359_refref",
					"process:314159265359",
					"reference:refref",
					"diskfs:/mnt/diskfs/disk.txt",
					"memfs:http://localhost/mnt/memfs/mem.txt",
					"fsdisk:/mnt/diskfs/fsdisk.txt",
					"fsmem:http://localhost/mnt/memfs/$outputid.txt",
				},
				Cleanup: []app.ConfigIOCleanup{
					{
						Pattern:       "pattern_out_314159265359_refref_314159265359_refref_{rtmp,name=$outputid}",
						MaxFiles:      0,
						MaxFileAge:    0,
						PurgeOnDelete: false,
					},
				},
			},
		},
		Options: []string{
			"-loglevel",
			"info",
			"/mnt/diskfs/foobar_on_disk.txt",
			"{memfs}/foobar_in_mem.txt",
			"/mnt/diskfs/foobar_on_disk_aswell.txt",
			"http://localhost/mnt/memfs/foobar_in_mem_aswell.txt",
			"hwdevice:{hwdevice}",
		},
		Reconnect:      true,
		ReconnectDelay: 10,
		Autostart:      false,
		StaleTimeout:   0,
		LogPatterns:    []string{},
	}

	task, ok := rs.tasks.LoadUnsafe(app.ProcessID{ID: "314159265359"})
	require.True(t, ok)

	require.Equal(t, process, task.config)

	err = rsi.StartProcess(app.ProcessID{ID: "314159265359"})
	require.NoError(t, err)

	state, err := rsi.GetProcessState(app.ProcessID{ID: "314159265359"})
	require.NoError(t, err)

	require.Equal(t, []string{
		"-loglevel",
		"info",
		"/mnt/diskfs/foobar_on_disk.txt",
		"{memfs}/foobar_in_mem.txt",
		"/mnt/diskfs/foobar_on_disk_aswell.txt",
		"http://localhost/mnt/memfs/foobar_in_mem_aswell.txt",
		"hwdevice:0",
		"-f",
		"lavfi",
		"-re",
		"input:in_314159265359_refref",
		"process:314159265359",
		"reference:refref",
		"diskfs:/mnt/diskfs/disk.txt",
		"memfs:http://localhost/mnt/memfs/mem.txt",
		"fsdisk:/mnt/diskfs/fsdisk_20191012_072050.txt",
		"fsmem:http://localhost/mnt/memfs/$inputid.txt",
		"hwdevice:0",
		"-i",
		"input:in_314159265359_refref_process:314159265359_reference:refref_diskfs:/mnt/diskfs/disk.txt_memfs:http://localhost/mnt/memfs/mem.txt_fsdisk:/mnt/diskfs/fsdisk.txt_fsmem:http://localhost/mnt/memfs/fsmem.txt_rtmp:rtmp://localhost/app/pmtr?token=foobar_srt:srt://localhost:6000?mode=caller&transtype=live&latency=20000&streamid=trs,mode:request,token:abcfoobar&passphrase=secret_rtmp:rtmp://localhost/app/in_314159265359_refref?token=foobar",
		"-codec",
		"copy",
		"-f",
		"null",
		"output:out_314159265359_refref",
		"process:314159265359",
		"reference:refref",
		"diskfs:/mnt/diskfs/disk.txt",
		"memfs:http://localhost/mnt/memfs/mem.txt",
		"fsdisk:/mnt/diskfs/fsdisk.txt",
		"fsmem:http://localhost/mnt/memfs/$outputid.txt",
		"output:out_314159265359_refref_process:314159265359_reference:refref_diskfs:/mnt/diskfs/disk.txt_memfs:http://localhost/mnt/memfs/mem.txt_fsdisk:/mnt/diskfs/fsdisk.txt_fsmem:http://localhost/mnt/memfs/fsmem.txt_rtmp:rtmp://localhost/app/314159265359?token=foobar_srt:srt://localhost:6000?mode=caller&transtype=live&latency=42&streamid=refref,mode:publish,token:abcfoobar&passphrase=secret_rtmp:rtmp://localhost/app/out_314159265359_refref?token=foobar",
	}, state.Command)

	rsi.StopProcess(app.ProcessID{ID: "314159265359"})
}

func TestProcessLogPattern(t *testing.T) {
	rs, err := getDummyRestreamer(nil, nil, nil, nil, false)
	require.NoError(t, err)

	process := getDummyProcess()
	process.LogPatterns = []string{
		"using cpu capabilities:",
	}
	process.Autostart = false
	process.Reconnect = true

	tid := app.ProcessID{ID: process.ID}

	err = rs.AddProcess(process)
	require.NoError(t, err)

	err = rs.StartProcess(tid)
	require.NoError(t, err)

	time.Sleep(5 * time.Second)

	log, err := rs.GetProcessReport(tid)
	require.NoError(t, err)

	require.Equal(t, 1, len(log.Matches))
	require.Equal(t, "[libx264 @ 0x7fa96a800600] using cpu capabilities: MMX2 SSE2Fast SSSE3 SSE4.2 AVX FMA3 BMI2 AVX2", log.Matches[0])

	err = rs.StopProcess(tid)
	require.NoError(t, err)
}

func TestProcessLimit(t *testing.T) {
	rsi, err := getDummyRestreamer(nil, nil, nil, nil, false)
	require.NoError(t, err)

	process := getDummyProcess()
	process.LimitCPU = 61
	process.LimitMemory = 42
	process.Autostart = false

	err = rsi.AddProcess(process)
	require.NoError(t, err)

	rs := rsi.(*restream)

	task, ok := rs.tasks.LoadUnsafe(app.ProcessID{ID: process.ID})
	require.True(t, ok)

	status := task.ffmpeg.Status()

	ncpu := rs.resources.Info().CPU.NCPU

	require.Equal(t, ncpu*process.LimitCPU, status.CPU.Limit)
	require.Equal(t, process.LimitMemory, status.Memory.Limit)
}

func BenchmarkGetProcessIDs(b *testing.B) {
	rs, err := getDummyRestreamer(nil, nil, nil, nil, false)
	require.NoError(b, err)

	for i := 0; i < 1000; i++ {
		process := getDummyProcess()
		process.ID = "test_" + strconv.Itoa(i)

		err = rs.AddProcess(process)
		require.Equal(b, nil, err, "Failed to add process (%s)", err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ids := rs.GetProcessIDs("", "", "", "")
		require.NotEmpty(b, ids)
		require.Equal(b, 1000, len(ids))
	}
}

func BenchmarkGetProcess(b *testing.B) {
	rs, err := getDummyRestreamer(nil, nil, nil, nil, false)
	require.NoError(b, err)

	for i := 0; i < 1000; i++ {
		process := getDummyProcess()
		process.ID = "test_" + strconv.Itoa(i)

		err = rs.AddProcess(process)
		require.Equal(b, nil, err, "Failed to add process (%s)", err)
	}

	rand := rand.New(rand.NewSource(42))

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		n := rand.Intn(1000)
		p, err := rs.GetProcess(app.NewProcessID("test_"+strconv.Itoa(n), ""))
		require.NotNil(b, p)
		require.Nil(b, err)
	}
}

func BenchmarkGetProcessState(b *testing.B) {
	rs, err := getDummyRestreamer(nil, nil, nil, nil, false)
	require.NoError(b, err)

	n := 10

	for i := 0; i < n; i++ {
		process := getDummyProcess()
		process.ID = "test_" + strconv.Itoa(i)
		process.Autostart = true

		err = rs.AddProcess(process)
		require.Equal(b, nil, err, "Failed to add process (%s)", err)
	}

	rand := rand.New(rand.NewSource(42))

	time.Sleep(10 * time.Second)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		n := rand.Intn(n)
		s, err := rs.GetProcessState(app.NewProcessID("test_"+strconv.Itoa(n), ""))
		require.NotNil(b, s)
		require.Nil(b, err)
	}

	for i := 0; i < n; i++ {
		rs.DeleteProcess(app.NewProcessID("test_"+strconv.Itoa(n), ""), true)
	}
}

func TestProcessCleanup(t *testing.T) {
	rsi, err := getDummyRestreamer(nil, nil, nil, nil, false)
	require.NoError(t, err)

	rsi.Start()

	rs := rsi.(*restream)

	memfs, ok := rs.fs.list[0].(fs.Filesystem)
	require.True(t, ok)

	for i := 0; i < 10; i++ {
		memfs.WriteFileReader(fmt.Sprintf("/foobar_%02d.dat", i), bytes.NewReader([]byte("hello")), -1)
	}

	files := memfs.List("/", fs.ListOptions{
		Pattern: "/foobar_*",
	})
	require.Equal(t, 10, len(files))

	process := getDummyProcess()
	process.ID = "foobar"
	output := process.Output[0]
	output.Cleanup = append(output.Cleanup, app.ConfigIOCleanup{
		Pattern:       "mem:/{processid}_*",
		MaxFiles:      5,
		MaxFileAge:    0,
		PurgeOnDelete: true,
	})
	process.Output[0] = output

	err = rsi.AddProcess(process)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		files := memfs.List("/", fs.ListOptions{
			Pattern: "/foobar_*",
		})

		return len(files) == 5
	}, 15*time.Second, time.Second)

	rsi.Stop()

	for i := 0; i < 10; i++ {
		memfs.WriteFileReader(fmt.Sprintf("/foobar_%02d.dat", i), bytes.NewReader([]byte("hello")), -1)
	}

	files = memfs.List("/", fs.ListOptions{
		Pattern: "/foobar_*",
	})
	require.Equal(t, 10, len(files))

	rsi.ReloadProcess(app.ProcessID{ID: process.ID})

	rsi.Start()

	require.Eventually(t, func() bool {
		files := memfs.List("/", fs.ListOptions{
			Pattern: "/foobar_*",
		})

		return len(files) == 5
	}, 15*time.Second, time.Second)

	rsi.Stop()

	for i := range 10 {
		memfs.WriteFileReader(fmt.Sprintf("/foobar_%02d.dat", i), bytes.NewReader([]byte("hello")), -1)
	}

	files = memfs.List("/", fs.ListOptions{
		Pattern: "/foobar_*",
	})
	require.Equal(t, 10, len(files))

	process.Reference = "foobar"
	rsi.UpdateProcess(app.ProcessID{ID: process.ID}, process, false)

	rsi.Start()

	require.Eventually(t, func() bool {
		files := memfs.List("/", fs.ListOptions{
			Pattern: "/foobar_*",
		})

		return len(files) == 5
	}, 15*time.Second, time.Second)

	rsi.Stop()

	//task, ok := rs.tasks.Load(app.ProcessID{ID: process.ID})
}
