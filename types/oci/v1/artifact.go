package v1

import (
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/oci"
)

// ArtifactSchemaVersion is a pre-configured versioned field for manifests
var ArtifactSchemaVersion = oci.Versioned{
	SchemaVersion: 2,
}

// ArtifactManifest defines an OCI Artifact
// This is EXPERIMENTAL
type ArtifactManifest struct {
	oci.Versioned

	// MediaType is the media type of the object this schema refers to.
	MediaType string `json:"mediaType"`

	// ArtifactType is the media type of the artifact this schema refers to.
	ArtifactType string `json:"artifactType"`

	// Blobs is a collection of blobs referenced by this manifest.
	Blobs []types.Descriptor `json:"blobs"`

	// Refers indicates this manifest references another manifest
	Refers *types.Descriptor `json:"refers,omitempty"`

	// Annotations contains arbitrary metadata for the artifact manifest.
	Annotations map[string]string `json:"annotations,omitempty"`
}
