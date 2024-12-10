package referrer

import (
	"errors"
	"strings"
	"testing"

	"github.com/opencontainers/go-digest"

	"github.com/regclient/regclient/types/descriptor"
	"github.com/regclient/regclient/types/errs"
	"github.com/regclient/regclient/types/manifest"
	"github.com/regclient/regclient/types/mediatype"
	v1 "github.com/regclient/regclient/types/oci/v1"
	"github.com/regclient/regclient/types/ref"
)

const bOCIImg = `
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
  "config": {
    "mediaType": "application/vnd.example.config.v1+json",
    "size": 2,
    "digest": "sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"
  },
  "layers": [
    {
      "mediaType": "application/vnd.example.data",
      "size": 12,
      "digest": "sha256:a948904f2f0f479b8f8197694b30184b0d2ed1c1cd2a1ec0fb85d299a192a447"
    }
	],
  "subject": {
    "mediaType": "application/vnd.oci.image.manifest.v1+json",
    "size": 1024,
    "digest": "sha256:81b44ad77a83506e00079bfb7df04240df39d8da45891018b2e5e00d5d69aff3"
  }
}
`

const bOCIImgAT = `
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
	"artifactType": "application/vnd.example.data",
  "config": {
    "mediaType": "application/vnd.oci.empty.v1+json",
    "size": 2,
    "digest": "sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"
  },
  "layers": [
    {
      "mediaType": "application/vnd.example.data",
      "size": 12,
      "digest": "sha256:a948904f2f0f479b8f8197694b30184b0d2ed1c1cd2a1ec0fb85d299a192a447"
    }
	],
	"annotations": {
		"com.example.instance": "test",
		"com.example.version": "1.0"
	},
  "subject": {
    "mediaType": "application/vnd.oci.image.manifest.v1+json",
    "size": 1024,
    "digest": "sha256:81b44ad77a83506e00079bfb7df04240df39d8da45891018b2e5e00d5d69aff3"
  }
}
`

const bOCIIndex = `
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.index.v1+json",
	"artifactType": "application/vnd.example.data",
  "manifests": [
    {
      "mediaType": "application/vnd.oci.image.manifest.v1+json",
      "size": 1024,
      "digest": "sha256:81b44ad77a83506e00079bfb7df04240df39d8da45891018b2e5e00d5d69aff3",
      "platform": {
        "architecture": "amd64",
        "os": "linux"
      }
    },
    {
      "mediaType": "application/vnd.oci.image.manifest.v1+json",
      "size": 1024,
      "digest": "sha256:82c1fa5ffa9c65d121c4e1386f30ac51d360f546814ab193adaf9ecf8c6fb0f2",
      "platform": {
        "architecture": "arm64",
        "os": "linux"
      }
    }
	],
	"annotations": {
		"com.example.instance": "test",
		"com.example.version": "1.0"
	},
  "subject": {
    "mediaType": "application/vnd.oci.image.manifest.v1+json",
    "size": 1024,
    "digest": "sha256:81b44ad77a83506e00079bfb7df04240df39d8da45891018b2e5e00d5d69aff3"
  }
}
`

const bDockerImg = `
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
  "config": {
    "mediaType": "application/vnd.docker.container.image.v1+json",
    "size": 1472,
    "digest": "sha256:b2aa39c304c27b96c1fef0c06bee651ac9241d49c4fe34381cab8453f9a89c7d"
  },
  "layers": [
    {
      "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
      "size": 3374446,
      "digest": "sha256:63b65145d645c1250c391b2d16ebe53b3747c295ca8ba2fcb6b0cf064a4dc21c"
    }
  ]
}
`

var mOCIImg, mOCIImgAT, mOCIIndex, mDockerImg manifest.Manifest
var dOCIImg = descriptor.Descriptor{
	MediaType:    "application/vnd.oci.image.manifest.v1+json",
	ArtifactType: "application/vnd.example.config.v1+json",
	Size:         int64(len(bOCIImg)),
	Digest:       digest.FromString(bOCIImg),
}
var dOCIImgAT = descriptor.Descriptor{
	MediaType:    "application/vnd.oci.image.manifest.v1+json",
	ArtifactType: "application/vnd.example.data",
	Size:         int64(len(bOCIImgAT)),
	Digest:       digest.FromString(bOCIImgAT),
	Annotations: map[string]string{
		"com.example.instance": "test",
		"com.example.version":  "1.0",
	},
}
var dOCIIndex = descriptor.Descriptor{
	MediaType:    "application/vnd.oci.image.index.v1+json",
	ArtifactType: "application/vnd.example.data",
	Size:         int64(len(bOCIIndex)),
	Digest:       digest.FromString(bOCIIndex),
	Annotations: map[string]string{
		"com.example.instance": "test",
		"com.example.version":  "1.0",
	},
}

