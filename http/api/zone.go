package api

import (
	"github.com/datarhei/core/v16/zone"
)

// ZoneResources are the resources of a zone, either its limits or its usage. A limit of 0 means unlimited.
type ZoneResources struct {
	CPU        float64 `json:"cpu_usage" jsonschema:"minimum=0"`                         // percent 0-100*ncpu
	Memory     uint64  `json:"memory_mbytes" jsonschema:"minimum=0" format:"uint64"`     // megabytes
	GPUUsage   float64 `json:"gpu_usage" jsonschema:"minimum=0"`                         // percent 0-100
	GPUEncoder float64 `json:"gpu_encoder" jsonschema:"minimum=0"`                       // percent 0-100
	GPUDecoder float64 `json:"gpu_decoder" jsonschema:"minimum=0"`                       // percent 0-100
	GPUMemory  uint64  `json:"gpu_memory_mbytes" jsonschema:"minimum=0" format:"uint64"` // megabytes
}

func (r *ZoneResources) Marshal(z zone.Resources) {
	r.CPU = z.CPU
	r.Memory = z.Memory / 1024 / 1024
	r.GPUUsage = z.GPUUsage
	r.GPUEncoder = z.GPUEncoder
	r.GPUDecoder = z.GPUDecoder
	r.GPUMemory = z.GPUMemory / 1024 / 1024
}

func (r *ZoneResources) Unmarshal() zone.Resources {
	return zone.Resources{
		CPU:        r.CPU,
		Memory:     r.Memory * 1024 * 1024,
		GPUUsage:   r.GPUUsage,
		GPUEncoder: r.GPUEncoder,
		GPUDecoder: r.GPUDecoder,
		GPUMemory:  r.GPUMemory * 1024 * 1024,
	}
}

// Zone is a zone with its limits. The ID of the default zone is "$default".
type Zone struct {
	ID     string        `json:"id"`
	Limits ZoneResources `json:"limits"`
}

func (z *Zone) Marshal(a zone.Zone) {
	z.ID = a.ID
	z.Limits.Marshal(a.Limits)
}

func (z *Zone) Unmarshal() zone.Zone {
	return zone.Zone{
		ID:     z.ID,
		Limits: z.Limits.Unmarshal(),
	}
}

// ZoneInfo is a zone with its limits and the resources that the processes in this zone claim.
type ZoneInfo struct {
	Zone
	Usage     ZoneResources `json:"usage"`
	Processes uint64        `json:"processes" format:"uint64"` // Number of processes in this zone
}

func (z *ZoneInfo) Marshal(a zone.Info) {
	z.Zone.Marshal(a.Zone)
	z.Usage.Marshal(a.Usage)
	z.Processes = uint64(a.Processes)
}
