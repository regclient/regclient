// Package reqmeta provides metadata on requests for prioritizing with a pqueue.
package reqmeta

type Data struct {
	Kind Kind
	Size int64
}

type Kind int

const (
	Unknown Kind = iota
	Manifest
	Blob
	Query
)
