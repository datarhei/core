// Package FS implements a FS that supports cleanup rules for removing files.
package fs

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/datarhei/core/v16/glob"
	"github.com/datarhei/core/v16/io/fs"
	"github.com/datarhei/core/v16/log"
)

type Config struct {
	FS       fs.Filesystem
	Interval time.Duration
	Logger   log.Logger
}

type Pattern struct {
	Pattern         string
	compiledPattern glob.Glob
	MaxFiles        uint
	MaxFileAge      time.Duration
	PurgeOnDelete   bool
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

	interval   time.Duration
	stopTicker context.CancelFunc

	startOnce sync.Once
	stopOnce  sync.Once

	logger log.Logger
}

func New(config Config) (Filesystem, error) {
	rfs := &filesystem{
		Filesystem: config.FS,
		interval:   config.Interval,
		logger:     config.Logger,
	}

	if rfs.interval <= time.Duration(0) {
		return nil, fmt.Errorf("interval must be greater than 0")
	}

	if rfs.logger == nil {
		rfs.logger = log.New("")
	}

	rfs.logger = rfs.logger.WithFields(log.Fields{
		"name": config.FS.Name(),
		"type": config.FS.Type(),
	})

	rfs.cleanupPatterns = make(map[string][]Pattern)

	// already drain the stop
	rfs.stopOnce.Do(func() {})

	return rfs, nil
}

func (rfs *filesystem) Start() {
	rfs.startOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		rfs.stopTicker = cancel
		go rfs.cleanupTicker(ctx, rfs.interval)

		rfs.stopOnce = sync.Once{}

		rfs.logger.Debug().Log("Starting cleanup")
	})
}

func (rfs *filesystem) Stop() {
	rfs.stopOnce.Do(func() {
		rfs.stopTicker()

		rfs.startOnce = sync.Once{}

		rfs.logger.Debug().Log("Stopping cleanup")
	})
}

func (rfs *filesystem) SetCleanup(id string, patterns []Pattern) {
	if len(patterns) == 0 {
		return
	}

	for i, p := range patterns {
		g, err := glob.Compile(p.Pattern, '/')
		if err != nil {
			continue
		}

		p.compiledPattern = g
		patterns[i] = p

		rfs.logger.Debug().WithFields(log.Fields{
			"id":           id,
			"pattern":      p.Pattern,
			"max_files":    p.MaxFiles,
			"max_file_age": p.MaxFileAge.Seconds(),
		}).Log("Add pattern")
	}

	rfs.cleanupLock.Lock()
	defer rfs.cleanupLock.Unlock()

	rfs.cleanupPatterns[id] = append(rfs.cleanupPatterns[id], patterns...)
}

func (rfs *filesystem) UnsetCleanup(id string) {
	rfs.logger.Debug().WithField("id", id).Log("Remove pattern group")

	rfs.cleanupLock.Lock()
	defer rfs.cleanupLock.Unlock()

	patterns := rfs.cleanupPatterns[id]
	delete(rfs.cleanupPatterns, id)

	rfs.purge(patterns)
}

func (rfs *filesystem) cleanup() {
	rfs.cleanupLock.RLock()
	nPatterns := len(rfs.cleanupPatterns)
	rfs.cleanupLock.RUnlock()

	if nPatterns == 0 {
		return
	}

	filesAndDirs := rfs.Filesystem.List("/", fs.ListOptions{})
	sort.SliceStable(filesAndDirs, func(i, j int) bool { return filesAndDirs[i].ModTime().Before(filesAndDirs[j].ModTime()) })

	rfs.cleanupLock.RLock()
	defer rfs.cleanupLock.RUnlock()

	for _, patterns := range rfs.cleanupPatterns {
		for _, pattern := range patterns {
			matchFiles := []fs.FileInfo{}
			for _, f := range filesAndDirs {
				if !pattern.compiledPattern.Match(f.Name()) {
					continue
				}

				matchFiles = append(matchFiles, f)
			}

			if pattern.MaxFiles > 0 && uint(len(matchFiles)) > pattern.MaxFiles {
				nFiles := uint(len(matchFiles)) - pattern.MaxFiles
				if nFiles > 0 {
					for _, f := range matchFiles {
						if f.IsDir() {
							continue
						}

						rfs.logger.Debug().WithField("path", f.Name()).Log("Remove file because MaxFiles is exceeded")
						rfs.Filesystem.Remove(f.Name())

						nFiles--
						if nFiles == 0 {
							break
						}
					}
				}
			}

			if pattern.MaxFileAge > 0 {
				bestBefore := time.Now().Add(-pattern.MaxFileAge)

				for _, f := range matchFiles {
					if f.IsDir() {
						continue
					}

					if f.ModTime().Before(bestBefore) {
						rfs.logger.Debug().WithField("path", f.Name()).Log("Remove file because MaxFileAge is exceeded")
						rfs.Filesystem.Remove(f.Name())
					}
				}
			}
		}
	}
}

func (rfs *filesystem) purge(patterns []Pattern) int64 {
	nfilesTotal := int64(0)

	for _, pattern := range patterns {
		if !pattern.PurgeOnDelete {
			continue
		}

		_, nfiles := rfs.Filesystem.RemoveList("/", fs.ListOptions{
			Pattern: pattern.Pattern,
		})

		nfilesTotal += nfiles
	}

	return nfilesTotal
}

func (rfs *filesystem) cleanupTicker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rfs.cleanup()
		}
	}
}
