//go:build !nolegacy
// +build !nolegacy

// Legacy package, this has been moved to the top level types/mediatype.go package

package regclient

import "github.com/regclient/regclient/types/mediatype"

var (
	// MediaTypeDocker1Manifest
	//
	// Deprecated: replace with [mediatype.Docker1Manifest].
	MediaTypeDocker1Manifest = mediatype.Docker1Manifest
	// MediaTypeDocker1ManifestSigned
	//
	// Deprecated: replace with [mediatype.Docker1ManifestSigned]
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
