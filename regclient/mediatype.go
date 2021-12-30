//go:build !nolegacy
// +build !nolegacy

// Legacy package, this has been moved to the top level types/mediatype.go package

package regclient

import "github.com/regclient/regclient/types"

var (
	MediaTypeDocker1Manifest       = types.MediaTypeDocker1Manifest
	MediaTypeDocker1ManifestSigned = types.MediaTypeDocker1ManifestSigned
	MediaTypeDocker2Manifest       = types.MediaTypeDocker2Manifest
	MediaTypeDocker2ManifestList   = types.MediaTypeDocker2ManifestList
	MediaTypeDocker2ImageConfig    = types.MediaTypeDocker2ImageConfig
	MediaTypeOCI1Manifest          = types.MediaTypeOCI1Manifest
	MediaTypeOCI1ManifestList      = types.MediaTypeOCI1ManifestList
	MediaTypeOCI1ImageConfig       = types.MediaTypeOCI1ImageConfig
	MediaTypeDocker2Layer          = types.MediaTypeDocker2Layer
	MediaTypeOCI1Layer             = types.MediaTypeOCI1Layer
	MediaTypeOCI1LayerGzip         = types.MediaTypeOCI1LayerGzip
	MediaTypeBuildkitCacheConfig   = types.MediaTypeBuildkitCacheConfig
)
