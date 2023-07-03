package slices

func Copy[T any](src []T) []T {
	dst := make([]T, len(src))
	copy(dst, src)

	return dst
}
