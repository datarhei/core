package forwarder

import (
	"time"

	apiclient "github.com/datarhei/core/v16/cluster/client"
)

func (f *Forwarder) SetKV(origin, key, value string) error {
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

	return client.SetKV(origin, r)
}

func (f *Forwarder) UnsetKV(origin, key string) error {
	if origin == "" {
		origin = f.ID
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.UnsetKV(origin, key)
}

func (f *Forwarder) GetKV(origin, key string) (string, time.Time, error) {
	if origin == "" {
		origin = f.ID
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return client.GetKV(origin, key)
}
