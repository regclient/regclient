package schema2

import (
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/docker"
)

// ManifestListSchemaVersion is a pre-configured versioned field for manifest lists
var ManifestListSchemaVersion = docker.Versioned{
	SchemaVersion: 2,
	MediaType:     types.MediaTypeDocker2ManifestList,
}

// ManifestList references manifests for various platforms.
type ManifestList struct {
	docker.Versioned

	// Config references the image configuration as a blob.
	Manifests []types.Descriptor `json:"manifests"`
}
