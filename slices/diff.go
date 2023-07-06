package slices

// DiffComparable diffs two arrays/slices and returns slices of elements that are only in A and only in B.
// If some element is present multiple times, each instance is counted separately (e.g. if something is 2x in A and
// 5x in B, it will be 0x in extraA and 3x in extraB). The order of items in both lists is ignored.
// Adapted from https://github.com/stretchr/testify/blob/f97607b89807936ac4ff96748d766cf4b9711f78/assert/assertions.go#L1073C21-L1073C21
func DiffComparable[T comparable](listA, listB []T) ([]T, []T) {
	extraA, extraB := []T{}, []T{}

	aLen := len(listA)
	bLen := len(listB)

	// Mark indexes in listA that we already used
	visited := make([]bool, bLen)
	for i := 0; i < aLen; i++ {
		element := listA[i]
		found := false
		for j := 0; j < bLen; j++ {
			if visited[j] {
				continue
			}
			if listB[j] == element {
				visited[j] = true
				found = true
				break
			}
		}
		if !found {
			extraA = append(extraA, element)
		}
	}

	for j := 0; j < bLen; j++ {
		if visited[j] {
			continue
		}
		extraB = append(extraB, listB[j])
	}

	return extraA, extraB
}

// DiffEqualer diffs two arrays/slices where each element implements the Equaler interface and returns slices of
// elements that are only in A and only in B. If some element is present multiple times, each instance is counted
// separately (e.g. if something is 2x in A and 5x in B, it will be 0x in extraA and 3x in extraB). The order of
// items in both lists is ignored.
func DiffEqualer[T any, X Equaler[T]](listA []T, listB []X) ([]T, []X) {
	extraA, extraB := []T{}, []X{}

	aLen := len(listA)
	bLen := len(listB)

	// Mark indexes in listA that we already used
	visited := make([]bool, bLen)
	for i := 0; i < aLen; i++ {
		element := listA[i]
		found := false
		for j := 0; j < bLen; j++ {
			if visited[j] {
				continue
			}
			if listB[j].Equal(element) {
				visited[j] = true
				found = true
				break
			}
		}
		if !found {
			extraA = append(extraA, element)
		}
	}

	for j := 0; j < bLen; j++ {
		if visited[j] {
			continue
		}
		extraB = append(extraB, listB[j])
	}

	return extraA, extraB
}
