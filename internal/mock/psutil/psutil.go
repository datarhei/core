package psutil

import (
	"sync"

	"github.com/datarhei/core/v16/resources/psutil"
)

type MockPSUtil struct {
	Lock sync.Mutex

	CPUInfo psutil.CPUInfo
	MemInfo psutil.MemoryInfo
	GPUInfo []psutil.GPUInfo
}

func New(ngpu int) *MockPSUtil {
	u := &MockPSUtil{
		CPUInfo: psutil.CPUInfo{
			System: 10,
			User:   50,
			Idle:   35,
			Other:  5,
		},
		MemInfo: psutil.MemoryInfo{
			Total:     200,
			Available: 40,
			Used:      160,
		},
	}

	for i := 0; i < ngpu; i++ {
		u.GPUInfo = append(u.GPUInfo, psutil.GPUInfo{
			Index:       i,
			Name:        "L4",
			MemoryTotal: 24 * 1024 * 1024 * 1024,
			MemoryUsed:  uint64(12+i) * 1024 * 1024 * 1024,
			Usage:       50 - float64((i+1)*5),
			Encoder:     50 - float64((i+1)*10),
			Decoder:     50 - float64((i+1)*3),
		})
	}

	return u
}

func (u *MockPSUtil) Start()  {}
func (u *MockPSUtil) Cancel() {}

func (u *MockPSUtil) CPUCounts() (float64, error) {
	return 2, nil
}

func (u *MockPSUtil) CPU() (*psutil.CPUInfo, error) {
	u.Lock.Lock()
	defer u.Lock.Unlock()

	cpu := u.CPUInfo

	return &cpu, nil
}

func (u *MockPSUtil) Disk(path string) (*psutil.DiskInfo, error) {
	return &psutil.DiskInfo{}, nil
}

func (u *MockPSUtil) Memory() (*psutil.MemoryInfo, error) {
	u.Lock.Lock()
	defer u.Lock.Unlock()

	mem := u.MemInfo

	return &mem, nil
}

func (u *MockPSUtil) Network() ([]psutil.NetworkInfo, error) {
	return nil, nil
}

func (u *MockPSUtil) GPU() ([]psutil.GPUInfo, error) {
	u.Lock.Lock()
	defer u.Lock.Unlock()

	gpu := []psutil.GPUInfo{}

	gpu = append(gpu, u.GPUInfo...)

	return gpu, nil
}

func (u *MockPSUtil) Process(pid int32) (psutil.Process, error) {
	return &mockPSUtilProcess{}, nil
}

type mockPSUtilProcess struct{}

func (p *mockPSUtilProcess) CPU() (*psutil.CPUInfo, error) {
	s := &psutil.CPUInfo{
		System: 1,
		User:   2,
		Idle:   0,
		Other:  3,
	}

	return s, nil
}

func (p *mockPSUtilProcess) Memory() (uint64, error) { return 42, nil }
func (p *mockPSUtilProcess) GPU() (*psutil.GPUInfo, error) {
	return &psutil.GPUInfo{
		Index:       0,
		Name:        "L4",
		MemoryTotal: 128,
		MemoryUsed:  42,
		Usage:       5,
		Encoder:     9,
		Decoder:     7,
	}, nil
}
func (p *mockPSUtilProcess) Cancel()        {}
func (p *mockPSUtilProcess) Suspend() error { return nil }
func (p *mockPSUtilProcess) Resume() error  { return nil }
