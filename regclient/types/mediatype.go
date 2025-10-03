//go:build legacy
// +build legacy

// Legacy package, this has been moved to top level types package

package types

import (
	"github.com/regclient/regclient/types/mediatype"
)

const (
	// MediaTypeDocker1Manifest
	//
	// Deprecated: replaced by [mediatype.Docker1Manifest].
	//go:fix inline
	MediaTypeDocker1Manifest = mediatype.Docker1Manifest
	// MediaTypeDocker1ManifestSigned
	//
	// Deprecated: replaced by [mediatype.Docker1ManifestSigned]
	//go:fix inline
	MediaTypeDocker1ManifestSigned = mediatype.Docker1ManifestSigned
	// MediaTypeDocker2Manifest
	//
	// Deprecated: replaced by [mediatype.Docker2Manifest].
	//go:fix inline
	MediaTypeDocker2Manifest = mediatype.Docker2Manifest
	// MediaTypeDocker2ManifestList
	//
	// Deprecated: replaced by [mediatype.Docker2ManifestList].
	//go:fix inline
	MediaTypeDocker2ManifestList = mediatype.Docker2ManifestList
	// MediaTypeDocker2ImageConfig
	//
	// Deprecated: replaced by [mediatype.Docker2ImageConfig].
	//go:fix inline
	MediaTypeDocker2ImageConfig = mediatype.Docker2ImageConfig
	// MediaTypeOCI1Manifest
	//
	// Deprecated: replaced by [mediatype.OCI1Manifest].
	//go:fix inline
	MediaTypeOCI1Manifest = mediatype.OCI1Manifest
	// MediaTypeOCI1ManifestList
	//
	// Deprecated: replaced by [mediatype.OCI1ManifestList].
	//go:fix inline
	MediaTypeOCI1ManifestList = mediatype.OCI1ManifestList
	// MediaTypeOCI1ImageConfig
	//
	// Deprecated: replaced by [mediatype.OCI1ImageConfig].
	//go:fix inline
	MediaTypeOCI1ImageConfig = mediatype.OCI1ImageConfig
	// MediaTypeDocker2Layer
	//
	// Deprecated: replaced by [mediatype.Docker2Layer].
	//go:fix inline
	MediaTypeDocker2Layer = mediatype.Docker2LayerGzip
	// MediaTypeOCI1Layer
	//
	// Deprecated: replaced by [mediatype.OCI1Layer].
	//go:fix inline
	MediaTypeOCI1Layer = mediatype.OCI1Layer
	// MediaTypeOCI1LayerGzip
	//
	// Deprecated: replaced by [mediatype.OCI1LayerGzip].
	//go:fix inline
	MediaTypeOCI1LayerGzip = mediatype.OCI1LayerGzip
	// MediaTypeBuildkitCacheConfig
	//
	// Deprecated: replaced by [mediatype.BuildkitCacheConfig].
	//go:fix inline
	MediaTypeBuildkitCacheConfig = mediatype.BuildkitCacheConfig
)
