package code_interpreter

import (
	"io"
)

// ClosableBody is an http response body whose Close releases the request's
// context, so timeouts do not leak when a caller is done reading early.
type ClosableBody interface {
	io.ReadCloser
}

// cancelOnCloseBody releases a request context when the body is closed.
type cancelOnCloseBody struct {
	io.ReadCloser
	cancel func()
}

// Close closes the underlying body and releases the context.
func (b *cancelOnCloseBody) Close() error {
	err := b.ReadCloser.Close()
	b.cancel()
	return err
}
