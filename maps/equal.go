package maps

// Equal returns whether two maps are equal, same keys and
// same value for matching keys.
func Equal[A, B comparable](a map[A]B, b map[A]B) bool {
	if len(a) != len(b) {
		return false
	}

	for akey, avalue := range a {
		bvalue, ok := b[akey]
		if !ok {
			return false
		}

		if avalue != bvalue {
			return false
		}
	}

	return true
}
