package psutil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	psutilgpu "github.com/datarhei/core/v16/resources/psutil/gpu"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

var cgroup1Files = []string{
	"cpu/cpu.cfs_quota_us",
	"cpu/cpu.cfs_period_us",
	"cpuacct/cpuacct.usage",
	"memory/memory.limit_in_bytes",
	"memory/memory.usage_in_bytes",
}

var cgroup2Files = []string{
	"cpu.max",
	"cpu.stat",
	"memory.max",
	"memory.current",
}

// https://github.com/netdata/netdata/blob/master/collectors/cgroups.plugin/sys_fs_cgroup.c
// https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/6/html/resource_management_guide/sec-cpuacct
// https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/6/html/resource_management_guide/sect-cpu-example_usage
// https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html

type DiskInfo struct {
	Path        string
	Fstype      string
	Total       uint64
	Used        uint64
	InodesTotal uint64
	InodesUsed  uint64
}

type MemoryInfo struct {
	Total     uint64 // bytes
	Available uint64 // bytes
	Used      uint64 // bytes
}

type NetworkInfo struct {
	Name      string // interface name
	BytesSent uint64 // number of bytes sent
	BytesRecv uint64 // number of bytes received
}

type CPUInfo struct {
	System float64 // percent 0-100
	User   float64 // percent 0-100
	Idle   float64 // percent 0-100
	Other  float64 // percent 0-100
}

type GPUInfo struct {
	Index int    // Index of the GPU
	Name  string // Name of the GPU (not populated for a specific process)

	MemoryTotal uint64 // bytes (not populated for a specific process)
	MemoryUsed  uint64 // bytes

	Usage   float64 // percent 0-100
	Encoder float64 // percent 0-100
	Decoder float64 // percent 0-100
}

type cpuTimesStat struct {
	total  float64 // seconds
	system float64 // seconds
	user   float64 // seconds
	idle   float64 // seconds
	other  float64 // seconds
}

type Util interface {
	Start()
	Stop()

	// CPUCounts returns the number of cores, either logical or physical.
	CPUCounts() (float64, error)

	// CPU returns the current CPU load in percent. The values range
	// from 0 to 100, independently of the number of logical cores.
	CPU() (*CPUInfo, error)

	// Disk returns the current usage of the partition specified by the path.
	Disk(path string) (*DiskInfo, error)

	// Memory return the current memory usage.
	Memory() (*MemoryInfo, error)

	// Network returns the current network interface statistics per network adapter.
	Network() ([]NetworkInfo, error)

	// GPU return the current usage for each CPU.
	GPU() ([]GPUInfo, error)

	// Process returns a process observer for a process with the given pid.
	Process(pid int32) (Process, error)
}

type util struct {
	root fs.FS

	cpuLimit   uint64  // Max. allowed CPU time in nanoseconds per second
	ncpu       float64 // Actual available CPUs
	hasCgroup  bool
	cgroupType int

	stopTicker context.CancelFunc
	startOnce  sync.Once
	stopOnce   sync.Once

	lock             sync.RWMutex
	statCurrent      cpuTimesStat
	statCurrentTime  time.Time
	statPrevious     cpuTimesStat
	statPreviousTime time.Time
	nTicks           uint64
	mem              MemoryInfo

	gpu psutilgpu.GPU
}

// New returns a new util, it will be started automatically
func New(root string, gpu psutilgpu.GPU) (Util, error) {
	if len(root) == 0 {
		root = "/sys/fs/cgroup"
	}

	u := &util{
		root: os.DirFS(root),
	}

	u.cgroupType = u.detectCgroupVersion()
	if u.cgroupType != 0 {
		u.hasCgroup = true
	}

	if u.hasCgroup {
		u.cpuLimit, u.ncpu = u.cgroupCPULimit(u.cgroupType)
	}

	if u.ncpu == 0 {
		var err error
		u.ncpu, err = u.CPUCounts()
		if err != nil {
			return nil, err
		}
	}

	mem, err := u.virtualMemory()
	if err != nil {
		return nil, fmt.Errorf("unable to determine system memory: %w", err)
	}

	u.mem = *mem

	u.gpu = gpu
	if u.gpu == nil {
		u.gpu = psutilgpu.NewNilGPU()
	}

	u.stopOnce.Do(func() {})

	u.Start()

	return u, nil
}

func (u *util) Start() {
	u.startOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		u.stopTicker = cancel

		go u.tickCPU(ctx, time.Second)
		go u.tickMemory(ctx, time.Second)
	})
}

func (u *util) Stop() {
	u.stopOnce.Do(func() {
		u.stopTicker()

		u.startOnce = sync.Once{}
	})
}

