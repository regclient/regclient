//go:build !nolegacy
// +build !nolegacy

// Legacy package, this has been moved to top level types package

package types

import (
	topTypes "github.com/regclient/regclient/types"
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
