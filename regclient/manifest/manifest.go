//go:build !nolegacy
// +build !nolegacy

// Package manifest is a legacy package, this has been moved to the types/manifest package
package manifest

import (
	"net/http"

	topTypes "github.com/regclient/regclient/types"
	topManifest "github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/ref"
)

const (
	MediaTypeDocker1Manifest       = topTypes.MediaTypeDocker1Manifest
	MediaTypeDocker1ManifestSigned = topTypes.MediaTypeDocker1ManifestSigned
	MediaTypeDocker2Manifest       = topTypes.MediaTypeDocker2Manifest
	MediaTypeDocker2ManifestList   = topTypes.MediaTypeDocker2ManifestList
	MediaTypeDocker2ImageConfig    = topTypes.MediaTypeDocker2ImageConfig
	MediaTypeOCI1Manifest          = topTypes.MediaTypeOCI1Manifest
	MediaTypeOCI1ManifestList      = topTypes.MediaTypeOCI1ManifestList
	MediaTypeOCI1ImageConfig       = topTypes.MediaTypeOCI1ImageConfig
	MediaTypeDocker2Layer          = topTypes.MediaTypeDocker2LayerGzip
	MediaTypeOCI1Layer             = topTypes.MediaTypeOCI1Layer
	MediaTypeOCI1LayerGzip         = topTypes.MediaTypeOCI1LayerGzip
	MediaTypeBuildkitCacheConfig   = topTypes.MediaTypeBuildkitCacheConfig
)

type Manifest = topManifest.Manifest

var (
	ErrNotFound             = topTypes.ErrNotFound
	ErrNotImplemented       = topTypes.ErrNotImplemented
	ErrUnavailable          = topTypes.ErrUnavailable
	ErrUnsupportedMediaType = topTypes.ErrUnsupported
)

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

func FromDescriptor(desc topTypes.Descriptor, mBytes []byte) (Manifest, error) {
	return topManifest.New(
		topManifest.WithDesc(desc),
		topManifest.WithRaw(mBytes),
	)
}

func FromOrig(orig interface{}) (Manifest, error) {
	return topManifest.New(topManifest.WithOrig(orig))
}
