package fs

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/datarhei/core/v16/io/fs"
	"github.com/datarhei/core/v16/log"
)

type Config struct {
	FS     fs.Filesystem
	Logger log.Logger
}

type Pattern struct {
	Pattern       string
	MaxFiles      uint
	MaxFileAge    time.Duration
	PurgeOnDelete bool
}

type Filesystem interface {
	fs.Filesystem

	// SetCleanup
	SetCleanup(id string, patterns []Pattern)

	// UnsetCleanup
	UnsetCleanup(id string)

	// Start
	Start()

	// Stop()
	Stop()
}

type filesystem struct {
	fs.Filesystem

	cleanupPatterns map[string][]Pattern
	cleanupLock     sync.RWMutex

	stopTicker context.CancelFunc

	startOnce sync.Once
	stopOnce  sync.Once

	logger log.Logger
}

func New(config Config) Filesystem {
	fs := &filesystem{
		Filesystem: config.FS,
		logger:     config.Logger,
	}

	if fs.logger == nil {
		fs.logger = log.New("")
	}

	fs.logger = fs.logger.WithFields(log.Fields{
		"name": config.FS.Name(),
		"type": config.FS.Type(),
	})

	fs.cleanupPatterns = make(map[string][]Pattern)

	// already drain the stop
	fs.stopOnce.Do(func() {})

	return fs
}

func (fs *filesystem) Start() {
	fs.startOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		fs.stopTicker = cancel
		go fs.cleanupTicker(ctx, time.Second)

		fs.stopOnce = sync.Once{}

		fs.logger.Debug().Log("Starting cleanup")
	})
}

func (fs *filesystem) Stop() {
	fs.stopOnce.Do(func() {
		fs.stopTicker()

		fs.startOnce = sync.Once{}

		fs.logger.Debug().Log("Stopping cleanup")
	})
}

func (fs *filesystem) SetCleanup(id string, patterns []Pattern) {
	if len(patterns) == 0 {
		return
	}

	for _, p := range patterns {
		fs.logger.Debug().WithFields(log.Fields{
			"id":           id,
			"pattern":      p.Pattern,
			"max_files":    p.MaxFiles,
			"max_file_age": p.MaxFileAge.Seconds(),
		}).Log("Add pattern")
	}

	fs.cleanupLock.Lock()
	defer fs.cleanupLock.Unlock()

	fs.cleanupPatterns[id] = append(fs.cleanupPatterns[id], patterns...)
}

func (fs *filesystem) UnsetCleanup(id string) {
	fs.logger.Debug().WithField("id", id).Log("Remove pattern group")

	fs.cleanupLock.Lock()
	defer fs.cleanupLock.Unlock()

	patterns := fs.cleanupPatterns[id]
	delete(fs.cleanupPatterns, id)

	fs.purge(patterns)
}

func (fs *filesystem) cleanup() {
	fs.cleanupLock.RLock()
	defer fs.cleanupLock.RUnlock()

	for _, patterns := range fs.cleanupPatterns {
		for _, pattern := range patterns {
			files := fs.Filesystem.List(pattern.Pattern)

			sort.Slice(files, func(i, j int) bool { return files[i].ModTime().Before(files[j].ModTime()) })

			if pattern.MaxFiles > 0 && uint(len(files)) > pattern.MaxFiles {
				for i := uint(0); i < uint(len(files))-pattern.MaxFiles; i++ {
					fs.logger.Debug().WithField("path", files[i].Name()).Log("Remove file because MaxFiles is exceeded")
					fs.Filesystem.Delete(files[i].Name())
				}
			}

			if pattern.MaxFileAge > 0 {
				bestBefore := time.Now().Add(-pattern.MaxFileAge)

				for _, f := range files {
					if f.ModTime().Before(bestBefore) {
						fs.logger.Debug().WithField("path", f.Name()).Log("Remove file because MaxFileAge is exceeded")
						fs.Filesystem.Delete(f.Name())
					}
				}
			}
		}
	}
}

func (fs *filesystem) purge(patterns []Pattern) (nfiles uint64) {
	for _, pattern := range patterns {
		if !pattern.PurgeOnDelete {
			continue
		}

		files := fs.Filesystem.List(pattern.Pattern)
		for _, f := range files {
			fs.logger.Debug().WithField("path", f.Name()).Log("Purging file")
			fs.Filesystem.Delete(f.Name())
			nfiles++
		}
	}

	return
}

func (fs *filesystem) cleanupTicker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fs.cleanup()
		}
	}
}
