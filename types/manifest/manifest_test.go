package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/docker/schema1"
	"github.com/regclient/regclient/types/docker/schema2"
	v1 "github.com/regclient/regclient/types/oci/v1"
	"github.com/regclient/regclient/types/platform"
	"github.com/regclient/regclient/types/ref"
)

var (
	rawDockerSchema2 = []byte(`
		{
			"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
			"schemaVersion": 2,
			"config": {
				"mediaType": "application/vnd.docker.container.image.v1+json",
				"digest": "sha256:10fdcbb8eac53c686023468e307adb6c0da03fc904f6739ee543143a2365be41",
				"size": 3023
			},
			"layers": [
				{
						"mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
						"digest": "sha256:f6e2d7fa40092cf3d9817bf6ff54183d68d108a47fdf5a5e476c612626c80e14",
						"size": 941
				},
				{
						"mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
						"digest": "sha256:92365f35877078c3e558e9a66ac083fe9a8d44bdb3150bdac058380054b05972",
						"size": 122412
				},
				{
						"mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
						"digest": "sha256:fa98de7a23a1c3debba4398c982decfd8b31bcfad1ac6e5e7d800375cefbd42f",
						"size": 146
				},
				{
						"mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
						"digest": "sha256:9767ed5c27ebed39ff76afe979043e52dc7714c78d1dda8a8581965e06be2535",
						"size": 3535944
				}
			],
			"annotations": {
				"org.example.test": "hello world"
			}
		}
	`)
	rawDockerSchema2List = []byte(`
	{
		"mediaType": "application/vnd.docker.distribution.manifest.list.v2+json",
		"schemaVersion": 2,
		"manifests": [
			{
				"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
				"digest": "sha256:69168abe0494a1f1e619725d23a8f85cb156a8986f342c7dc86915b551f5a711",
				"size": 1152,
				"platform": {
					"architecture": "386",
					"os": "linux"
				}
			},
			{
				"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
				"digest": "sha256:41b9947d8f19e154a5415c88ef71b851d37fa3ceb1de56ffe88d1b616ce503d9",
				"size": 1152,
				"platform": {
					"architecture": "amd64",
					"os": "linux"
				}
			},
			{
				"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
				"digest": "sha256:e8baa0ddeed304ed91e91f155392462fcfab79df67f1052f92a377305dd521b6",
				"size": 1152,
				"platform": {
					"architecture": "arm",
					"os": "linux",
					"variant": "v6"
				}
			},
			{
				"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
				"digest": "sha256:5536e52b2508b905c7f37bf120435c3c75684bab53c04467b61904be1febe5f8",
				"size": 1152,
				"platform": {
					"architecture": "arm",
					"os": "linux",
					"variant": "v7"
				}
			},
			{
				"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
				"digest": "sha256:b302f648065bb2ba542dc75167db065781f296ef72bb504585d652b27b5079ad",
				"size": 1152,
				"platform": {
					"architecture": "arm64",
					"os": "linux"
				}
			},
			{
				"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
				"digest": "sha256:2d6a26eeb5a58c3c2534470f201b471778cc2ed37352775c9632e60880339e24",
				"size": 1152,
				"platform": {
					"architecture": "ppc64le",
					"os": "linux"
				}
			},
			{
				"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
				"digest": "sha256:201dd5b2dcc8793566b3d2cfa4d32eb3963028d20cc7befb3260de6d7ceac8a4",
				"size": 1152,
				"platform": {
					"architecture": "s390x",
					"os": "linux"
				}
			}
			],
			"annotations": {
				"org.example.test": "hello world"
			}
		}
	`)
	rawAmbiguousOCI = []byte(`
		{
			"schemaVersion": 2,
			"config": {
				"mediaType": "application/vnd.oci.image.config.v1+json",
				"size": 733,
				"digest": "sha256:35481f6488745b7eb5748f759b939deb063f458e9c3f9f998abc423e6652ece5"
			},
			"layers": [
				{
					"mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
					"size": 657696,
					"digest": "sha256:b49b96595fd4bd6de7cb7253fe5e89d242d0eb4f993b2b8280c0581c3a62ddc2"
				},
				{
					"mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
					"size": 127,
					"digest": "sha256:250c06f7c38e52dc77e5c7586c3e40280dc7ff9bb9007c396e06d96736cf8542"
				},
				{
					"mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
					"size": 1136676,
					"digest": "sha256:c6690738d95e2b3d3c9ddfd34aa88ddce6e8d6e31c826989b869c25f8888f158"
				}
			],
			"manifests": [
				{
					"mediaType": "application/vnd.oci.image.manifest.v1+json",
					"size": 659,
					"digest": "sha256:bdde23183a221cc31fb66df0d93b834b11f2a0c2e8a03e6304c5e17d3cd5038f",
					"platform": {
						"architecture": "amd64",
						"os": "linux"
					}
				}
			],
			"annotations": {
				"org.example.test": "hello world"
			}
		}
	`)
	rawOCIImage = []byte(`
		{
			"schemaVersion": 2,
			"mediaType": "application/vnd.oci.image.manifest.v1+json",
			"config": {
				"mediaType": "application/vnd.oci.image.config.v1+json",
				"size": 733,
				"digest": "sha256:35481f6488745b7eb5748f759b939deb063f458e9c3f9f998abc423e6652ece5"
			},
			"layers": [
				{
					"mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
					"size": 657696,
					"digest": "sha256:b49b96595fd4bd6de7cb7253fe5e89d242d0eb4f993b2b8280c0581c3a62ddc2"
				},
				{
					"mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
					"size": 127,
					"digest": "sha256:250c06f7c38e52dc77e5c7586c3e40280dc7ff9bb9007c396e06d96736cf8542"
				},
				{
					"mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
					"size": 1136676,
					"digest": "sha256:c6690738d95e2b3d3c9ddfd34aa88ddce6e8d6e31c826989b869c25f8888f158"
				}
			],
			"manifests": [
				{
					"mediaType": "application/vnd.oci.image.manifest.v1+json",
					"size": 659,
					"digest": "sha256:bdde23183a221cc31fb66df0d93b834b11f2a0c2e8a03e6304c5e17d3cd5038f",
					"platform": {
						"architecture": "amd64",
						"os": "linux"
					}
				}
			],
			"annotations": {
				"org.example.test": "hello world"
			}
		}
	`)
	rawOCIIndex = []byte(`
		{
			"schemaVersion": 2,
			"mediaType": "application/vnd.oci.image.index.v1+json",
			"config": {
				"mediaType": "application/vnd.oci.image.config.v1+json",
				"size": 733,
				"digest": "sha256:35481f6488745b7eb5748f759b939deb063f458e9c3f9f998abc423e6652ece5"
			},
			"layers": [
				{
					"mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
					"size": 657696,
					"digest": "sha256:b49b96595fd4bd6de7cb7253fe5e89d242d0eb4f993b2b8280c0581c3a62ddc2"
				},
				{
					"mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
					"size": 127,
					"digest": "sha256:250c06f7c38e52dc77e5c7586c3e40280dc7ff9bb9007c396e06d96736cf8542"
				},
				{
					"mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
					"size": 1136676,
					"digest": "sha256:c6690738d95e2b3d3c9ddfd34aa88ddce6e8d6e31c826989b869c25f8888f158"
				}
			],
			"manifests": [
				{
					"mediaType": "application/vnd.oci.image.manifest.v1+json",
					"size": 659,
					"digest": "sha256:bdde23183a221cc31fb66df0d93b834b11f2a0c2e8a03e6304c5e17d3cd5038f",
					"platform": {
						"architecture": "amd64",
						"os": "linux"
					}
				}
			],
			"annotations": {
				"org.example.test": "hello world"
			}
		}
	`)
	rawOCI1Artifact = []byte(`
		{
			"schemaVersion": 2,
			"mediaType": "application/vnd.oci.artifact.manifest.v1+json",
			"blobs": [
				{
					"mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
					"size": 657696,
					"digest": "sha256:b49b96595fd4bd6de7cb7253fe5e89d242d0eb4f993b2b8280c0581c3a62ddc2"
				},
				{
					"mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
					"size": 127,
					"digest": "sha256:250c06f7c38e52dc77e5c7586c3e40280dc7ff9bb9007c396e06d96736cf8542"
				}
			],
			"annotations": {
				"org.example.test": "hello world"
			}
		}
	`)
	// signed schemas are white space sensitive, contents here must be indented with 3 spaces, no tabs
	rawDockerSchema1Signed = []byte(`
{
   "schemaVersion": 1,
   "name": "library/debian",
   "tag": "6",
   "architecture": "amd64",
   "fsLayers": [
      {
         "blobSum": "sha256:a3ed95caeb02ffe68cdd9fd84406680ae93d633cb16422d00e8a7c22955b46d4"
      },
      {
         "blobSum": "sha256:069873d23334d65630bbe5e303ced0c68181b694c7f5506b54bf5d8115b5af20"
      }
   ],
   "history": [
      {
         "v1Compatibility": "{\"id\":\"ff11dd0897b8ded12196819a787b5bd6d5bf886d9a7836c21b070efb5d9e77e4\",\"parent\":\"4e507d091336a8ec91e1b0fd0e33f11625d8bf3494765d3dbec37ec17387cbf5\",\"created\":\"2016-02-16T21:25:24.035599122Z\",\"container\":\"0fd99658f7a77c1170f8ff325c14437eaced7bab6b3152264cb1946d8d018e2e\",\"container_config\":{\"Hostname\":\"71f62d8ce24c\",\"Domainname\":\"\",\"User\":\"\",\"AttachStdin\":false,\"AttachStdout\":false,\"AttachStderr\":false,\"Tty\":false,\"OpenStdin\":false,\"StdinOnce\":false,\"Env\":null,\"Cmd\":[\"/bin/sh\",\"-c\",\"#(nop) CMD [\\\"/bin/bash\\\"]\"],\"Image\":\"4e507d091336a8ec91e1b0fd0e33f11625d8bf3494765d3dbec37ec17387cbf5\",\"Volumes\":null,\"WorkingDir\":\"\",\"Entrypoint\":null,\"OnBuild\":null,\"Labels\":{}},\"docker_version\":\"1.9.1\",\"config\":{\"Hostname\":\"71f62d8ce24c\",\"Domainname\":\"\",\"User\":\"\",\"AttachStdin\":false,\"AttachStdout\":false,\"AttachStderr\":false,\"Tty\":false,\"OpenStdin\":false,\"StdinOnce\":false,\"Env\":null,\"Cmd\":[\"/bin/bash\"],\"Image\":\"4e507d091336a8ec91e1b0fd0e33f11625d8bf3494765d3dbec37ec17387cbf5\",\"Volumes\":null,\"WorkingDir\":\"\",\"Entrypoint\":null,\"OnBuild\":null,\"Labels\":{}},\"architecture\":\"amd64\",\"os\":\"linux\"}"
      },
      {
         "v1Compatibility": "{\"id\":\"4e507d091336a8ec91e1b0fd0e33f11625d8bf3494765d3dbec37ec17387cbf5\",\"created\":\"2016-02-16T21:25:21.747984969Z\",\"container\":\"71f62d8ce24cd81b2835a2a4457e9e745f775a225cb2e75a5e76fc8b5f44874c\",\"container_config\":{\"Hostname\":\"71f62d8ce24c\",\"Domainname\":\"\",\"User\":\"\",\"AttachStdin\":false,\"AttachStdout\":false,\"AttachStderr\":false,\"Tty\":false,\"OpenStdin\":false,\"StdinOnce\":false,\"Env\":null,\"Cmd\":[\"/bin/sh\",\"-c\",\"#(nop) ADD file:09d717d62608e18d79af6b6cd5aae36f675bd5c4f34452ab1693b56bfbfe2520 in /\"],\"Image\":\"\",\"Volumes\":null,\"WorkingDir\":\"\",\"Entrypoint\":null,\"OnBuild\":null,\"Labels\":null},\"docker_version\":\"1.9.1\",\"config\":{\"Hostname\":\"71f62d8ce24c\",\"Domainname\":\"\",\"User\":\"\",\"AttachStdin\":false,\"AttachStdout\":false,\"AttachStderr\":false,\"Tty\":false,\"OpenStdin\":false,\"StdinOnce\":false,\"Env\":null,\"Cmd\":null,\"Image\":\"\",\"Volumes\":null,\"WorkingDir\":\"\",\"Entrypoint\":null,\"OnBuild\":null,\"Labels\":null},\"architecture\":\"amd64\",\"os\":\"linux\",\"Size\":76534288}"
      }
   ],
   "signatures": [
      {
         "header": {
            "jwk": {
               "crv": "P-256",
               "kid": "FD6K:7VOX:ZVOM:34T7:2ZT5:753N:ZM4C:RJIF:WPOO:NPC2:7VPJ:3TVM",
               "kty": "EC",
               "x": "kHg6ZEbadXH4gC5ggkduHEAeJP40vdudo7tekiigA00",
               "y": "K5r269kJQV1ERenXMuEQbY7_hrbxy1JnTnSOBR0bvTg"
            },
            "alg": "ES256"
         },
         "signature": "mtuG3ORjrX8o7lqyx78tX_JIX-JuiBAWX2sEvf60t4zXzLB61gNecwasp56Mn3LT7fxmJzC3-IcHW-UryDm6uw",
         "protected": "eyJmb3JtYXRMZW5ndGgiOjI3NDYsImZvcm1hdFRhaWwiOiJDbjAiLCJ0aW1lIjoiMjAyMS0xMi0xM1QxMzo0OTozNFoifQ"
      }
   ]
} 
`)
)

