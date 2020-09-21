package archive

import "errors"

var (
	// ErrNotImplemented used for routines that need to be developed still
	ErrNotImplemented = errors.New("This archive routine is not implemented yet")
	// ErrXzUnsupported because there isn't a Go package for this and I'm
	// avoiding dependencies on external binaries
	ErrXzUnsupported = errors.New("Xz compression is currently unsupported")
)
