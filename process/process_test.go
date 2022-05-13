package process

import (
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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

	assert.Equal(t, "finished", p.Status().State)

	p.Start()

	assert.Equal(t, "running", p.Status().State)

	time.Sleep(5 * time.Second)

	assert.Equal(t, "running", p.Status().State)

	p.Stop()

	time.Sleep(2 * time.Second)

	assert.Equal(t, "killed", p.Status().State)
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

	assert.Equal(t, "finished", p.Status().State)

	p.Stop()

	assert.Equal(t, "finished", p.Status().State)
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

	assert.Equal(t, "killed", p.Status().State)

	p.Stop()

	assert.Equal(t, "killed", p.Status().State)
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

	assert.Equal(t, "killed", p.Status().State)

	p.Stop()

	assert.Equal(t, "killed", p.Status().State)
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

	assert.Equal(t, "failed", p.Status().State)

	p.Stop()

	assert.Equal(t, "failed", p.Status().State)
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

	assert.Equal(t, "failed", p.Status().State)

	p.Stop()

	assert.Equal(t, "failed", p.Status().State)
}

func TestProcessFailed(t *testing.T) {
	p, _ := New(Config{
		Binary: "ffmpeg",
		Args: []string{
			"-i",
		},
		Reconnect:    false,
		StaleTimeout: 0,
	})

	p.Start()

	time.Sleep(5 * time.Second)

	p.Stop()

	assert.Equal(t, "failed", p.Status().State)
}

func TestFFmpegKill(t *testing.T) {
	p, _ := New(Config{
		Binary: "ffmpeg",
		Args: []string{
			"-f", "lavfi",
			"-i", "testsrc2",
			"-codec", "copy",
			"-f", "null",
			"-",
		},
		Reconnect:    false,
		StaleTimeout: 0,
	})

	p.Start()

	time.Sleep(5 * time.Second)

	p.Stop()

	time.Sleep(3 * time.Second)

	assert.Equal(t, "finished", p.Status().State)
}

func TestProcessForceKill(t *testing.T) {
	if err := exec.Command("go", "build", "-o", "./helper/ignoresigint", "./helper").Run(); err != nil {
		t.Errorf("Failed to build helper program: %s", err)
		return
	}

	p, _ := New(Config{
		Binary:       "./helper/ignoresigint",
		Args:         []string{},
		Reconnect:    false,
		StaleTimeout: 0,
	})

	p.Start()

	time.Sleep(3 * time.Second)

	p.Stop()

	time.Sleep(1 * time.Second)

	assert.Equal(t, "finishing", p.Status().State)

	time.Sleep(5 * time.Second)

	assert.Equal(t, "killed", p.Status().State)
}
