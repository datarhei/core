package nvidia

import (
	"bytes"
	"os"
	"sync"
	"testing"

	"github.com/datarhei/core/v16/internal/testhelper"
	"github.com/datarhei/core/v16/resources/psutil/gpu"
	"github.com/stretchr/testify/require"
)

func TestParseQuery(t *testing.T) {
	data, err := os.ReadFile("./fixtures/query1.xml")
	require.NoError(t, err)

	nv, err := parseQuery(data)
	require.NoError(t, err)

	require.Equal(t, Stats{
		GPU: []GPUStats{
			{
				ID:           "00000000:01:00.0",
				Name:         "NVIDIA GeForce GTX 1080",
				Architecture: "Pascal",
				MemoryTotal:  8119 * 1024 * 1024,
				MemoryUsed:   918 * 1024 * 1024,
				Usage:        15,
				UsageEncoder: 3,
				UsageDecoder: 0,
			},
		},
	}, nv)

	data, err = os.ReadFile("./fixtures/query2.xml")
	require.NoError(t, err)

	nv, err = parseQuery(data)
	require.NoError(t, err)

	require.Equal(t, Stats{
		GPU: []GPUStats{
			{
				ID:           "00000000:01:00.0",
				Name:         "NVIDIA L4",
				Architecture: "Ada Lovelace",
				MemoryTotal:  23034 * 1024 * 1024,
				MemoryUsed:   1 * 1024 * 1024,
				Usage:        2,
				UsageEncoder: 0,
				UsageDecoder: 0,
			},
			{
				ID:           "00000000:C1:00.0",
				Name:         "NVIDIA L4",
				Architecture: "Ada Lovelace",
				MemoryTotal:  23034 * 1024 * 1024,
				MemoryUsed:   1 * 1024 * 1024,
				Usage:        3,
				UsageEncoder: 0,
				UsageDecoder: 0,
			},
		},
	}, nv)

	data, err = os.ReadFile("./fixtures/query3.xml")
	require.NoError(t, err)

	nv, err = parseQuery(data)
	require.NoError(t, err)

	require.Equal(t, Stats{
		GPU: []GPUStats{
			{
				ID:           "00000000:01:00.0",
				Name:         "GeForce GTX 1080",
				MemoryTotal:  8119 * 1024 * 1024,
				MemoryUsed:   2006 * 1024 * 1024,
				Usage:        32,
				UsageEncoder: 17,
				UsageDecoder: 25,
			},
		},
	}, nv)
}

func TestParseProcess(t *testing.T) {
	data, err := os.ReadFile("./fixtures/process.txt")
	require.NoError(t, err)

	lines := bytes.Split(data, []byte("\n"))
	process := map[int32]Process{}

	for _, line := range lines {
		p, err := parseProcess(line)
		if err != nil {
			continue
		}

		process[p.PID] = p
	}

	require.Equal(t, map[int32]Process{
		7372: {
			Index:   0,
			PID:     7372,
			Memory:  136 * 1024 * 1024,
			Usage:   2,
			Encoder: 2,
			Decoder: 0,
		},
		12176: {
			Index:   0,
			PID:     12176,
			Memory:  782 * 1024 * 1024,
			Usage:   7,
			Encoder: 2,
			Decoder: 6,
		},
		20035: {
			Index:   0,
			PID:     20035,
			Memory:  1145 * 1024 * 1024,
			Usage:   7,
			Encoder: 4,
			Decoder: 3,
		},
		20141: {
			Index:   0,
			PID:     20141,
			Memory:  429 * 1024 * 1024,
			Usage:   5,
			Encoder: 1,
			Decoder: 3,
		},
		29591: {
			Index:   0,
			PID:     29591,
			Memory:  435 * 1024 * 1024,
			Usage:   0,
			Encoder: 1,
			Decoder: 1,
		},
	}, process)
}

func TestParseProcessNoProcesses(t *testing.T) {
	data, err := os.ReadFile("./fixtures/process_noprocesses.txt")
	require.NoError(t, err)

	lines := bytes.Split(data, []byte("\n"))
	process := map[int32]Process{}

	for _, line := range lines {
		p, err := parseProcess(line)
		if err != nil {
			continue
		}

		process[p.PID] = p
	}

	require.Equal(t, map[int32]Process{}, process)
}

func TestWriterQuery(t *testing.T) {
	data, err := os.ReadFile("./fixtures/query2.xml")
	require.NoError(t, err)

	wr := &writerQuery{
		ch:         make(chan Stats, 1),
		terminator: []byte("</nvidia_smi_log>"),
	}

	stats := Stats{}
	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()

		for s := range wr.ch {
			stats = s
		}
	}()

	_, err = wr.Write(data)
	require.NoError(t, err)

	close(wr.ch)

	wg.Wait()

	require.Equal(t, Stats{
		GPU: []GPUStats{
			{
				ID:           "00000000:01:00.0",
				Name:         "NVIDIA L4",
				Architecture: "Ada Lovelace",
				MemoryTotal:  23034 * 1024 * 1024,
				MemoryUsed:   1 * 1024 * 1024,
				Usage:        2,
				UsageEncoder: 0,
				UsageDecoder: 0,
			},
			{
				ID:           "00000000:C1:00.0",
				Name:         "NVIDIA L4",
				Architecture: "Ada Lovelace",
				MemoryTotal:  23034 * 1024 * 1024,
				MemoryUsed:   1 * 1024 * 1024,
				Usage:        3,
				UsageEncoder: 0,
				UsageDecoder: 0,
			},
		},
	}, stats)
}

