package maps

// Copy returns a shallow copy of a map
func Copy[A comparable, B any](a map[A]B) map[A]B {
	b := map[A]B{}

	for k, v := range a {
		b[k] = v
	}

	return b
}
