package process

import (
	"sync"
	"testing"
	"time"

	"github.com/datarhei/core/v16/internal/testhelper"
	"github.com/stretchr/testify/require"
)

func TestProcess(t *testing.T) {
	p, _ := New(Config{
		Binary: "sleep",
		Args: []string{
			"10",
		},
		Reconnect:    false,
		StaleTimeout: 0,
	})

	require.Equal(t, "finished", p.Status().State)

	p.Start()

	require.Equal(t, "running", p.Status().State)

	time.Sleep(5 * time.Second)

	require.Equal(t, "running", p.Status().State)

	p.Stop(false)

	time.Sleep(2 * time.Second)

	require.Equal(t, "killed", p.Status().State)
}

func TestReconnectProcess(t *testing.T) {
	p, _ := New(Config{
		Binary: "sleep",
		Args: []string{
			"2",
		},
		Reconnect:      true,
		ReconnectDelay: 2 * time.Second,
		StaleTimeout:   0,
	})

	p.Start()

	time.Sleep(3 * time.Second)

	require.Equal(t, "finished", p.Status().State)

	p.Stop(false)

	require.Equal(t, "finished", p.Status().State)
}

func TestStaleProcess(t *testing.T) {
	p, _ := New(Config{
		Binary: "sleep",
		Args: []string{
			"10",
		},
		Reconnect:    false,
		StaleTimeout: 2 * time.Second,
	})

	p.Start()

	time.Sleep(5 * time.Second)

	require.Equal(t, "killed", p.Status().State)

	p.Stop(false)

	require.Equal(t, "killed", p.Status().State)
}

func TestStaleReconnectProcess(t *testing.T) {
	p, _ := New(Config{
		Binary: "sleep",
		Args: []string{
			"10",
		},
		Reconnect:    false,
		StaleTimeout: 2 * time.Second,
	})

	p.Start()

	time.Sleep(10 * time.Second)

	require.Equal(t, "killed", p.Status().State)

	p.Stop(false)

	require.Equal(t, "killed", p.Status().State)
}

func TestNonExistingProcess(t *testing.T) {
	p, _ := New(Config{
		Binary: "sloop",
		Args: []string{
			"10",
		},
		Reconnect:      false,
		ReconnectDelay: 5 * time.Second,
		StaleTimeout:   0,
	})

	p.Start()

	time.Sleep(3 * time.Second)

	require.Equal(t, "failed", p.Status().State)

	p.Stop(false)

	require.Equal(t, "failed", p.Status().State)
}

func TestNonExistingReconnectProcess(t *testing.T) {
	p, _ := New(Config{
		Binary: "sloop",
		Args: []string{
			"10",
		},
		Reconnect:      true,
		ReconnectDelay: 2 * time.Second,
		StaleTimeout:   0,
	})

	p.Start()

	time.Sleep(5 * time.Second)

	require.Equal(t, "failed", p.Status().State)

	p.Stop(false)

	require.Equal(t, "failed", p.Status().State)
}

func TestProcessFailed(t *testing.T) {
	p, _ := New(Config{
		Binary: "sleep",
		Args: []string{
			"hello",
		},
		Reconnect:    false,
		StaleTimeout: 0,
	})

	p.Start()

	time.Sleep(5 * time.Second)

	p.Stop(false)

	require.Equal(t, "failed", p.Status().State)
}

func TestFFmpegWaitStop(t *testing.T) {
	binary, err := testhelper.BuildBinary("sigintwait", "../internal/testhelper")
	require.NoError(t, err, "Failed to build helper program")

	p, _ := New(Config{
		Binary:       binary,
		Args:         []string{},
		Reconnect:    false,
		StaleTimeout: 0,
		OnExit: func(state string) {
			time.Sleep(2 * time.Second)
		},
	})

	err = p.Start()
	require.NoError(t, err)

	time.Sleep(4 * time.Second)

	p.Stop(true)

	require.Equal(t, "finished", p.Status().State)
}

func TestFFmpegKill(t *testing.T) {
	binary, err := testhelper.BuildBinary("sigint", "../internal/testhelper")
	require.NoError(t, err, "Failed to build helper program")

	p, _ := New(Config{
		Binary:       binary,
		Args:         []string{},
		Reconnect:    false,
		StaleTimeout: 0,
	})

	err = p.Start()
	require.NoError(t, err)

	time.Sleep(5 * time.Second)

	p.Stop(false)

	time.Sleep(3 * time.Second)

	require.Equal(t, "finished", p.Status().State)
}

