package forwarder

import (
	apiclient "github.com/datarhei/core/v16/cluster/client"
	"github.com/datarhei/core/v16/zone"
)

func (f *Forwarder) ZoneAdd(origin string, z zone.Zone) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.AddZoneRequest{
		Zone: z,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return reconstructError(client.ZoneAdd(origin, r))
}

func (f *Forwarder) ZoneUpdate(origin string, z zone.Zone) error {
	if origin == "" {
		origin = f.ID
	}

	r := apiclient.UpdateZoneRequest{
		Zone: z,
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return reconstructError(client.ZoneUpdate(origin, z.ID, r))
}

func (f *Forwarder) ZoneRemove(origin, id string) error {
	if origin == "" {
		origin = f.ID
	}

	f.lock.RLock()
	client := f.client
	f.lock.RUnlock()

	return reconstructError(client.ZoneRemove(origin, id))
}
