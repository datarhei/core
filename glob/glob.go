package glob

import (
	"github.com/gobwas/glob"
)

func Match(pattern, name string, separators ...rune) (bool, error) {
	g, err := glob.Compile(pattern, separators...)
	if err != nil {
		return false, err
	}

	return g.Match(name), nil
}