func TestProcessForceKill(t *testing.T) {
	binary, err := testhelper.BuildBinary("ignoresigint", "../internal/testhelper")
	require.NoError(t, err, "Failed to build helper program")

	p, _ := New(Config{
		Binary:       binary,
		Args:         []string{},
		Reconnect:    false,
		StaleTimeout: 0,
	})

	err = p.Start()
	require.NoError(t, err)

	time.Sleep(3 * time.Second)

	p.Stop(false)

	time.Sleep(1 * time.Second)

	require.Equal(t, "finishing", p.Status().State)

	time.Sleep(5 * time.Second)

	require.Equal(t, "killed", p.Status().State)
}

func TestProcessDuration(t *testing.T) {
	binary, err := testhelper.BuildBinary("sigint", "../internal/testhelper")
	require.NoError(t, err, "Failed to build helper program")

	p, err := New(Config{
		Binary:  binary,
		Args:    []string{},
		Timeout: 3 * time.Second,
	})
	require.NoError(t, err)

	status := p.Status()
	require.Equal(t, "stop", status.Order)
	require.Equal(t, "finished", status.State)

	err = p.Start()
	require.NoError(t, err)

	time.Sleep(time.Second)

	status = p.Status()
	require.Equal(t, "start", status.Order)
	require.Equal(t, "running", status.State)

	time.Sleep(5 * time.Second)

	status = p.Status()
	require.Equal(t, "start", status.Order)
	require.Equal(t, "finished", status.State)

	px := p.(*process)

	require.Nil(t, px.stopTimer)
}

func TestProcessSchedulePointInTime(t *testing.T) {
	now := time.Now()
	s, err := NewScheduler(now.Add(5 * time.Second).Format(time.RFC3339))
	require.NoError(t, err)

	p, _ := New(Config{
		Binary: "sleep",
		Args: []string{
			"5",
		},
		Reconnect: false,
		Scheduler: s,
	})

	status := p.Status()
	require.Equal(t, "stop", status.Order)
	require.Equal(t, "finished", status.State)

	err = p.Start()
	require.NoError(t, err)

	status = p.Status()
	require.Equal(t, "start", status.Order)
	require.Equal(t, "finished", status.State)
	require.Greater(t, status.Reconnect, time.Duration(0))

	time.Sleep(status.Reconnect + (2 * time.Second))

	status = p.Status()
	require.Equal(t, "running", status.State)

	time.Sleep(5 * time.Second)

	status = p.Status()
	require.Equal(t, "finished", status.State)
	require.Less(t, status.Reconnect, time.Duration(0))
}

func TestProcessSchedulePointInTimeGone(t *testing.T) {
	now := time.Now()
	s, err := NewScheduler(now.Add(-5 * time.Second).Format(time.RFC3339))
	require.NoError(t, err)

	p, _ := New(Config{
		Binary: "sleep",
		Args: []string{
			"5",
		},
		Reconnect: false,
		Scheduler: s,
	})

	status := p.Status()
	require.Equal(t, "stop", status.Order)
	require.Equal(t, "finished", status.State)

	err = p.Start()
	require.Error(t, err)

	status = p.Status()
	require.Equal(t, "start", status.Order)
	require.Equal(t, "finished", status.State)
}

func TestProcessScheduleCron(t *testing.T) {
	s, err := NewScheduler("* * * * *")
	require.NoError(t, err)

	p, _ := New(Config{
		Binary: "sleep",
		Args: []string{
			"5",
		},
		Reconnect: false,
		Scheduler: s,
	})

	status := p.Status()
	require.Equal(t, "stop", status.Order)
	require.Equal(t, "finished", status.State)

	err = p.Start()
	require.NoError(t, err)

	status = p.Status()

	time.Sleep(status.Reconnect + (2 * time.Second))

	status = p.Status()
	require.Equal(t, "running", status.State)

	time.Sleep(5 * time.Second)

	status = p.Status()
	require.Equal(t, "finished", status.State)
	require.Greater(t, status.Reconnect, time.Duration(0))
}

func TestProcessDelayNoScheduler(t *testing.T) {
	p, _ := New(Config{
		Binary:         "sleep",
		Reconnect:      false,
		ReconnectDelay: 5 * time.Second,
	})

	px := p.(*process)

	// negative delay for finished process
	d := px.delay(stateFinished)
	require.Less(t, d, time.Duration(0))

	// negative delay for failed process
	d = px.delay(stateFailed)
	require.Less(t, d, time.Duration(0))

	p, _ = New(Config{
		Binary:         "sleep",
		Reconnect:      true,
		ReconnectDelay: 5 * time.Second,
	})

	px = p.(*process)

	// positive delay for finished process
	d = px.delay(stateFinished)
	require.Greater(t, d, time.Duration(0))

	// positive delay for failed process
	d = px.delay(stateFailed)
	require.Greater(t, d, time.Duration(0))
}

