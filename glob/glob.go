package glob

import (
	"strings"

	"github.com/gobwas/glob"
)

type Glob interface {
	Match(name string) bool
	Prefix() string
}

type globber struct {
	pattern string
	glob    glob.Glob
}

func MustCompile(pattern string, separators ...rune) Glob {
	g := glob.MustCompile(pattern, separators...)

	return &globber{pattern: pattern, glob: g}
}

func Compile(pattern string, separators ...rune) (Glob, error) {
	g, err := glob.Compile(pattern, separators...)
	if err != nil {
		return nil, err
	}

	return &globber{pattern: pattern, glob: g}, nil
}

func (g *globber) Match(name string) bool {
	return g.glob.Match(name)
}

func (g *globber) Prefix() string {
	return Prefix(g.pattern)
}

func Prefix(pattern string) string {
	index := strings.IndexAny(pattern, "*[{")
	if index == -1 {
		return pattern
	}

	return strings.Clone(pattern[:index])
}

func IsPattern(pattern string) bool {
	index := strings.IndexAny(pattern, "*[{")
	return index != -1
}

// Match returns whether the name matches the glob pattern, also considering
// one or several optionnal separator. An error is only returned if the pattern
// is invalid.
func Match(pattern, name string, separators ...rune) (bool, error) {
	g, err := Compile(pattern, separators...)
	if err != nil {
		return false, err
	}

	return g.Match(name), nil
}