var (
	digestDockerSchema2          = digest.FromBytes(rawDockerSchema2)
	digestDockerSchema2List      = digest.FromBytes(rawDockerSchema2List)
	digestInvalid                = digest.FromString("invalid")
	digestDockerSchema1Signed, _ = digest.Parse("sha256:f3ef067962554c3352dc0c659ca563f73cc396fe0dea2a2c23a7964c6290f782")
	digestOCIImage               = digest.FromBytes(rawOCIImage)
	digestOCIIndex               = digest.FromBytes(rawOCIIndex)
	digestOCIArtifact            = digest.FromBytes(rawOCI1Artifact)
)

func TestNew(t *testing.T) {
	r, _ := ref.New("localhost:5000/test:latest")
	var manifestDockerSchema2, manifestInvalid schema2.Manifest
	var manifestDockerSchema1Signed schema1.SignedManifest
	var manifestOCIArtifact v1.ArtifactManifest
	err := json.Unmarshal(rawDockerSchema2, &manifestDockerSchema2)
	if err != nil {
		t.Fatalf("failed to unmarshal docker schema2 json: %v", err)
	}
	err = json.Unmarshal(rawDockerSchema2, &manifestInvalid)
	if err != nil {
		t.Fatalf("failed to unmarshal docker schema2 json: %v", err)
	}
	err = json.Unmarshal(rawOCI1Artifact, &manifestOCIArtifact)
	if err != nil {
		t.Fatalf("failed to unmarshal OCI Artifact json: %v", err)
	}
	manifestInvalid.MediaType = types.MediaTypeOCI1Manifest
	err = json.Unmarshal(rawDockerSchema1Signed, &manifestDockerSchema1Signed)
	if err != nil {
		t.Fatalf("failed to unmarshal docker schema1 signed json: %v", err)
	}
	var tests = []struct {
		name        string
		opts        []Opts
		wantR       ref.Ref
		wantDesc    types.Descriptor
		wantE       error
		isSet       bool
		testAnnot   bool
		hasAnnot    bool
		testPlat    string
		wantPlat    types.Descriptor
		testSubject bool
		hasSubject  bool
	}{
		{
			name:  "empty",
			wantE: fmt.Errorf("%w: \"%s\"", types.ErrUnsupportedMediaType, ""),
		},
		{
			name: "Docker Schema 2 Manifest",
			opts: []Opts{
				WithRef(r),
				WithRaw(rawDockerSchema2),
			},
			wantR: r,
			wantDesc: types.Descriptor{
				MediaType: types.MediaTypeDocker2Manifest,
				Size:      int64(len(rawDockerSchema2)),
				Digest:    digestDockerSchema2,
			},
			wantE:       nil,
			isSet:       true,
			testAnnot:   true,
			testSubject: true,
			hasAnnot:    true,
		},
		{
			name: "Docker Schema 2 Manifest full desc",
			opts: []Opts{
				WithDesc(types.Descriptor{
					MediaType: types.MediaTypeDocker2Manifest,
					Digest:    digestDockerSchema2,
					Size:      int64(len(rawDockerSchema2)),
				}),
				WithRaw(rawDockerSchema2),
			},
			wantDesc: types.Descriptor{
				MediaType: types.MediaTypeDocker2Manifest,
				Size:      int64(len(rawDockerSchema2)),
				Digest:    digestDockerSchema2,
			},
			testAnnot:   true,
			testSubject: true,
			wantE:       nil,
			isSet:       true,
			hasAnnot:    true,
		},
		{
			name: "Docker Schema 2 List from Http",
			opts: []Opts{
				WithRef(r),
				WithRaw(rawDockerSchema2List),
				WithHeader(http.Header{
					"Content-Type":          []string{MediaTypeDocker2ManifestList},
					"Docker-Content-Digest": []string{digestDockerSchema2List.String()},
				}),
			},
			wantE:       nil,
			isSet:       true,
			testAnnot:   true,
			testSubject: true,
			hasAnnot:    true,
			testPlat:    "linux/amd64",
			wantPlat: types.Descriptor{
				MediaType: "application/vnd.docker.distribution.manifest.v2+json",
				Digest:    "sha256:41b9947d8f19e154a5415c88ef71b851d37fa3ceb1de56ffe88d1b616ce503d9",
				Size:      1152,
			},
		},
		{
			name: "Docker Schema 2 List get Darwin",
			opts: []Opts{
				WithRef(r),
				WithRaw(rawDockerSchema2List),
			},
			wantE:       nil,
			isSet:       true,
			testAnnot:   true,
			testSubject: true,
			hasAnnot:    true,
			testPlat:    "darwin/arm64",
			wantPlat: types.Descriptor{
				MediaType: "application/vnd.docker.distribution.manifest.v2+json",
				Digest:    "sha256:b302f648065bb2ba542dc75167db065781f296ef72bb504585d652b27b5079ad",
				Size:      1152,
			},
		},
		{
			name: "OCI Artifact from Http",
			opts: []Opts{
				WithRef(r),
				WithRaw(rawOCI1Artifact),
				WithHeader(http.Header{
					"Content-Type":          []string{types.MediaTypeOCI1Artifact},
					"Docker-Content-Digest": []string{digestOCIArtifact.String()},
				}),
			},
			wantE:       nil,
			isSet:       true,
			testAnnot:   true,
			testSubject: true,
			hasAnnot:    true,
			hasSubject:  true,
		},
		{
			name: "Header Request Docker 2 Manifest",
			opts: []Opts{
				WithRef(r),
				WithHeader(http.Header{
					"Content-Type":          []string{types.MediaTypeDocker2Manifest},
					"Content-Length":        []string{fmt.Sprintf("%d", len(rawDockerSchema2))},
					"Docker-Content-Digest": []string{digestDockerSchema2.String()},
				}),
			},
			wantDesc: types.Descriptor{
				MediaType: types.MediaTypeDocker2Manifest,
				Size:      int64(len(rawDockerSchema2)),
				Digest:    digestDockerSchema2,
			},
			wantE: nil,
		},
		{
			name: "Header Request Docker 1 Manifest",
			opts: []Opts{
				WithRef(r),
				WithHeader(http.Header{
					"Content-Type":          []string{types.MediaTypeDocker1Manifest},
					"Content-Length":        []string{fmt.Sprintf("%d", len(rawDockerSchema2))},
					"Docker-Content-Digest": []string{digestDockerSchema2.String()},
				}),
			},
			wantDesc: types.Descriptor{
				MediaType: types.MediaTypeDocker1Manifest,
				Size:      int64(len(rawDockerSchema2)),
				Digest:    digestDockerSchema2,
			},
			wantE: nil,
		},
		{
			name: "Header Request Docker 1 Manifest Signed",
			opts: []Opts{
				WithRef(r),
				WithHeader(http.Header{
					"Content-Type":          []string{types.MediaTypeDocker1ManifestSigned},
					"Content-Length":        []string{fmt.Sprintf("%d", len(rawDockerSchema2))},
					"Docker-Content-Digest": []string{digestDockerSchema2.String()},
				}),
			},
			wantDesc: types.Descriptor{
				MediaType: types.MediaTypeDocker1ManifestSigned,
				Size:      int64(len(rawDockerSchema2)),
				Digest:    digestDockerSchema2,
			},
			wantE: nil,
		},
		{
			name: "Header Request Docker 2 Manifest",
			opts: []Opts{
				WithRef(r),
				WithHeader(http.Header{
					"Content-Type":          []string{types.MediaTypeDocker2Manifest},
					"Content-Length":        []string{fmt.Sprintf("%d", len(rawDockerSchema2))},
					"Docker-Content-Digest": []string{digestDockerSchema2.String()},
				}),
			},
			wantDesc: types.Descriptor{
				MediaType: types.MediaTypeDocker2Manifest,
				Size:      int64(len(rawDockerSchema2)),
				Digest:    digestDockerSchema2,
			},
			wantE: nil,
		},
		{
			name: "Header Request Docker 2 Manifest List",
			opts: []Opts{
				WithRef(r),
				WithHeader(http.Header{
					"Content-Type":          []string{types.MediaTypeDocker2ManifestList},
					"Content-Length":        []string{fmt.Sprintf("%d", len(rawDockerSchema2))},
					"Docker-Content-Digest": []string{digestDockerSchema2.String()},
				}),
			},
			wantDesc: types.Descriptor{
				MediaType: types.MediaTypeDocker2ManifestList,
				Size:      int64(len(rawDockerSchema2)),
				Digest:    digestDockerSchema2,
			},
			wantE: nil,
		},
		{
			name: "Header Request OCI Manifest",
			opts: []Opts{
				WithRef(r),
				WithHeader(http.Header{
					"Content-Type":          []string{types.MediaTypeOCI1Manifest},
					"Content-Length":        []string{fmt.Sprintf("%d", len(rawDockerSchema2))},
					"Docker-Content-Digest": []string{digestDockerSchema2.String()},
				}),
			},
			wantDesc: types.Descriptor{
				MediaType: types.MediaTypeOCI1Manifest,
				Size:      int64(len(rawDockerSchema2)),
				Digest:    digestDockerSchema2,
			},
			wantE: nil,
		},
		{
			name: "Header Request OCI Manifest List",
			opts: []Opts{
				WithRef(r),
				WithHeader(http.Header{
					"Content-Type":          []string{types.MediaTypeOCI1ManifestList},
					"Content-Length":        []string{fmt.Sprintf("%d", len(rawDockerSchema2))},
					"Docker-Content-Digest": []string{digestDockerSchema2.String()},
				}),
			},
			wantDesc: types.Descriptor{
				MediaType: types.MediaTypeOCI1ManifestList,
				Size:      int64(len(rawDockerSchema2)),
				Digest:    digestDockerSchema2,
			},
			wantE: nil,
		},
		{
			name: "Header Request OCI Artifact",
			opts: []Opts{
				WithRef(r),
				WithHeader(http.Header{
					"Content-Type":          []string{types.MediaTypeOCI1Artifact},
					"Content-Length":        []string{fmt.Sprintf("%d", len(rawDockerSchema2))},
					"Docker-Content-Digest": []string{digestDockerSchema2.String()},
				}),
			},
			wantDesc: types.Descriptor{
				MediaType: types.MediaTypeOCI1Artifact,
				Size:      int64(len(rawDockerSchema2)),
				Digest:    digestDockerSchema2,
			},
			wantE: nil,
		},
		{
			name: "Docker Schema 1 Signed",
			opts: []Opts{
				WithRef(r),
				WithRaw(rawDockerSchema1Signed),
			},
			wantE:       nil,
			isSet:       true,
			testAnnot:   true,
			testSubject: true,
		},
		{
			name: "Docker Schema 1 Signed Manifest",
			opts: []Opts{
				WithRaw(rawDockerSchema1Signed),
				WithDesc(types.Descriptor{
					MediaType: types.MediaTypeDocker1ManifestSigned,
					Digest:    digestDockerSchema1Signed,
					Size:      int64(len(rawDockerSchema1Signed)),
				}),
			},
			wantE: nil,
			isSet: true,
		},
		{
			name: "Invalid Http Digest",
			opts: []Opts{
				WithRef(r),
				WithRaw(rawDockerSchema2List),
				WithHeader(http.Header{
					"Content-Type":          []string{MediaTypeDocker2ManifestList},
					"Docker-Content-Digest": []string{digestInvalid.String()},
				}),
			},
			wantE: fmt.Errorf("manifest digest mismatch, expected %s, computed %s", digestInvalid, digestDockerSchema2List),
		},
		{
			name: "Ambiguous OCI Image",
			opts: []Opts{
				WithRef(r),
				WithRaw(rawAmbiguousOCI),
				WithDesc(types.Descriptor{
					MediaType: types.MediaTypeOCI1Manifest,
				}),
			},
			wantE: nil,
			isSet: true,
		},
		{
			name: "Ambiguous OCI Index",
			opts: []Opts{
				WithRef(r),
				WithRaw(rawAmbiguousOCI),
				WithDesc(types.Descriptor{
					MediaType: types.MediaTypeOCI1ManifestList,
				}),
			},
			wantE: nil,
			isSet: true,
		},
		{
			name: "Invalid OCI Index",
			opts: []Opts{
				WithRef(r),
				WithRaw(rawOCIImage),
				WithDesc(types.Descriptor{
					MediaType: types.MediaTypeOCI1ManifestList,
				}),
			},
			wantE: fmt.Errorf("manifest contains an unexpected media type: expected %s, received %s", types.MediaTypeOCI1ManifestList, types.MediaTypeOCI1Manifest),
		},
		{
			name: "Invalid OCI Image",
			opts: []Opts{
				WithRef(r),
				WithRaw(rawOCIIndex),
				WithDesc(types.Descriptor{
					MediaType: types.MediaTypeOCI1Manifest,
				}),
			},
			wantE: fmt.Errorf("manifest contains an unexpected media type: expected %s, received %s", types.MediaTypeOCI1Manifest, types.MediaTypeOCI1ManifestList),
		},
		{
			name: "Invalid digest",
			opts: []Opts{
				WithRef(r),
				WithRaw(rawDockerSchema2),
				WithDesc(types.Descriptor{
					MediaType: types.MediaTypeDocker2Manifest,
					Digest:    digestInvalid,
					Size:      int64(len(rawDockerSchema2)),
				}),
			},
			wantE: fmt.Errorf("manifest digest mismatch, expected %s, computed %s", digestInvalid, digestDockerSchema2),
		},
		{
			name: "Invalid Media Type",
			opts: []Opts{
				WithRef(r),
				WithRaw(rawOCIImage),
				WithDesc(types.Descriptor{
					MediaType: types.MediaTypeOCI1ManifestList,
					Digest:    digestOCIImage,
					Size:      int64(len(rawOCIImage)),
				}),
			},
			wantE: fmt.Errorf("manifest contains an unexpected media type: expected %s, received %s", types.MediaTypeOCI1ManifestList, types.MediaTypeOCI1Manifest),
		},
		{
			name: "Docker Schema2 Orig",
			opts: []Opts{
				WithOrig(manifestDockerSchema2),
			},
			wantE:       nil,
			isSet:       true,
			testAnnot:   true,
			testSubject: true,
			hasAnnot:    true,
		},
		{
			name: "OCI Artifact Orig",
			opts: []Opts{
				WithOrig(manifestOCIArtifact),
			},
			wantE:       nil,
			isSet:       true,
			testAnnot:   true,
			testSubject: true,
			hasAnnot:    true,
			hasSubject:  true,
		},
		{
			name: "Docker Schema1 Signed Orig",
			opts: []Opts{
				WithOrig(manifestDockerSchema1Signed),
			},
			wantE: nil,
			isSet: true,
		},
		{
			name: "Invalid Media Type",
			opts: []Opts{
				WithOrig(manifestInvalid),
			},
			wantE: fmt.Errorf("manifest contains an unexpected media type: expected %s, received %s", types.MediaTypeDocker2Manifest, types.MediaTypeOCI1Manifest),
		},

		// TODO: add more tests to improve coverage
		// - test rate limit
		// - test retrieving descriptor lists from manifest lists
		// - test if manifest is set
		// - test raw body
	}
	subDesc := types.Descriptor{
		MediaType: types.MediaTypeOCI1Manifest,
		Size:      1234,
		Digest:    digest.FromString("test referrer"),
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := New(tt.opts...)
			if tt.wantE != nil {
				if err == nil {
					t.Errorf("did not receive expected error %v", tt.wantE)
				} else if !errors.Is(err, tt.wantE) && err.Error() != tt.wantE.Error() {
					t.Errorf("expected error not received, expected %v, received %v", tt.wantE, err)
				}
				return
			}
			if err != nil {
				t.Errorf("failed running New: %v", err)
				return
			}
			// MarshalPretty succeeds even if manifest is not set (it shows available metadata)
			if mp, ok := m.(interface{ MarshalPretty() ([]byte, error) }); ok {
				pretty, err := mp.MarshalPretty()
				if err != nil {
					t.Errorf("failed to MarshalPretty: %v", err)
				} else {
					t.Logf("marshal pretty:\n%s", string(pretty))
				}
			} else {
				t.Errorf("MarshalPretty not available")
			}
			if tt.wantR.Scheme != "" && m.GetRef().CommonName() != tt.wantR.CommonName() {
				t.Errorf("ref mismatch, expected %s, received %s", tt.wantR.CommonName(), m.GetRef().CommonName())
			}
			if tt.wantDesc.Digest != "" && GetDigest(m) != tt.wantDesc.Digest {
				t.Errorf("digest mismatch, expected %s, received %s", tt.wantDesc.Digest, GetDigest(m))
			}
			if tt.wantDesc.MediaType != "" && GetMediaType(m) != tt.wantDesc.MediaType {
				t.Errorf("media type mismatch, expected %s, received %s", tt.wantDesc.MediaType, GetMediaType(m))
			}
			if !tt.isSet {
				// test methods on unset manifest
				if m.IsSet() {
					t.Errorf("manifest reports it is set")
				}
				if _, err := m.RawBody(); !errors.Is(err, types.ErrManifestNotSet) && !errors.Is(err, types.ErrUnsupportedMediaType) {
					t.Errorf("RawBody did not return ManifestNotSet: %v", err)
				}
				if _, err := m.MarshalJSON(); !errors.Is(err, types.ErrManifestNotSet) && !errors.Is(err, types.ErrUnsupportedMediaType) {
					t.Errorf("MarshalJSON did not return ManifestNotSet: %v", err)
				}
				if ma, ok := m.(Annotator); ok {
					if _, err := ma.GetAnnotations(); !errors.Is(err, types.ErrManifestNotSet) && !errors.Is(err, types.ErrUnsupportedMediaType) {
						t.Errorf("GetAnnotations did not return ManifestNotSet: %v", err)
					}
				}
				if mi, ok := m.(Indexer); ok {
					if _, err := mi.GetManifestList(); !errors.Is(err, types.ErrManifestNotSet) && !errors.Is(err, types.ErrUnsupportedMediaType) {
						t.Errorf("GetManifestList did not return ManifestNotSet: %v", err)
					}
				}
				if mi, ok := m.(Imager); ok {
					if _, err := mi.GetConfig(); !errors.Is(err, types.ErrManifestNotSet) && !errors.Is(err, types.ErrUnsupportedMediaType) {
						t.Errorf("GetConfig did not return ManifestNotSet: %v", err)
					}
					if _, err := mi.GetLayers(); !errors.Is(err, types.ErrManifestNotSet) && !errors.Is(err, types.ErrUnsupportedMediaType) {
						t.Errorf("GetLayers did not return ManifestNotSet: %v", err)
					}
				}
				if ms, ok := m.(Subjecter); ok {
					if _, err := ms.GetSubject(); !errors.Is(err, types.ErrManifestNotSet) && !errors.Is(err, types.ErrUnsupportedMediaType) {
						t.Errorf("GetSubject did not return ManifestNotSet: %v", err)
					}
				}
			} else {
				// test methods on set manifest
				if !m.IsSet() {
					t.Errorf("manifest reports it is not set")
				}
				if _, err := m.RawBody(); errors.Is(err, types.ErrManifestNotSet) {
					t.Errorf("RawBody returned ManifestNotSet: %v", err)
				}
				if _, err := m.MarshalJSON(); errors.Is(err, types.ErrManifestNotSet) {
					t.Errorf("MarshalJSON returned ManifestNotSet: %v", err)
				}
				if ma, ok := m.(Annotator); ok {
					if _, err := ma.GetAnnotations(); errors.Is(err, types.ErrManifestNotSet) {
						t.Errorf("GetAnnotations returned ManifestNotSet: %v", err)
					}
				}
				if mi, ok := m.(Indexer); ok {
					if _, err := mi.GetManifestList(); errors.Is(err, types.ErrManifestNotSet) {
						t.Errorf("GetManifestList returned ManifestNotSet: %v", err)
					}
				}
				if mi, ok := m.(Imager); ok {
					if _, err := mi.GetConfig(); errors.Is(err, types.ErrManifestNotSet) {
						t.Errorf("GetConfig returned ManifestNotSet: %v", err)
					}
					if _, err := mi.GetLayers(); errors.Is(err, types.ErrManifestNotSet) {
						t.Errorf("GetLayers returned ManifestNotSet: %v", err)
					}
				}
				if ms, ok := m.(Subjecter); ok {
					if _, err := ms.GetSubject(); errors.Is(err, types.ErrManifestNotSet) {
						t.Errorf("GetSubject returned ManifestNotSet: %v", err)
					}
				}
			}
			if tt.testAnnot {
				mr, ok := m.(Annotator)
				if tt.hasAnnot {
					if !ok {
						t.Errorf("manifest does not support annotations")
					}
					err = mr.SetAnnotation("testkey", "testval")
					if err != nil {
						t.Errorf("failed setting annotation: %v", err)
					}
					getAnnot, err := mr.GetAnnotations()
					if err != nil {
						t.Errorf("failed getting annotations: %v", err)
					}
					if getAnnot == nil || getAnnot["testkey"] != "testval" {
						t.Errorf("annotation testkey missing, expected testval, received map %v", getAnnot)
					}
				} else if ok {
					t.Errorf("manifest supports annotations")
				}
			}
			if tt.testSubject {
				mr, ok := m.(Subjecter)
				if tt.hasSubject {
					if !ok {
						t.Errorf("manifest does not support subject")
					} else {
						err = mr.SetSubject(&subDesc)
						if err != nil {
							t.Errorf("failed setting subject: %v", err)
						}
						getDesc, err := mr.GetSubject()
						if err != nil {
							t.Errorf("failed getting subject: %v", err)
						}
						if getDesc == nil || getDesc.MediaType != subDesc.MediaType || getDesc.Digest != subDesc.Digest {
							t.Errorf("subject did not match, expected %v, received %v", subDesc, getDesc)
						}
					}
				} else if ok {
					t.Errorf("manifest supports subject")
				}
			}
			if tt.testPlat != "" {
				p, err := platform.Parse(tt.testPlat)
				if err != nil {
					t.Errorf("failed to parse platform %s: %v", tt.testPlat, err)
					return
				}
				d, err := GetPlatformDesc(m, &p)
				if err != nil {
					t.Errorf("failed to get descriptor: %v", err)
				} else if !tt.wantPlat.Same(*d) {
					t.Errorf("received platform mismatch, expected %v, received %v", tt.wantPlat, *d)
				}
			}
		})
	}
}

