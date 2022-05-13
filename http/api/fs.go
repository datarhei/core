package api

// FileInfo represents informatiion about a file on a filesystem
type FileInfo struct {
	Name    string `json:"name" jsonschema:"minLength=1"`
	Size    int64  `json:"size_bytes" jsonschema:"minimum=0"`
	LastMod int64  `json:"last_modified" jsonschema:"minimum=0"`
}
