package slices

// Diff returns a sliceof newly added entries and a slice of removed entries based
// the provided slices.
func Diff[T comparable](next, current []T) ([]T, []T) {
	added, removed := []T{}, []T{}

	currentMap := map[T]struct{}{}

	for _, name := range current {
		currentMap[name] = struct{}{}
	}

	for _, name := range next {
		if _, ok := currentMap[name]; ok {
			delete(currentMap, name)
			continue
		}

		added = append(added, name)
	}

	for name := range currentMap {
		removed = append(removed, name)
	}

	return added, removed
}
