package value

import (
	"fmt"
	"regexp"
	"strings"
)

// Alias maps request paths that match Pattern to the path given by Target. The
// target may reference capture groups of the pattern with $1, ${1}, or ${name}.
type Alias struct {
	Pattern string `json:"pattern"`
	Target  string `json:"target"`
}

func (a *Alias) String() string {
	return a.Pattern + aliasSeparator + a.Target
}

const aliasSeparator = "->"

// array of aliases
// ^/memfs/b/(.*)$->/memfs/a/$1 ^/data/x\.txt$->/data/y.txt

type aliasListValue struct {
	p         *[]Alias
	separator string
}

func NewAliasList(p *[]Alias, val []Alias, separator string) *aliasListValue {
	v := &aliasListValue{
		p:         p,
		separator: separator,
	}

	*p = val
	if *p == nil {
		*p = []Alias{}
	}

	return v
}

func (s *aliasListValue) Set(val string) error {
	list := []Alias{}

	for elm := range strings.SplitSeq(val, s.separator) {
		elm = strings.TrimSpace(elm)
		if len(elm) == 0 {
			continue
		}

		pattern, target, found := strings.Cut(elm, aliasSeparator)
		if !found {
			return fmt.Errorf("invalid alias '%s', expected 'pattern%starget'", elm, aliasSeparator)
		}

		list = append(list, Alias{
			Pattern: pattern,
			Target:  target,
		})
	}

	*s.p = list

	return nil
}

func (s *aliasListValue) String() string {
	if s.IsEmpty() {
		return "(empty)"
	}

	list := []string{}

	for _, a := range *s.p {
		list = append(list, a.String())
	}

	return strings.Join(list, s.separator)
}

func (s *aliasListValue) Validate() error {
	for i, a := range *s.p {
		if len(a.Pattern) == 0 {
			return fmt.Errorf("the pattern for alias %d is missing", i)
		}

		if _, err := regexp.Compile(a.Pattern); err != nil {
			return fmt.Errorf("the pattern for alias %d is invalid: %w", i, err)
		}

		if !strings.HasPrefix(a.Target, "/") {
			return fmt.Errorf("the target for alias %d must be an absolute path", i)
		}
	}

	return nil
}

func (s *aliasListValue) IsEmpty() bool {
	return len(*s.p) == 0
}
