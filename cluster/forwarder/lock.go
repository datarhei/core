package forwarder

import (
	"time"

	apiclient "github.com/datarhei/core/v16/cluster/client"
)

func (f *Forwarder) CreateLock(origin string, name string, validUntil time.Time) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.LockRequest{
		Name:       name,
		ValidUntil: validUntil,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.Lock(origin, r)
}

func (f *Forwarder) DeleteLock(origin string, name string) error {
	if origin == "" {
		origin = f.ID
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.Unlock(origin, name)
}
