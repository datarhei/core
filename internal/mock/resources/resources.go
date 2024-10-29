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
