package forwarder

import (
	"fmt"

	apiclient "github.com/datarhei/core/v16/cluster/client"
	"github.com/datarhei/core/v16/cluster/store"
)

func reconstructError(err error) error {
	if cerr, ok := err.(apiclient.Error); ok {
		switch cerr.Code {
		case 400:
			err = fmt.Errorf("%s%w", err.Error(), store.ErrBadRequest)
		case 404:
			err = fmt.Errorf("%s%w", err.Error(), store.ErrNotFound)
		}
	}

	return err
}
