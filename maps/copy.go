package maps

import "maps"

// Copy returns a shallow copy of a map
func Copy[A comparable, B any](a map[A]B) map[A]B {
	b := map[A]B{}

	maps.Copy(b, a)

	return b
}
