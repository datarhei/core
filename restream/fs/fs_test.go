package fs

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/datarhei/core/v16/io/fs"
	"github.com/datarhei/core/v16/math/rand"

	"github.com/stretchr/testify/require"
)

func TestUpdateCleanup(t *testing.T) {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	clean, err := New(Config{
		FS:       memfs,
		Interval: time.Second,
	})
	require.NoError(t, err)

	cleanfs := clean.(*filesystem)

	cleanfs.Start()

	patterns := []Pattern{
		{
			Pattern:    "/*.ts",
			MaxFiles:   3,
			MaxFileAge: 0,
		},
	}

	cleanfs.UpdateCleanup("foobar", patterns, true)

	require.Equal(t, cleanfs.cleanupPatterns["foobar"], patterns)

	patterns = append(patterns, Pattern{
		Pattern:    "/*.m3u8",
		MaxFiles:   5,
		MaxFileAge: 0,
	})

	cleanfs.UpdateCleanup("foobar", patterns, true)

	require.Equal(t, cleanfs.cleanupPatterns["foobar"], patterns)

	patterns[0].MaxFiles = 42

	cleanfs.UpdateCleanup("foobar", patterns, true)

	require.Equal(t, cleanfs.cleanupPatterns["foobar"], patterns)

	cleanfs.UpdateCleanup("foobar", patterns[1:], true)

	require.Equal(t, cleanfs.cleanupPatterns["foobar"], patterns[1:])

	cleanfs.UpdateCleanup("foobar", nil, true)

	require.Empty(t, cleanfs.cleanupPatterns["foobar"])
}

func TestMaxFiles(t *testing.T) {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	cleanfs, err := New(Config{
		FS:       memfs,
		Interval: time.Second,
	})
	require.NoError(t, err)

	cleanfs.Start()

	cleanfs.UpdateCleanup("foobar", []Pattern{
		{
			Pattern:    "/*.ts",
			MaxFiles:   3,
			MaxFileAge: 0,
		},
	}, true)

	cleanfs.WriteFileReader("/chunk_0.ts", strings.NewReader("chunk_0"), -1)
	cleanfs.WriteFileReader("/chunk_1.ts", strings.NewReader("chunk_1"), -1)
	cleanfs.WriteFileReader("/chunk_2.ts", strings.NewReader("chunk_2"), -1)

	require.Eventually(t, func() bool {
		return cleanfs.Files() == 3
	}, 3*time.Second, time.Second)

	cleanfs.WriteFileReader("/chunk_3.ts", strings.NewReader("chunk_3"), -1)

	require.Eventually(t, func() bool {
		if cleanfs.Files() != 3 {
			return false
		}

		names := []string{}

		for _, f := range cleanfs.List("/", fs.ListOptions{Pattern: "/*.ts"}) {
			names = append(names, f.Name())
		}

		require.ElementsMatch(t, []string{"/chunk_1.ts", "/chunk_2.ts", "/chunk_3.ts"}, names)

		return true
	}, 3*time.Second, time.Second)

	cleanfs.Stop()
}

func TestMaxAge(t *testing.T) {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	cleanfs, err := New(Config{
		FS:       memfs,
		Interval: time.Second,
	})
	require.NoError(t, err)

	cleanfs.Start()

	cleanfs.UpdateCleanup("foobar", []Pattern{
		{
			Pattern:    "/*.ts",
			MaxFiles:   0,
			MaxFileAge: 3 * time.Second,
		},
	}, true)

	cleanfs.WriteFileReader("/chunk_0.ts", strings.NewReader("chunk_0"), -1)
	cleanfs.WriteFileReader("/chunk_1.ts", strings.NewReader("chunk_1"), -1)
	cleanfs.WriteFileReader("/chunk_2.ts", strings.NewReader("chunk_2"), -1)

	require.Eventually(t, func() bool {
		return cleanfs.Files() == 0
	}, 10*time.Second, time.Second)

	cleanfs.WriteFileReader("/chunk_3.ts", strings.NewReader("chunk_3"), -1)

	require.Eventually(t, func() bool {
		if cleanfs.Files() != 1 {
			return false
		}

		names := []string{}

		for _, f := range cleanfs.List("/", fs.ListOptions{Pattern: "/*.ts"}) {
			names = append(names, f.Name())
		}

		require.ElementsMatch(t, []string{"/chunk_3.ts"}, names)

		return true
	}, 5*time.Second, time.Second)

	cleanfs.Stop()
}

