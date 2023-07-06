package slices

func Copy[T any](src []T) []T {
	dst := make([]T, len(src))
	copy(dst, src)

	return dst
}

type Cloner[T any] interface {
	Clone() T
}

func CopyDeep[T any, X Cloner[T]](src []X) []T {
	dst := make([]T, len(src))

	for i, c := range src {
		dst[i] = c.Clone()
	}

	return dst
}
