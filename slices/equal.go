package slices

// EqualComparableElements returns whether two slices have the same elements.
func EqualComparableElements[T comparable](a, b []T) bool {
	extraA, extraB := DiffComparable(a, b)

	if len(extraA) == 0 && len(extraB) == 0 {
		return true
	}

	return false
}

// Equaler defines a type that implements the Equal function.
type Equaler[T any] interface {
	Equal(T) bool
}

// EqualEqualerElements returns whether two slices of Equaler have the same elements.
func EqualEqualerElements[T any, X Equaler[T]](a []T, b []X) bool {
	extraA, extraB := DiffEqualer(a, b)

	if len(extraA) == 0 && len(extraB) == 0 {
		return true
	}

	return false
}
