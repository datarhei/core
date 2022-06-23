package api

// SRTChannel represents details about a currently connected SRT publisher
type SRTChannel struct {
	Name string `json:"name" jsonschema:"minLength=1"`
}
