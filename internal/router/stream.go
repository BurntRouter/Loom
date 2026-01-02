package router

import (
	"context"
	"io"
)

type Stream interface {
	io.Reader
	io.Writer
	io.Closer
	Context() context.Context
}