func TestUnsetCleanup(t *testing.T) {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	cleanfs, err := New(Config{
		FS:       memfs,
		Interval: time.Second,
	})
	require.NoError(t, err)

	cleanfs.Start()

	cleanfs.UpdateCleanup("foobar", []Pattern{
		{
			Pattern:    "/*.ts",
			MaxFiles:   3,
			MaxFileAge: 0,
		},
	}, true)

	cleanfs.WriteFileReader("/chunk_0.ts", strings.NewReader("chunk_0"), -1)
	cleanfs.WriteFileReader("/chunk_1.ts", strings.NewReader("chunk_1"), -1)
	cleanfs.WriteFileReader("/chunk_2.ts", strings.NewReader("chunk_2"), -1)

	require.Eventually(t, func() bool {
		return cleanfs.Files() == 3
	}, 3*time.Second, time.Second)

	cleanfs.WriteFileReader("/chunk_3.ts", strings.NewReader("chunk_3"), -1)

	require.Eventually(t, func() bool {
		if cleanfs.Files() != 3 {
			return false
		}

		names := []string{}

		for _, f := range cleanfs.List("/", fs.ListOptions{Pattern: "/*.ts"}) {
			names = append(names, f.Name())
		}

		require.ElementsMatch(t, []string{"/chunk_1.ts", "/chunk_2.ts", "/chunk_3.ts"}, names)

		return true
	}, 3*time.Second, time.Second)

	cleanfs.UpdateCleanup("foobar", nil, true)

	cleanfs.WriteFileReader("/chunk_4.ts", strings.NewReader("chunk_4"), -1)

	require.Eventually(t, func() bool {
		if cleanfs.Files() != 4 {
			return false
		}

		names := []string{}

		for _, f := range cleanfs.List("/", fs.ListOptions{Pattern: "/*.ts"}) {
			names = append(names, f.Name())
		}

		require.ElementsMatch(t, []string{"/chunk_1.ts", "/chunk_2.ts", "/chunk_3.ts", "/chunk_4.ts"}, names)

		return true
	}, 3*time.Second, time.Second)

	cleanfs.Stop()
}

func TestPurge(t *testing.T) {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(t, err)

	cleanfs, err := New(Config{
		FS:       memfs,
		Interval: time.Second,
	})
	require.NoError(t, err)

	cleanfs.Start()

	cleanfs.UpdateCleanup("foobar", []Pattern{
		{
			Pattern:       "/*.ts",
			MaxFiles:      3,
			MaxFileAge:    0,
			PurgeOnDelete: true,
		},
	}, true)

	cleanfs.WriteFileReader("/chunk_0.ts", strings.NewReader("chunk_0"), -1)
	cleanfs.WriteFileReader("/chunk_1.ts", strings.NewReader("chunk_1"), -1)
	cleanfs.WriteFileReader("/chunk_2.ts", strings.NewReader("chunk_2"), -1)

	require.Eventually(t, func() bool {
		return cleanfs.Files() == 3
	}, 3*time.Second, time.Second)

	cleanfs.WriteFileReader("/chunk_3.ts", strings.NewReader("chunk_3"), -1)

	require.Eventually(t, func() bool {
		if cleanfs.Files() != 3 {
			return false
		}

		names := []string{}

		for _, f := range cleanfs.List("/", fs.ListOptions{Pattern: "/*.ts"}) {
			names = append(names, f.Name())
		}

		require.ElementsMatch(t, []string{"/chunk_1.ts", "/chunk_2.ts", "/chunk_3.ts"}, names)

		return true
	}, 3*time.Second, time.Second)

	cleanfs.UpdateCleanup("foobar", nil, true)

	cleanfs.WriteFileReader("/chunk_4.ts", strings.NewReader("chunk_4"), -1)

	require.Eventually(t, func() bool {
		if cleanfs.Files() != 1 {
			return false
		}

		names := []string{}

		for _, f := range cleanfs.List("/", fs.ListOptions{Pattern: "/*.ts"}) {
			names = append(names, f.Name())
		}

		require.ElementsMatch(t, []string{"/chunk_4.ts"}, names)

		return true
	}, 3*time.Second, time.Second)

	cleanfs.Stop()
}