func TestProcessDelaySchedulerNoReconnect(t *testing.T) {
	now := time.Now()
	s, err := NewScheduler(now.Add(5 * time.Second).Format(time.RFC3339))
	require.NoError(t, err)

	p, _ := New(Config{
		Binary:         "sleep",
		Reconnect:      false,
		ReconnectDelay: 1 * time.Second,
		Scheduler:      s,
	})

	px := p.(*process)

	// scheduled delay for finished process
	d := px.delay(stateFinished)
	require.Greater(t, d, time.Second)

	// scheduled delay for failed process
	d = px.delay(stateFailed)
	require.Greater(t, d, time.Second)

	now = time.Now()
	s, err = NewScheduler(now.Add(-5 * time.Second).Format(time.RFC3339))
	require.NoError(t, err)

	p, _ = New(Config{
		Binary:         "sleep",
		Reconnect:      false,
		ReconnectDelay: 1 * time.Second,
		Scheduler:      s,
	})

	px = p.(*process)

	// negative delay for finished process
	d = px.delay(stateFinished)
	require.Less(t, d, time.Duration(0))

	// negative delay for failed process
	d = px.delay(stateFailed)
	require.Less(t, d, time.Duration(0))
}

func TestProcessDelaySchedulerReconnect(t *testing.T) {
	now := time.Now()
	s, err := NewScheduler(now.Add(5 * time.Second).Format(time.RFC3339))
	require.NoError(t, err)

	p, _ := New(Config{
		Binary:         "sleep",
		Reconnect:      true,
		ReconnectDelay: 1 * time.Second,
		Scheduler:      s,
	})

	px := p.(*process)

	// scheduled delay for finished process
	d := px.delay(stateFinished)
	require.Greater(t, d, time.Second)

	// reconnect delay for failed process
	d = px.delay(stateFailed)
	require.Equal(t, d, time.Second)

	now = time.Now()
	s, err = NewScheduler(now.Add(-5 * time.Second).Format(time.RFC3339))
	require.NoError(t, err)

	p, _ = New(Config{
		Binary:         "sleep",
		Reconnect:      true,
		ReconnectDelay: 1 * time.Second,
		Scheduler:      s,
	})

	px = p.(*process)

	// negative delay for finished process
	d = px.delay(stateFinished)
	require.Less(t, d, time.Duration(0))

	// reconnect delay for failed process
	d = px.delay(stateFailed)
	require.Equal(t, d, time.Second)

	now = time.Now()
	s, err = NewScheduler(now.Add(5 * time.Second).Format(time.RFC3339))
	require.NoError(t, err)

	p, _ = New(Config{
		Binary:         "sleep",
		Reconnect:      true,
		ReconnectDelay: 10 * time.Second,
		Scheduler:      s,
	})

	px = p.(*process)

	// scheduled delay for failed process
	d = px.delay(stateFailed)
	require.Less(t, d, 10*time.Second)
}

func TestProcessCallbacks(t *testing.T) {
	var args []string
	onStart := false
	onExit := ""
	onState := []string{}

	lock := sync.Mutex{}

	p, err := New(Config{
		Binary: "sleep",
		Args: []string{
			"2",
		},
		Reconnect: false,
		OnArgs: func(a []string) []string {
			lock.Lock()
			defer lock.Unlock()

			args = make([]string, len(a))
			copy(args, a)
			return a
		},
		OnStart: func() {
			lock.Lock()
			defer lock.Unlock()

			onStart = true
		},
		OnExit: func(state string) {
			lock.Lock()
			defer lock.Unlock()

			onExit = state
		},
		OnStateChange: func(from, to string) {
			lock.Lock()
			defer lock.Unlock()

			onState = append(onState, from+"/"+to)
		},
	})
	require.NoError(t, err)

	err = p.Start()
	require.NoError(t, err)

	time.Sleep(5 * time.Second)

	lock.Lock()
	require.ElementsMatch(t, []string{"2"}, args)
	require.True(t, onStart)
	require.Equal(t, stateFinished.String(), onExit)
	require.ElementsMatch(t, []string{
		"finished/starting",
		"starting/running",
		"running/finished",
	}, onState)
	lock.Unlock()
}
