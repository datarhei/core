package api

// Process represents all information on a process
type Process struct {
	ID        string                 `json:"id" jsonschema:"minLength=1"`
	Owner     string                 `json:"owner"`
	Domain    string                 `json:"domain"`
	Type      string                 `json:"type" jsonschema:"enum=ffmpeg"`
	Reference string                 `json:"reference"`
	CreatedAt int64                  `json:"created_at" jsonschema:"minimum=0" format:"int64"` // Unix timestamp
	UpdatedAt int64                  `json:"updated_at" jsonschema:"minimum=0" format:"int64"` // Unix timestamp
	Config    *ProcessConfig         `json:"config,omitempty"`
	State     *ProcessState          `json:"state,omitempty"`
	Report    *ProcessReport         `json:"report,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// ProcessConfigIO represents an input or output of an ffmpeg process config
type ProcessConfigIO struct {
	ID      string                   `json:"id"`
	Address string                   `json:"address" validate:"required" jsonschema:"minLength=1"`
	Options []string                 `json:"options"`
	Cleanup []ProcessConfigIOCleanup `json:"cleanup,omitempty"`
}

type ProcessConfigIOCleanup struct {
	Pattern       string `json:"pattern" validate:"required"`
	MaxFiles      uint   `json:"max_files" format:"uint"`
	MaxFileAge    uint   `json:"max_file_age_seconds" format:"uint"` // seconds
	PurgeOnDelete bool   `json:"purge_on_delete"`
}

type ProcessConfigLimits struct {
	CPU     float64 `json:"cpu_usage" jsonschema:"minimum=0"`                       // percent 0-100*ncpu
	Memory  uint64  `json:"memory_mbytes" jsonschema:"minimum=0" format:"uint64"`   // megabytes
	WaitFor uint64  `json:"waitfor_seconds" jsonschema:"minimum=0" format:"uint64"` // seconds
}

// ProcessConfig represents the configuration of an ffmpeg process
type ProcessConfig struct {
	ID             string                 `json:"id"`
	Owner          string                 `json:"owner"`
	Domain         string                 `json:"domain"`
	Type           string                 `json:"type" validate:"oneof='ffmpeg' ''" jsonschema:"enum=ffmpeg,enum="`
	Reference      string                 `json:"reference"`
	Input          []ProcessConfigIO      `json:"input" validate:"required"`
	Output         []ProcessConfigIO      `json:"output" validate:"required"`
	Options        []string               `json:"options"`
	Reconnect      bool                   `json:"reconnect"`
	ReconnectDelay uint64                 `json:"reconnect_delay_seconds" format:"uint64"` // seconds
	Autostart      bool                   `json:"autostart"`
	StaleTimeout   uint64                 `json:"stale_timeout_seconds" format:"uint64"`    // seconds
	Timeout        uint64                 `json:"runtime_duration_seconds" format:"uint64"` // seconds
	Scheduler      string                 `json:"scheduler"`
	LogPatterns    []string               `json:"log_patterns"`
	Limits         ProcessConfigLimits    `json:"limits"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// ProcessState represents the current state of an ffmpeg process
type ProcessState struct {
	Order     string       `json:"order" jsonschema:"enum=start,enum=stop"`
	State     string       `json:"exec" jsonschema:"enum=finished,enum=starting,enum=running,enum=finishing,enum=killed,enum=failed"`
	Runtime   int64        `json:"runtime_seconds" jsonschema:"minimum=0" format:"int64"` // seconds
	Reconnect int64        `json:"reconnect_seconds" format:"int64"`                      // seconds
	LastLog   string       `json:"last_logline"`
	Progress  *Progress    `json:"progress"`
	Memory    uint64       `json:"memory_bytes" format:"uint64"`                            // bytes
	CPU       float64      `json:"cpu_usage" swaggertype:"number" jsonschema:"type=number"` // percent 0-100*ncpu
	Resources ProcessUsage `json:"resources"`
	Command   []string     `json:"command"`
}

type ProcessUsageCPU struct {
	NCPU    float64 `json:"ncpu" swaggertype:"number" jsonschema:"type=number"`
	Current float64 `json:"cur" swaggertype:"number" jsonschema:"type=number"`   // percent 0-100*ncpu
	Average float64 `json:"avg" swaggertype:"number" jsonschema:"type=number"`   // percent 0-100*ncpu
	Max     float64 `json:"max" swaggertype:"number" jsonschema:"type=number"`   // percent 0-100*ncpu
	Limit   float64 `json:"limit" swaggertype:"number" jsonschema:"type=number"` // percent 0-100*ncpu
}

type ProcessUsageMemory struct {
	Current uint64  `json:"cur" format:"uint64"`                               // bytes
	Average float64 `json:"avg" swaggertype:"number" jsonschema:"type=number"` // bytes
	Max     uint64  `json:"max" format:"uint64"`                               // bytes
	Limit   uint64  `json:"limit" format:"uint64"`                             // bytes
}

type ProcessUsage struct {
	CPU    ProcessUsageCPU    `json:"cpu_usage"`
	Memory ProcessUsageMemory `json:"memory_bytes"`
}