func TestWriterProcess(t *testing.T) {
	data, err := os.ReadFile("./fixtures/process.txt")
	require.NoError(t, err)

	wr := &writerProcess{
		ch:         make(chan Process, 32),
		terminator: []byte("\n"),
	}

	process := map[int32]Process{}
	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()
		for p := range wr.ch {
			process[p.PID] = p
		}
	}()

	_, err = wr.Write(data)
	require.NoError(t, err)

	close(wr.ch)

	wg.Wait()

	require.Equal(t, map[int32]Process{
		7372: {
			Index:   0,
			PID:     7372,
			Memory:  136 * 1024 * 1024,
			Usage:   2,
			Encoder: 2,
			Decoder: 0,
		},
		12176: {
			Index:   0,
			PID:     12176,
			Memory:  782 * 1024 * 1024,
			Usage:   7,
			Encoder: 2,
			Decoder: 6,
		},
		20035: {
			Index:   0,
			PID:     20035,
			Memory:  1145 * 1024 * 1024,
			Usage:   7,
			Encoder: 4,
			Decoder: 3,
		},
		20141: {
			Index:   0,
			PID:     20141,
			Memory:  429 * 1024 * 1024,
			Usage:   5,
			Encoder: 1,
			Decoder: 3,
		},
		29591: {
			Index:   0,
			PID:     29591,
			Memory:  435 * 1024 * 1024,
			Usage:   0,
			Encoder: 1,
			Decoder: 1,
		},
	}, process)
}

func TestNvidiaGPUCount(t *testing.T) {
	binary, err := testhelper.BuildBinary("nvidia-smi")
	require.NoError(t, err, "Failed to build helper program")

	nv := New(binary)

	t.Cleanup(func() {
		nv.Close()
	})

	_, ok := nv.(*dummy)
	require.False(t, ok)

	count, err := nv.Count()
	require.NoError(t, err)
	require.NotEqual(t, 0, count)
}

func TestNvidiaGPUStats(t *testing.T) {
	binary, err := testhelper.BuildBinary("nvidia-smi")
	require.NoError(t, err, "Failed to build helper program")

	nv := New(binary)

	t.Cleanup(func() {
		nv.Close()
	})

	_, ok := nv.(*dummy)
	require.False(t, ok)

	stats, err := nv.Stats()
	require.NoError(t, err)
	require.Equal(t, []gpu.Stats{
		{
			Index:        0,
			ID:           "00000000:01:00.0",
			Name:         "NVIDIA L4",
			Architecture: "Ada Lovelace",
			MemoryTotal:  23034 * 1024 * 1024,
			MemoryUsed:   1 * 1024 * 1024,
			Usage:        2,
			Encoder:      0,
			Decoder:      0,
			Process: []gpu.Process{
				{
					Index:   0,
					PID:     7372,
					Memory:  136 * 1024 * 1024,
					Usage:   2,
					Encoder: 2,
					Decoder: 0,
				},
				{
					Index:   0,
					PID:     12176,
					Memory:  782 * 1024 * 1024,
					Usage:   5,
					Encoder: 3,
					Decoder: 7,
				},
				{
					Index:   0,
					PID:     29591,
					Memory:  435 * 1024 * 1024,
					Usage:   2,
					Encoder: 0,
					Decoder: 2,
				},
			},
		},
		{
			Index:        1,
			ID:           "00000000:C1:00.0",
			Name:         "NVIDIA L4",
			Architecture: "Ada Lovelace",
			MemoryTotal:  23034 * 1024 * 1024,
			MemoryUsed:   1 * 1024 * 1024,
			Usage:        3,
			Encoder:      0,
			Decoder:      0,
			Process: []gpu.Process{
				{
					Index:   1,
					PID:     20035,
					Memory:  1145 * 1024 * 1024,
					Usage:   8,
					Encoder: 4,
					Decoder: 1,
				},
				{
					Index:   1,
					PID:     20141,
					Memory:  429 * 1024 * 1024,
					Usage:   2,
					Encoder: 1,
					Decoder: 3,
				},
			},
		},
	}, stats)
}

func TestNvidiaGPUProcess(t *testing.T) {
	binary, err := testhelper.BuildBinary("nvidia-smi")
	require.NoError(t, err, "Failed to build helper program")

	nv := New(binary)

	t.Cleanup(func() {
		nv.Close()
	})

	_, ok := nv.(*dummy)
	require.False(t, ok)

	proc, err := nv.Process(12176)
	require.NoError(t, err)
	require.Equal(t, gpu.Process{
		Index:   0,
		PID:     12176,
		Memory:  782 * 1024 * 1024,
		Usage:   5,
		Encoder: 3,
		Decoder: 7,
	}, proc)
}
