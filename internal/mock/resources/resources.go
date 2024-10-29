package resources

import (
	"github.com/datarhei/core/v16/internal/mock/psutil"
	"github.com/datarhei/core/v16/resources"
)

func New() resources.Resources {
	res, _ := resources.New(resources.Config{
		PSUtil: psutil.New(1),
	})

	return res
}

func NewWithLimits() resources.Resources {
	res, _ := resources.New(resources.Config{
		MaxCPU:       100,
		MaxMemory:    100,
		MaxGPU:       100,
		MaxGPUMemory: 100,
		PSUtil:       psutil.New(1),
	})

	return res
}
