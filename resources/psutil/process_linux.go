//go:build linux
// +build linux

package psutil

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tklauser/go-sysconf"
)

// Extracted from "github.com/shirou/gopsutil/v3/process/process_linux.go"
// We only need the CPU times. p.proc.Times() calls a function that is
// doing more than we actually need.

var clockTicks = 100 // default value

func init() {
	clkTck, err := sysconf.Sysconf(sysconf.SC_CLK_TCK)
	// ignore errors
	if err == nil {
		clockTicks = int(clkTck)
	}
}

func cpuTimes(pid int32) (*cpuTimesStat, error) {
	value := os.Getenv("HOST_PROC")
	if value == "" {
		value = "/proc"
	}

	path := filepath.Join(value, strconv.FormatInt(int64(pid), 10), "stat")

	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// Indexing from one, as described in `man proc` about the file /proc/[pid]/stat
	fields := splitProcStat(contents)

	utime, err := strconv.ParseFloat(fields[14], 64)
	if err != nil {
		return nil, err
	}

	stime, err := strconv.ParseFloat(fields[15], 64)
	if err != nil {
		return nil, err
	}

	// There is no such thing as iotime in stat file.  As an approximation, we
	// will use delayacct_blkio_ticks (aggregated block I/O delays, as per Linux
	// docs).  Note: I am assuming at least Linux 2.6.18
	var iotime float64
	if len(fields) > 42 {
		iotime, err = strconv.ParseFloat(fields[42], 64)
		if err != nil {
			iotime = 0 // Ancient linux version, most likely
		}
	} else {
		iotime = 0 // e.g. SmartOS containers
	}

	userTime := utime / float64(clockTicks)
	systemTime := stime / float64(clockTicks)
	iowaitTime := iotime / float64(clockTicks)

	s := &cpuTimesStat{
		total:  userTime + systemTime + iowaitTime,
		system: systemTime,
		user:   userTime,
		other:  iowaitTime,
	}

	if s.other < 0.0001 {
		s.other = 0
	}

	return s, nil
}

func splitProcStat(content []byte) []string {
	nameStart := bytes.IndexByte(content, '(')
	nameEnd := bytes.LastIndexByte(content, ')')
	restFields := strings.Fields(string(content[nameEnd+2:])) // +2 skip ') '
	name := content[nameStart+1 : nameEnd]
	pid := strings.TrimSpace(string(content[:nameStart]))
	fields := make([]string, 3, len(restFields)+3)
	fields[1] = string(pid)
	fields[2] = string(name)
	fields = append(fields, restFields...)
	return fields
}
