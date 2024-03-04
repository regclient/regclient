//go:build !nolegacy
// +build !nolegacy

// Package manifest is a legacy package, this has been moved to the types/manifest package
package manifest

import (
	"net/http"

	topTypes "github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/errs"
	topManifest "github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/mediatype"
	"github.com/regclient/regclient/types/ref"
)

const (
	// MediaTypeDocker1Manifest
	//
	// Deprecated: replace with [mediatype.Docker1Manifest].
	MediaTypeDocker1Manifest = mediatype.Docker1Manifest
	// MediaTypeDocker1ManifestSigned
	//
	// Deprecated: replace with [mediatype.Docker1ManifestSigned].
	MediaTypeDocker1ManifestSigned = mediatype.Docker1ManifestSigned
	// MediaTypeDocker2Manifest
	//
	// Deprecated: replace with [mediatype.Docker2Manifest].
	MediaTypeDocker2Manifest = mediatype.Docker2Manifest
	// MediaTypeDocker2ManifestList
	//
	// Deprecated: replace with [mediatype.Docker2ManifestList].
	MediaTypeDocker2ManifestList = mediatype.Docker2ManifestList
	// MediaTypeDocker2ImageConfig
	//
	// Deprecated: replace with [mediatype.Docker2ImageConfig].
	MediaTypeDocker2ImageConfig = mediatype.Docker2ImageConfig
	// MediaTypeOCI1Manifest
	//
	// Deprecated: replace with [mediatype.OCI1Manifest].
	MediaTypeOCI1Manifest = mediatype.OCI1Manifest
	// MediaTypeOCI1ManifestList
	//
	// Deprecated: replace with [mediatype.OCI1ManifestList].
	MediaTypeOCI1ManifestList = mediatype.OCI1ManifestList
	// MediaTypeOCI1ImageConfig
	//
	// Deprecated: replace with [mediatype.OCI1ImageConfig].
	MediaTypeOCI1ImageConfig = mediatype.OCI1ImageConfig
	// MediaTypeDocker2Layer
	//
	// Deprecated: replace with [mediatype.Docker2Layer].
	MediaTypeDocker2Layer = mediatype.Docker2LayerGzip
	// MediaTypeOCI1Layer
	//
	// Deprecated: replace with [mediatype.OCI1Layer].
	MediaTypeOCI1Layer = mediatype.OCI1Layer
	// MediaTypeOCI1LayerGzip
	//
	// Deprecated: replace with [mediatype.OCI1LayerGzip].
	MediaTypeOCI1LayerGzip = mediatype.OCI1LayerGzip
	// MediaTypeBuildkitCacheConfig
	//
	// Deprecated: replace with [mediatype.BuildkitCacheConfig].
	MediaTypeBuildkitCacheConfig = mediatype.BuildkitCacheConfig
)

type (
	// Manifest interface is implemented by all supported manifests but
	// many calls are only supported by certain underlying media types.
	//
	// Deprecated: replace with [manifest.Manifest].
	Manifest = topManifest.Manifest
)

var (
	// ErrNotFound
	//
	// Deprecated: replace with [errs.ErrNotFound].
	ErrNotFound = errs.ErrNotFound
	// ErrNotImplemented
	//
	// Deprecated: replace with [errs.ErrNotImplemented].
	ErrNotImplemented = errs.ErrNotImplemented
	// ErrUnavailable
	//
	// Deprecated: replace with [errs.ErrUnavailable].
	ErrUnavailable = errs.ErrUnavailable
	// ErrUnsupportedMediaType
	//
	// Deprecated: replace with [errs.ErrUnsupportedMediaType].
	ErrUnsupportedMediaType = errs.ErrUnsupported
)

// New creates a new manifest.
//
// Deprecated: replace with [manifest.New].
func New(mediaType string, raw []byte, r ref.Ref, header http.Header) (Manifest, error) {
	return topManifest.New(
		topManifest.WithDesc(topTypes.Descriptor{
			MediaType: mediaType,
		}),
		topManifest.WithRef(r),
		topManifest.WithRaw(raw),
		topManifest.WithHeader(header),
	)
}

// FromDescriptor creates a manifest from a descriptor.
//
// Deprecated: replace with [manifest.New].
func FromDescriptor(desc topTypes.Descriptor, mBytes []byte) (Manifest, error) {
	return topManifest.New(
		topManifest.WithDesc(desc),
		topManifest.WithRaw(mBytes),
	)
}

// FromOrig creates a manifest from an underlying manifest struct.
//
// Deprecated: replace with [manifest.New].
func FromOrig(orig interface{}) (Manifest, error) {
	return topManifest.New(topManifest.WithOrig(orig))
}