func init() {
	var err error
	mOCIImg, err = manifest.New(manifest.WithRaw([]byte(bOCIImg)))
	if err != nil {
		panic(err)
	}
	mOCIImgAT, err = manifest.New(manifest.WithRaw([]byte(bOCIImgAT)))
	if err != nil {
		panic(err)
	}
	mOCIIndex, err = manifest.New(manifest.WithRaw([]byte(bOCIIndex)))
	if err != nil {
		panic(err)
	}
	mDockerImg, err = manifest.New(manifest.WithRaw([]byte(bDockerImg)))
	if err != nil {
		panic(err)
	}
}

func TestEmpty(t *testing.T) {
	t.Parallel()
	// create an empty list and full list, test is empty
	rlEmpty := &ReferrerList{
		Descriptors: []descriptor.Descriptor{},
		Annotations: map[string]string{},
		Tags:        []string{},
	}
	mEmpty, err := manifest.New(manifest.WithOrig(v1.Index{
		Versioned: v1.IndexSchemaVersion,
		MediaType: mediatype.OCI1ManifestList,
	}))
	if err != nil {
		t.Fatalf("failed to generate index: %v", err)
	}
	rlEmpty.Manifest = mEmpty
	if !rlEmpty.IsEmpty() {
		t.Errorf("empty list returns false on IsEmpty")
	}

	rlPopulated := &ReferrerList{
		Descriptors: []descriptor.Descriptor{
			dOCIImg,
			dOCIImgAT,
			dOCIIndex,
		},
		Annotations: map[string]string{},
		Tags:        []string{},
	}
	mPopulated, err := manifest.New(manifest.WithOrig(v1.Index{
		Versioned: v1.IndexSchemaVersion,
		MediaType: mediatype.OCI1ManifestList,
		Manifests: []descriptor.Descriptor{
			dOCIImg,
			dOCIImgAT,
			dOCIIndex,
		},
	}))
	if err != nil {
		t.Fatalf("failed to generate index: %v", err)
	}
	rlPopulated.Manifest = mPopulated
	if rlPopulated.IsEmpty() {
		t.Errorf("populated list returns true on IsEmpty")
	}
}

func TestAdd(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		m           manifest.Manifest
		expectedErr error
	}{
		{
			name: "OCI Image",
			m:    mOCIImg,
		},
		{
			name: "OCI Image artifact",
			m:    mOCIImgAT,
		},
		{
			name: "OCI Image again",
			m:    mOCIImg,
		},
		{
			name: "OCI Index",
			m:    mOCIIndex,
		},
		{
			name:        "Docker Image",
			m:           mDockerImg,
			expectedErr: errs.ErrUnsupportedMediaType,
		},
	}
	// add manifests (image without AT, image with AT, artifact, docker, no subject), verify list contents and error handling
	rl := &ReferrerList{
		Descriptors: []descriptor.Descriptor{},
		Annotations: map[string]string{},
		Tags:        []string{},
	}
	m, err := manifest.New(manifest.WithOrig(v1.Index{
		Versioned: v1.IndexSchemaVersion,
		MediaType: mediatype.OCI1ManifestList,
	}))
	if err != nil {
		t.Fatalf("failed to generate empty index: %v", err)
	}
	rl.Manifest = m
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rl.Add(tt.m)
			if tt.expectedErr != nil {
				if err == nil {
					t.Errorf("add succeeded, expected %v", tt.expectedErr)
				} else if !errors.Is(err, tt.expectedErr) && err.Error() != tt.expectedErr.Error() {
					t.Errorf("unexpected error, expected %v, received %v", tt.expectedErr, err)
				}
				return
			} else if err != nil {
				t.Errorf("add failed: %v", err)
				return
			}
		})
	}
	// 4 success - 1 dup
	if len(rl.Descriptors) != 3 {
		t.Errorf("number of descriptors, expected 3, received %d", len(rl.Descriptors))
	}
	for _, d := range rl.Descriptors {
		if d.ArtifactType == mediatype.OCI1Empty || d.ArtifactType == "" {
			t.Errorf("unexpected artifact type: %s", d.ArtifactType)
		}
	}
}

