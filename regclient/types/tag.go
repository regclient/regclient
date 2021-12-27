package types

import "net/http"

// TagList is the returned interface from a tag listing
type TagList interface {
	// GetOrig returns the parent object
	GetOrig() interface{}
	// MarshalJSON outputs the tag list as json
	MarshalJSON() ([]byte, error)
	// RawBody returns the unprocessed original data
	RawBody() ([]byte, error)
	// RawHeaders returns any http headers from remote requests
	RawHeaders() (http.Header, error)
	// GetTags returns an []string of tags extracted from the listing
	GetTags() ([]string, error)
}
