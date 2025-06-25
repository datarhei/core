package psutil

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"regexp"
	"slices"
	"strconv"
	"sync"
	"time"
)

type Procfs interface {
	// Children returns all direct children of a process
	Children(ppid int32) []int32

	// AllChildren returns all children of a process
	AllChildren(ppid int32) []int32
}

type procfs struct {
	children map[int32][]int32

	lock sync.RWMutex
}

func NewProcfs(ctx context.Context, interval time.Duration) (Procfs, error) {
	p := &procfs{
		children: map[int32][]int32{},
	}

	children, err := p.createChildrenMap()
	if err != nil {
		return p, err
	}

	p.children = children

	go p.ticker(ctx, interval)

	return p, nil
}

func (p *procfs) Children(ppid int32) []int32 {
	p.lock.RLock()
	defer p.lock.RUnlock()

	pids, ok := p.children[ppid]
	if !ok {
		return []int32{}
	}

	return slices.Clone(pids)
}

func (p *procfs) AllChildren(ppid int32) []int32 {
	children := p.Children(ppid)

	allchildren := slices.Clone(children)

	for _, child := range children {
		allchildren = append(allchildren, p.AllChildren(child)...)
	}

	return allchildren
}

func (p *procfs) ticker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			children, err := p.createChildrenMap()
			if err == nil {
				p.lock.Lock()
				p.children = children
				p.lock.Unlock()
			}
		}
	}
}

func (p *procfs) createChildrenMap() (map[int32][]int32, error) {
	children := map[int32][]int32{}
	re := regexp.MustCompile(`^[0-9]+$`)

	proc := os.Getenv("HOST_PROC")
	if proc == "" {
		proc = "/proc"
	}

	fs := os.DirFS(proc).(fs.ReadDirFS)
	dirents, err := fs.ReadDir(".")
	if err != nil {
		return nil, err
	}

	for _, d := range dirents {
		if !d.IsDir() {
			continue
		}

		name := d.Name()

		if !re.MatchString(name) {
			continue
		}

		data, err := os.ReadFile(proc + "/" + name + "/stat")
		if err != nil {
			continue
		}

		fields := bytes.Split(data, []byte{' '})
		if len(fields) < 4 {
			continue
		}

		var pid int32 = 0
		var ppid int32 = 0

		if x, err := strconv.ParseInt(string(fields[3]), 10, 32); err == nil {
			ppid = int32(x)
		}

		if x, err := strconv.ParseInt(name, 10, 32); err == nil {
			pid = int32(x)
		}

		if pid == 0 {
			continue
		}

		c := children[ppid]
		c = append(c, pid)
		children[ppid] = c
	}

	return children, nil
}
