package api

import "github.com/datarhei/core/v16/alias"

// Alias is an alias rule. Requests for a path that matches the pattern are served
// with the content of the target path.
type Alias struct {
	// Regular expression that is matched against the requested path. It is the ID of the alias.
	Pattern string `json:"pattern" validate:"required"`
	// Absolute path that will be served. Can reference capture groups of the pattern with $1, ${1}, or ${name}.
	Target string `json:"target" validate:"required"`
}

func (a *Alias) Marshal(r alias.Rule) {
	a.Pattern = r.Pattern
	a.Target = r.Target
}

func (a *Alias) Unmarshal() alias.Rule {
	return alias.Rule{
		Pattern: a.Pattern,
		Target:  a.Target,
	}
}