func TestModify(t *testing.T) {
	addDigest := digest.FromString("new layer digest")
	addDesc := types.Descriptor{
		Digest: addDigest,
		Size:   42,
		Annotations: map[string]string{
			"test": "new descriptor",
		},
	}
	// test list includes each original media type, a layer to add, run the convert
	// verify new layer, altered digest, and/or any error conditions
	tests := []struct {
		name     string
		opts     []Opts
		addDesc  types.Descriptor
		origDesc types.Descriptor
	}{
		{
			name: "Docker Schema 2 Manifest",
			opts: []Opts{
				WithDesc(types.Descriptor{
					MediaType: types.MediaTypeDocker2Manifest,
					Digest:    digestDockerSchema2,
					Size:      int64(len(rawDockerSchema2)),
				}),
				WithRaw(rawDockerSchema2),
			},
			addDesc: addDesc,
			origDesc: types.Descriptor{
				MediaType: types.MediaTypeDocker2Manifest,
				Digest:    digestDockerSchema2,
				Size:      int64(len(rawDockerSchema2)),
			},
		},
		{
			name: "Docker Schema 2 List",
			opts: []Opts{
				WithDesc(types.Descriptor{
					MediaType: types.MediaTypeDocker2ManifestList,
					Digest:    digestDockerSchema2List,
					Size:      int64(len(rawDockerSchema2List)),
				}),
				WithRaw(rawDockerSchema2List),
			},
			addDesc: addDesc,
			origDesc: types.Descriptor{
				MediaType: types.MediaTypeDocker2ManifestList,
				Digest:    digestDockerSchema2List,
				Size:      int64(len(rawDockerSchema2List)),
			},
		},
		{
			name: "OCI Image",
			opts: []Opts{
				WithRaw(rawOCIImage),
				WithDesc(types.Descriptor{
					MediaType: types.MediaTypeOCI1Manifest,
					Digest:    digestOCIImage,
					Size:      int64(len(rawOCIImage)),
				}),
			},
			addDesc: addDesc,
			origDesc: types.Descriptor{
				MediaType: types.MediaTypeOCI1Manifest,
				Digest:    digestOCIImage,
				Size:      int64(len(rawOCIImage)),
			},
		},
		{
			name: "OCI Index",
			opts: []Opts{
				WithRaw(rawOCIIndex),
				WithDesc(types.Descriptor{
					MediaType: types.MediaTypeOCI1ManifestList,
					Digest:    digestOCIIndex,
					Size:      int64(len(rawOCIIndex)),
				}),
			},
			addDesc: addDesc,
			origDesc: types.Descriptor{
				MediaType: types.MediaTypeOCI1ManifestList,
				Digest:    digestOCIIndex,
				Size:      int64(len(rawOCIIndex)),
			},
		},
	}

	// Loop of tests performing a round trip with different types
	// round trip creates the manifest, GetOrig, to OCI, modify, to Orig, SetOrig
	// resulting manifest should have a changed digest from the added layer
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := New(tt.opts...)
			if err != nil {
				t.Errorf("error creating manifest: %v", err)
				return
			}
			orig := m.GetOrig()
			if m.IsList() {
				ociI, err := OCIIndexFromAny(orig)
				if err != nil {
					t.Errorf("error converting to index: %v", err)
					return
				}
				ociI.Manifests = append(ociI.Manifests, tt.addDesc)
				err = OCIIndexToAny(ociI, &orig)
				if err != nil {
					t.Errorf("error converting back to orig: %v", err)
					return
				}
			} else {
				ociM, err := OCIManifestFromAny(orig)
				if err != nil {
					t.Errorf("error converting to index: %v", err)
					return
				}
				ociM.Layers = append(ociM.Layers, tt.addDesc)
				err = OCIManifestToAny(ociM, &orig)
				if err != nil {
					t.Errorf("error converting back to orig: %v", err)
					return
				}
			}
			err = m.SetOrig(orig)
			if err != nil {
				t.Errorf("error setting orig: %v", err)
				return
			}
			raw, _ := m.RawBody()
			t.Logf("raw manifest: %s", string(raw))
			desc := m.GetDescriptor()
			if tt.origDesc.Digest == desc.Digest {
				t.Errorf("digest was not modified")
			}
			if tt.origDesc.MediaType != desc.MediaType {
				t.Errorf("media type was modified: %s", desc.MediaType)
			}
		})
	}

	// Other test cases for error conditions
	var manifestDockerSchema2 schema2.Manifest
	var manifestDockerSchema2List schema2.ManifestList
	var manifestOCIImage v1.Manifest
	var manifestOCIIndex v1.Index
	err := json.Unmarshal(rawDockerSchema2, &manifestDockerSchema2)
	if err != nil {
		t.Errorf("failed to unmarshal docker schema2 json: %v", err)
		return
	}
	err = json.Unmarshal(rawDockerSchema2List, &manifestDockerSchema2List)
	if err != nil {
		t.Errorf("failed to unmarshal docker schema2 list json: %v", err)
		return
	}
	err = json.Unmarshal(rawOCIImage, &manifestOCIImage)
	if err != nil {
		t.Errorf("failed to unmarshal OCI image json: %v", err)
		return
	}
	err = json.Unmarshal(rawOCIIndex, &manifestOCIIndex)
	if err != nil {
		t.Errorf("failed to unmarshal OCI index json: %v", err)
		return
	}
	if manifestDockerSchema2.Annotations == nil || manifestDockerSchema2.Annotations["org.example.test"] != "hello world" {
		t.Errorf("annotation missing from docker manifest")
	}
	if manifestDockerSchema2List.Annotations == nil || manifestDockerSchema2List.Annotations["org.example.test"] != "hello world" {
		t.Errorf("annotation missing from docker manifest list")
	}
	if manifestOCIImage.Annotations == nil || manifestOCIImage.Annotations["org.example.test"] != "hello world" {
		t.Errorf("annotation missing from oci image")
	}
	if manifestOCIIndex.Annotations == nil || manifestOCIIndex.Annotations["org.example.test"] != "hello world" {
		t.Errorf("annotation missing from oci index")
	}

	t.Run("BadIndex", func(t *testing.T) {
		_, err = OCIIndexFromAny(manifestDockerSchema2)
		if err == nil {
			t.Errorf("did not fail converting docker manifest to OCI index")
		}
	})
	t.Run("BadManifest", func(t *testing.T) {
		_, err = OCIManifestFromAny(manifestDockerSchema2List)
		if err == nil {
			t.Errorf("did not fail converting docker manifest list to OCI image")
		}
	})
	t.Run("IndexToManifest", func(t *testing.T) {
		err = OCIIndexToAny(manifestOCIIndex, &manifestDockerSchema2)
		if err == nil {
			t.Errorf("did not fail converting OCI index to docker image")
		}
	})
	t.Run("ManifestToIndex", func(t *testing.T) {
		err = OCIManifestToAny(manifestOCIImage, &manifestDockerSchema2List)
		if err == nil {
			t.Errorf("did not fail converting OCI image to docker manifest list")
		}
	})
	t.Run("IndexWithoutPtr", func(t *testing.T) {
		err = OCIIndexToAny(manifestOCIIndex, manifestDockerSchema2List)
		if err == nil {
			t.Errorf("did not fail converting OCI index to non-pointer")
		}
	})
	t.Run("ManifestWithoutPtr", func(t *testing.T) {
		err = OCIManifestToAny(manifestOCIImage, manifestDockerSchema2)
		if err == nil {
			t.Errorf("did not fail converting OCI image to non-pointer")
		}
	})
	t.Run("IndexNil", func(t *testing.T) {
		err = OCIIndexToAny(manifestOCIIndex, nil)
		if err == nil {
			t.Errorf("did not fail converting OCI index to non-pointer")
		}
	})
	t.Run("ManifestNil", func(t *testing.T) {
		err = OCIManifestToAny(manifestOCIImage, nil)
		if err == nil {
			t.Errorf("did not fail converting OCI image to non-pointer")
		}
	})
}

