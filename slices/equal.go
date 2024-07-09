package slices

import (
	"errors"
	"fmt"
	"strings"
)

// EqualComparableElements returns whether two slices have the same elements.
func EqualComparableElements[T comparable](a, b []T) error {
	extraA, extraB := DiffComparable(a, b)

	if len(extraA) == 0 && len(extraB) == 0 {
		return nil
	}

	diff := []string{}

	for _, e := range extraA {
		diff = append(diff, fmt.Sprintf("+ %v", e))
	}

	for _, e := range extraB {
		diff = append(diff, fmt.Sprintf("- %v", e))
	}

	return errors.New(strings.Join(diff, ","))
}

// Equaler defines a type that implements the Equal function.
type Equaler[T any] interface {
	Equal(T) error
}

// EqualEqualerElements returns whether two slices of Equaler have the same elements.
func EqualEqualerElements[T any, X Equaler[T]](a []T, b []X) error {
	extraA, extraB := DiffEqualer(a, b)

	if len(extraA) == 0 && len(extraB) == 0 {
		return nil
	}

	diff := []string{}

	for _, e := range extraA {
		diff = append(diff, fmt.Sprintf("- %v", e))
	}

	for _, e := range extraB {
		diff = append(diff, fmt.Sprintf("+ %v", e))
	}

	return errors.New(strings.Join(diff, ","))
}