func TestDelete(t *testing.T) {
	t.Parallel()
	rl := &ReferrerList{
		Descriptors: []descriptor.Descriptor{
			dOCIImg,
			dOCIImgAT,
		},
		Annotations: map[string]string{},
		Tags:        []string{},
	}
	m, err := manifest.New(manifest.WithOrig(v1.Index{
		Versioned: v1.IndexSchemaVersion,
		MediaType: mediatype.OCI1ManifestList,
		Manifests: []descriptor.Descriptor{
			dOCIImg,
			dOCIImgAT,
			dOCIIndex,
		},
	}))
	if err != nil {
		t.Fatalf("failed to generate index: %v", err)
	}
	rl.Manifest = m

	tests := []struct {
		name        string
		m           manifest.Manifest
		expectedErr error
	}{
		{
			name: "OCI Image",
			m:    mOCIImg,
		},
		{
			name: "OCI Image artifact",
			m:    mOCIImgAT,
		},
		{
			name:        "OCI Image again",
			m:           mOCIImg,
			expectedErr: errs.ErrNotFound,
		},
		{
			name: "OCI Index",
			m:    mOCIIndex,
		},
		{
			name:        "Docker Image",
			m:           mDockerImg,
			expectedErr: errs.ErrNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rl.Delete(tt.m)
			if tt.expectedErr != nil {
				if err == nil {
					t.Errorf("delete succeeded, expected %v", tt.expectedErr)
				} else if !errors.Is(err, tt.expectedErr) && err.Error() != tt.expectedErr.Error() {
					t.Errorf("unexpected error, expected %v, received %v", tt.expectedErr, err)
				}
				return
			} else if err != nil {
				t.Errorf("delete failed: %v", err)
				return
			}
		})
	}
	if len(rl.Descriptors) != 0 {
		t.Errorf("number of descriptors, expected 0, received %d", len(rl.Descriptors))
	}
}

func TestMarshal(t *testing.T) {
	t.Parallel()
	rl := &ReferrerList{
		Descriptors: []descriptor.Descriptor{},
		Annotations: map[string]string{},
		Tags:        []string{},
	}
	outB, err := rl.MarshalPretty()
	if err != nil {
		t.Fatalf("failed to marshal empty referrer list: %v", err)
	}
	out := string(outB)
	if strings.Contains(out, "Subject:") {
		t.Errorf("empty response contains a subject line: %s", out)
	}
	if strings.Contains(out, "Source:") {
		t.Errorf("empty response contains a source line: %s", out)
	}
	if strings.Contains(out, "Annotations:") {
		t.Errorf("empty response contains an annotations line: %s", out)
	}

	rSubj, err := ref.New("registry.example.org/test/subject:latest")
	if err != nil {
		t.Fatalf("failed to parse subject ref: %v", err)
	}
	rSource, err := ref.New("registry.example.com/test/external")
	if err != nil {
		t.Fatalf("failed to parse source ref: %v", err)
	}
	rl = &ReferrerList{
		Subject: rSubj,
		Source:  rSource,
		Descriptors: []descriptor.Descriptor{
			dOCIImg,
			dOCIImgAT,
		},
		Annotations: map[string]string{
			"com.example.test": "test annotation",
		},
		Tags: []string{},
	}
	outB, err = rl.MarshalPretty()
	if err != nil {
		t.Fatalf("failed to marshal empty referrer list: %v", err)
	}
	out = string(outB)
	if !strings.Contains(out, "Subject:") {
		t.Errorf("empty response is missing a subject line: %s", out)
	}
	if !strings.Contains(out, "Source:") {
		t.Errorf("empty response is missing a source line: %s", out)
	}
	if !strings.Contains(out, "Annotations:") {
		t.Errorf("empty response is missing an annotations line: %s", out)
	}

}
