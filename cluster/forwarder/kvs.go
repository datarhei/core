package forwarder

import (
	"time"

	apiclient "github.com/datarhei/core/v16/cluster/client"
)

func (f *Forwarder) KVSet(origin, key, value string) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.SetKVRequest{
		Key:   key,
		Value: value,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return reconstructError(client.KVSet(origin, r))
}

func (f *Forwarder) KVUnset(origin, key string) error {
	if origin == "" {
		origin = f.ID
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return reconstructError(client.KVUnset(origin, key))
}

func (f *Forwarder) KVGet(origin, key string) (string, time.Time, error) {
	if origin == "" {
		origin = f.ID
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	value, at, err := client.KVGet(origin, key)

	return value, at, reconstructError(err)
}