// test set methods for config, layers, and manifest list
func TestSet(t *testing.T) {
	addDigest := digest.FromString("new digest")
	addDesc := types.Descriptor{
		Digest: addDigest,
		Size:   42,
		Annotations: map[string]string{
			"test": "new descriptor",
		},
	}
	// test list includes each original media type, a layer to add, run the convert
	// verify new layer, altered digest, and/or any error conditions
	tests := []struct {
		name        string
		opts        []Opts
		expectAnnot bool
		expectImage bool
		expectIndex bool
		expectErr   error
	}{
		{
			name: "Docker Schema 1",
			opts: []Opts{
				WithDesc(types.Descriptor{
					MediaType: types.MediaTypeDocker1ManifestSigned,
					Digest:    digestDockerSchema1Signed,
					Size:      int64(len(rawDockerSchema1Signed)),
				}),
				WithRaw(rawDockerSchema1Signed),
			},
			expectImage: true,
			expectErr:   types.ErrUnsupportedMediaType,
		},
		{
			name: "Docker Schema 2 Manifest",
			opts: []Opts{
				WithDesc(types.Descriptor{
					MediaType: types.MediaTypeDocker2Manifest,
					Digest:    digestDockerSchema2,
					Size:      int64(len(rawDockerSchema2)),
				}),
				WithRaw(rawDockerSchema2),
			},
			expectAnnot: true,
			expectImage: true,
		},
		{
			name: "Docker Schema 2 List",
			opts: []Opts{
				WithDesc(types.Descriptor{
					MediaType: types.MediaTypeDocker2ManifestList,
					Digest:    digestDockerSchema2List,
					Size:      int64(len(rawDockerSchema2List)),
				}),
				WithRaw(rawDockerSchema2List),
			},
			expectAnnot: true,
			expectIndex: true,
		},
		{
			name: "OCI Image",
			opts: []Opts{
				WithRaw(rawOCIImage),
				WithDesc(types.Descriptor{
					MediaType: types.MediaTypeOCI1Manifest,
					Digest:    digestOCIImage,
					Size:      int64(len(rawOCIImage)),
				}),
			},
			expectAnnot: true,
			expectImage: true,
		},
		{
			name: "OCI Index",
			opts: []Opts{
				WithRaw(rawOCIIndex),
				WithDesc(types.Descriptor{
					MediaType: types.MediaTypeOCI1ManifestList,
					Digest:    digestOCIIndex,
					Size:      int64(len(rawOCIIndex)),
				}),
			},
			expectAnnot: true,
			expectIndex: true,
		},
	}

	// Loop of tests performing a round trip with different types
	// round trip creates the manifest, GetOrig, to OCI, modify, to Orig, SetOrig
	// resulting manifest should have a changed digest from the added layer
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := New(tt.opts...)
			if err != nil {
				t.Errorf("error creating manifest: %v", err)
				return
			}
			if mi, ok := m.(Imager); ok {
				if !tt.expectImage {
					t.Errorf("image methods not expected")
				}
				prevDig := m.GetDescriptor().Digest
				dl, err := mi.GetLayers()
				if err != nil {
					t.Errorf("failed getting layers")
				}
				dl = append(dl, addDesc)
				err = mi.SetLayers(dl)
				if tt.expectErr != nil {
					if err == nil {
						t.Errorf("did not receive expected error")
					} else if !errors.Is(err, tt.expectErr) && err.Error() != tt.expectErr.Error() {
						t.Errorf("unexpected error, expected %v, received %v", tt.expectErr, err)
					}
					return
				} else if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if m.GetDescriptor().Digest == prevDig {
					t.Errorf("digest did not change after adding layer/manifest")
				}

				prevDig = m.GetDescriptor().Digest
				err = mi.SetConfig(addDesc)
				if err != nil {
					t.Errorf("failed setting config")
				}
				if m.GetDescriptor().Digest == prevDig {
					t.Errorf("digest did not change after adding layer/manifest")
				}
			} else if tt.expectImage {
				t.Errorf("image methods not found")
			}

			if mi, ok := m.(Indexer); ok {
				if !tt.expectIndex {
					t.Errorf("index methods not expected")
				}
				prevDig := m.GetDescriptor().Digest
				dl, err := mi.GetManifestList()
				if err != nil {
					t.Errorf("failed getting manifest list")
				}
				dl = append(dl, addDesc)
				err = mi.SetManifestList(dl)
				if tt.expectErr != nil {
					if err == nil {
						t.Errorf("did not receive expected error")
					} else if !errors.Is(err, tt.expectErr) && err.Error() != tt.expectErr.Error() {
						t.Errorf("unexpected error, expected %v, received %v", tt.expectErr, err)
					}
					return
				} else if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if m.GetDescriptor().Digest == prevDig {
					t.Errorf("digest did not change after adding layer/manifest")
				}
			} else if tt.expectIndex {
				t.Errorf("image methods not found")
			}

			if ma, ok := m.(Annotator); ok {
				if !tt.expectAnnot {
					t.Errorf("annotation methods not expected")
				}
				prevDig := m.GetDescriptor().Digest
				_, err := ma.GetAnnotations()
				if err != nil {
					t.Errorf("failed fetching annotations: %v", err)
					return
				}
				err = ma.SetAnnotation("new annotation", "new value")
				if err != nil {
					t.Errorf("failed setting annotation: %v", err)
					return
				}
				if m.GetDescriptor().Digest == prevDig {
					t.Errorf("digest did not change after setting annotation")
				}
			} else if tt.expectAnnot {
				t.Errorf("annotation methods not found")
			}

		})
	}
}
