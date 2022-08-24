package glob

import (
	"github.com/gobwas/glob"
)

// Match returns whether the name matches the glob pattern, also considering
// one or several optionnal separator. An error is only returned if the pattern
// is invalid.
func Match(pattern, name string, separators ...rune) (bool, error) {
	g, err := glob.Compile(pattern, separators...)
	if err != nil {
		return false, err
	}

	return g.Match(name), nil
}
