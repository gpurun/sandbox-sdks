package core

import "context"

// Paginator incrementally retrieves pages of API resources.
type Paginator[T any] interface {
	Next(context.Context) ([]T, error)
	HasNext() bool
}