func (u *util) detectCgroupVersion() int {
	f, err := u.root.Open(".")
	if err != nil {
		// no cgroup available
		return 0
	}

	f.Close()

	for _, file := range cgroup1Files {
		if f, err := u.root.Open(file); err == nil {
			f.Close()
			return 1
		}
	}

	for _, file := range cgroup2Files {
		if f, err := u.root.Open(file); err == nil {
			f.Close()
			return 2
		}
	}

	return 0
}

func (u *util) cgroupCPULimit(version int) (uint64, float64) {
	if version == 1 {
		lines, err := u.readFile("cpu/cpu.cfs_quota_us")
		if err != nil {
			return 0, 0
		}

		quota, err := strconv.ParseFloat(lines[0], 64) // microseconds
		if err != nil {
			return 0, 0
		}

		if quota > 0 {
			lines, err := u.readFile("cpu/cpu.cfs_period_us")
			if err != nil {
				return 0, 0
			}

			period, err := strconv.ParseFloat(lines[0], 64) // microseconds
			if err != nil {
				return 0, 0
			}

			return uint64(1e6/period*quota) * 1e3, quota / period // nanoseconds
		}
	} else if version == 2 {
		lines, err := u.readFile("cpu.max")
		if err != nil {
			return 0, 0
		}

		if strings.HasPrefix(lines[0], "max") {
			return 0, 0
		}

		fields := strings.Split(lines[0], " ")
		if len(fields) != 2 {
			return 0, 0
		}

		quota, err := strconv.ParseFloat(fields[0], 64) // microseconds
		if err != nil {
			return 0, 0
		}

		period, err := strconv.ParseFloat(fields[1], 64) // microseconds
		if err != nil {
			return 0, 0
		}

		return uint64(1e6/period*quota) * 1e3, quota / period // nanoseconds
	}

	return 0, 0
}

func (u *util) tickCPU(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			stat := u.collectCPU()

			u.lock.Lock()
			u.statPrevious, u.statCurrent = u.statCurrent, stat
			u.statPreviousTime, u.statCurrentTime = u.statCurrentTime, t
			u.nTicks++
			u.lock.Unlock()
		}
	}
}

func (u *util) collectCPU() cpuTimesStat {
	stat, err := u.cpuTimes()
	if err != nil {
		return cpuTimesStat{
			total: float64(time.Now().Unix()),
			idle:  float64(time.Now().Unix()),
		}
	}

	return *stat
}

func (u *util) tickMemory(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stat := u.collectMemory()
			if stat != nil {
				u.lock.Lock()
				u.mem = *stat
				u.lock.Unlock()
			}
		}
	}
}

func (u *util) collectMemory() *MemoryInfo {
	stat, err := u.virtualMemory()
	if err != nil {
		return nil
	}

	return stat
}

func (u *util) CPUCounts() (float64, error) {
	if u.hasCgroup && u.ncpu > 0 {
		return u.ncpu, nil
	}

	ncpu, err := cpu.Counts(true)
	if err != nil {
		return 0, err
	}

	return float64(ncpu), nil
}

// cpuTimes returns the current cpu usage times in seconds.
func (u *util) cpuTimes() (*cpuTimesStat, error) {
	if u.hasCgroup && u.cpuLimit > 0 {
		if stat, err := u.cgroupCPUTimes(u.cgroupType); err == nil {
			return stat, nil
		}
	}

	times, err := cpu.Times(true)
	if err != nil {
		return nil, err
	}

	if len(times) == 0 {
		return nil, errors.New("cpu.Times() returned an empty slice")
	}

	s := &cpuTimesStat{}

	for _, t := range times {
		s.total += cpuTotal(&t)
		s.system += t.System
		s.user += t.User
		s.idle += t.Idle

		s.other = s.total - s.system - s.user - s.idle
		if s.other < 0.0001 {
			s.other = 0
		}
	}

	return s, nil
}

