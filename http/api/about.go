package api

// About is some general information about the API
type About struct {
	App       string         `json:"app"`
	Auths     []string       `json:"auths"`
	Name      string         `json:"name"`
	ID        string         `json:"id"`
	CreatedAt string         `json:"created_at"` // RFC3339
	Uptime    uint64         `json:"uptime_seconds"`
	Version   AboutVersion   `json:"version"`
	Resources AboutResources `json:"resources"`
}

// AboutVersion is some information about the binary
type AboutVersion struct {
	Number   string `json:"number"`
	Commit   string `json:"repository_commit"`
	Branch   string `json:"repository_branch"`
	Build    string `json:"build_date"` // RFC3339
	Arch     string `json:"arch"`
	Compiler string `json:"compiler"`
}

// AboutResources holds information about the current resource usage
type AboutResources struct {
	IsThrottling bool    // Whether this core is currently throttling
	NCPU         float64 // Number of CPU on this node
	CPU          float64 // Current CPU load, 0-100*ncpu
	CPULimit     float64 // Defined CPU load limit, 0-100*ncpu
	CPUCore      float64 // Current CPU load of the core itself, 0-100*ncpu
	Mem          uint64  // Currently used memory in bytes
	MemLimit     uint64  // Defined memory limit in bytes
	MemTotal     uint64  // Total available memory in bytes
	MemCore      uint64  // Current used memory of the core itself in bytes
}

// MinimalAbout is the minimal information about the API
type MinimalAbout struct {
	App     string              `json:"app"`
	Auths   []string            `json:"auths"`
	Version AboutVersionMinimal `json:"version"`
}

type AboutVersionMinimal struct {
	Number string `json:"number"`
}