func BenchmarkCleanup(b *testing.B) {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(b, err)

	cleanfs, err := New(Config{
		FS:       memfs,
		Interval: time.Second,
	})
	require.NoError(b, err)

	nProcs := 200

	ids := make([]string, nProcs)

	for i := 0; i < nProcs; i++ {
		id := rand.StringAlphanumeric(8)

		patterns := []Pattern{
			{
				Pattern:       fmt.Sprintf("/%d/%s.m3u8", i, id),
				MaxFiles:      2,
				MaxFileAge:    0,
				PurgeOnDelete: true,
			},
			{
				Pattern:       fmt.Sprintf("/%d/%s_0.m3u8", i, id),
				MaxFiles:      2,
				MaxFileAge:    0,
				PurgeOnDelete: true,
			},
			{
				Pattern:       fmt.Sprintf("/%d/%s_1.m3u8", i, id),
				MaxFiles:      2,
				MaxFileAge:    0,
				PurgeOnDelete: true,
			},
			{
				Pattern:       fmt.Sprintf("/%d/%s_0_*.ts", i, id),
				MaxFiles:      16,
				MaxFileAge:    0,
				PurgeOnDelete: true,
			},
			{
				Pattern:       fmt.Sprintf("/%d/%s_1_*.ts", i, id),
				MaxFiles:      16,
				MaxFileAge:    0,
				PurgeOnDelete: true,
			},
		}

		cleanfs.UpdateCleanup(id, patterns, true)

		ids[i] = id
	}

	// Fill the filesystem with files
	for j := 0; j < nProcs; j++ {
		path := fmt.Sprintf("/%d/%s.m3u8", j, ids[j])
		memfs.WriteFile(path, []byte("foobar"))
		path = fmt.Sprintf("/%d/%s_0.m3u8", j, ids[j])
		memfs.WriteFile(path, []byte("foobar"))
		path = fmt.Sprintf("/%d/%s_1.m3u8", j, ids[j])
		memfs.WriteFile(path, []byte("foobar"))
		for k := 0; k < 20; k++ {
			path = fmt.Sprintf("/%d/%s_0_%d.ts", j, ids[j], k)
			memfs.WriteFile(path, []byte("foobar"))
			path = fmt.Sprintf("/%d/%s_1_%d.ts", j, ids[j], k)
			memfs.WriteFile(path, []byte("foobar"))
		}
	}

	rfs := cleanfs.(*filesystem)

	for b.Loop() {
		rfs.cleanup()
	}
}

func BenchmarkPurge(b *testing.B) {
	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	require.NoError(b, err)

	cleanfs, err := New(Config{
		FS:       memfs,
		Interval: time.Second,
	})
	require.NoError(b, err)

	nProcs := 200

	ids := make([]string, nProcs)

	for i := range nProcs {
		id := rand.StringAlphanumeric(8)

		patterns := []Pattern{
			{
				Pattern:       fmt.Sprintf("/%d/%s.m3u8", i, id),
				MaxFiles:      2,
				MaxFileAge:    0,
				PurgeOnDelete: true,
			},
			{
				Pattern:       fmt.Sprintf("/%d/%s_0.m3u8", i, id),
				MaxFiles:      2,
				MaxFileAge:    0,
				PurgeOnDelete: true,
			},
			{
				Pattern:       fmt.Sprintf("/%d/%s_1.m3u8", i, id),
				MaxFiles:      2,
				MaxFileAge:    0,
				PurgeOnDelete: true,
			},
			{
				Pattern:       fmt.Sprintf("/%d/%s_0_*.ts", i, id),
				MaxFiles:      16,
				MaxFileAge:    0,
				PurgeOnDelete: true,
			},
			{
				Pattern:       fmt.Sprintf("/%d/%s_1_*.ts", i, id),
				MaxFiles:      16,
				MaxFileAge:    0,
				PurgeOnDelete: true,
			},
		}

		cleanfs.UpdateCleanup(id, patterns, false)

		ids[i] = id
	}

	// Fill the filesystem with files
	for j := range nProcs {
		path := fmt.Sprintf("/%d/%s.m3u8", j, ids[j])
		memfs.WriteFile(path, []byte("foobar"))
		path = fmt.Sprintf("/%d/%s_0.m3u8", j, ids[j])
		memfs.WriteFile(path, []byte("foobar"))
		path = fmt.Sprintf("/%d/%s_1.m3u8", j, ids[j])
		memfs.WriteFile(path, []byte("foobar"))
		for k := 0; k < 20; k++ {
			path = fmt.Sprintf("/%d/%s_0_%d.ts", j, ids[j], k)
			memfs.WriteFile(path, []byte("foobar"))
			path = fmt.Sprintf("/%d/%s_1_%d.ts", j, ids[j], k)
			memfs.WriteFile(path, []byte("foobar"))
		}
	}

	rfs := cleanfs.(*filesystem)

	for b.Loop() {
		rfs.purge(rfs.cleanupPatterns[ids[42]])
	}
}