func (u *util) CPU() (*CPUInfo, error) {
	var total float64

	for {
		u.lock.RLock()
		nTicks := u.nTicks
		u.lock.RUnlock()

		if nTicks < 2 {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		break
	}

	u.lock.RLock()
	defer u.lock.RUnlock()

	if u.hasCgroup && u.cpuLimit > 0 {
		total = float64(u.cpuLimit) * (u.statCurrentTime.Sub(u.statPreviousTime)).Seconds()
	} else {
		total = (u.statCurrent.total - u.statPrevious.total)
	}

	s := &CPUInfo{
		System: 0,
		User:   0,
		Idle:   100,
		Other:  0,
	}

	if total == 0 {
		return s, nil
	}

	s.System = 100 * (u.statCurrent.system - u.statPrevious.system) / total
	s.User = 100 * (u.statCurrent.user - u.statPrevious.user) / total
	s.Idle = 100 * (u.statCurrent.idle - u.statPrevious.idle) / total
	s.Other = 100 * (u.statCurrent.other - u.statPrevious.other) / total

	if u.hasCgroup && u.cpuLimit > 0 {
		s.Idle = 100 - s.User - s.System
	}

	return s, nil
}

func (u *util) cgroupCPUTimes(version int) (*cpuTimesStat, error) {
	info := &cpuTimesStat{}

	if version == 1 {
		lines, err := u.readFile("cpuacct/cpuacct.usage")
		if err != nil {
			return nil, err
		}

		usage, err := strconv.ParseFloat(lines[0], 64) // nanoseconds
		if err != nil {
			return nil, err
		}

		info.system = usage
	} else if version == 2 {
		lines, err := u.readFile("cpu.stat")
		if err != nil {
			return nil, err
		}

		var usage float64

		if _, err := fmt.Sscanf(lines[0], "usage_usec %f", &usage); err != nil {
			return nil, err
		}

		info.system = usage * 1e3 // convert to nanoseconds
	}

	return info, nil
}

func (u *util) Disk(path string) (*DiskInfo, error) {
	usage, err := disk.Usage(path)
	if err != nil {
		return nil, err
	}

	info := &DiskInfo{
		Path:        usage.Path,
		Fstype:      usage.Fstype,
		Total:       usage.Total,
		Used:        usage.Used,
		InodesTotal: usage.InodesTotal,
		InodesUsed:  usage.InodesUsed,
	}

	return info, nil
}

func (u *util) virtualMemory() (*MemoryInfo, error) {
	info, err := mem.VirtualMemory()
	if err != nil {
		return nil, err
	}

	if u.hasCgroup {
		if cginfo, err := u.cgroupVirtualMemory(u.cgroupType); err == nil {
			// if total is a huge garbage number, then there are no limits set
			if cginfo.Total <= info.Total {
				return cginfo, nil
			}
		}
	}

	return &MemoryInfo{
		Total:     info.Total,
		Available: info.Available,
		Used:      info.Used,
	}, nil
}

func (u *util) Memory() (*MemoryInfo, error) {
	u.lock.RLock()
	defer u.lock.RUnlock()

	stat := &MemoryInfo{
		Total:     u.mem.Total,
		Available: u.mem.Available,
		Used:      u.mem.Used,
	}

	return stat, nil
}

func (u *util) cgroupVirtualMemory(version int) (*MemoryInfo, error) {
	info := &MemoryInfo{}

	if version == 1 {
		lines, err := u.readFile("memory/memory.limit_in_bytes")
		if err != nil {
			return nil, err
		}

		total, err := strconv.ParseUint(lines[0], 10, 64)
		if err != nil {
			return nil, err
		}

		lines, err = u.readFile("memory/memory.usage_in_bytes")
		if err != nil {
			return nil, err
		}

		used, err := strconv.ParseUint(lines[0], 10, 64)
		if err != nil {
			return nil, err
		}

		info.Total = total
		info.Available = total - used
		info.Used = used
	} else if version == 2 {
		lines, err := u.readFile("memory.max")
		if err != nil {
			return nil, err
		}

		total, err := strconv.ParseUint(lines[0], 10, 64)
		if err != nil {
			total = uint64(math.MaxUint64)
		}

		lines, err = u.readFile("memory.current")
		if err != nil {
			return nil, err
		}

		used, err := strconv.ParseUint(lines[0], 10, 64)
		if err != nil {
			return nil, err
		}

		info.Total = total
		info.Available = total - used
		info.Used = used
	}

	return info, nil
}

func (u *util) Network() ([]NetworkInfo, error) {
	netio, err := net.IOCounters(true)
	if err != nil {
		return nil, err
	}

	info := []NetworkInfo{}

	for _, io := range netio {
		info = append(info, NetworkInfo{
			Name:      io.Name,
			BytesSent: io.BytesSent,
			BytesRecv: io.BytesRecv,
		})
	}

	return info, nil
}

func (u *util) readFile(path string) ([]string, error) {
	file, err := u.root.Open(path)
	if err != nil {
		return nil, err
	}

	data := []byte{}
	buf := make([]byte, 20148)
	for {
		n, err := file.Read(buf)

		if n > 0 {
			data = append(data, buf[:n]...)
		}

		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
	}

	lines := strings.Split(string(data), "\n")

	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}

	return lines, nil
}

func cpuTotal(c *cpu.TimesStat) float64 {
	return c.User + c.System + c.Idle + c.Nice + c.Iowait + c.Irq +
		c.Softirq + c.Steal + c.Guest + c.GuestNice
}

func (u *util) GPU() ([]GPUInfo, error) {
	nvstats, err := u.gpu.Stats()
	if err != nil {
		return nil, err
	}

	stats := []GPUInfo{}

	for _, nv := range nvstats {
		stats = append(stats, GPUInfo{
			Name:        nv.Name,
			MemoryTotal: nv.MemoryTotal,
			MemoryUsed:  nv.MemoryUsed,
			Usage:       nv.Usage,
			Encoder:     nv.Encoder,
			Decoder:     nv.Decoder,
		})
	}

	return stats, nil
}
