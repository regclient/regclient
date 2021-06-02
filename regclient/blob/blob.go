package blob

// Blob interface is used for returning blobs
type Blob interface {
	Common
	RawBody() ([]byte, error)
}
