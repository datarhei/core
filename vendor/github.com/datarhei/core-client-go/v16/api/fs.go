package api

// FileInfo represents informatiion about a file on a filesystem
type FileInfo struct {
	Name    string `json:"name" jsonschema:"minLength=1"`
	Size    int64  `json:"size_bytes" jsonschema:"minimum=0"`
	LastMod int64  `json:"last_modified" jsonschema:"minimum=0"`
	CoreID  string `json:"core_id,omitempty"`
}

type FilesystemInfo struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Mount string `json:"mount"`
}

// FilesystemOperation represents a file operation on one or more filesystems
type FilesystemOperation struct {
	Operation string `json:"operation" validate:"required" enums:"copy,move" jsonschema:"enum=copy,enum=move"`
	Source    string `json:"source"`
	Target    string `json:"target"`
	RateLimit uint64 `json:"bandwidth_limit_kbit"` // kbit/s
}
