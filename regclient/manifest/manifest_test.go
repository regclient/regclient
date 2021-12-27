package manifest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	dockerSchema1 "github.com/docker/distribution/manifest/schema1"
	dockerSchema2 "github.com/docker/distribution/manifest/schema2"
	"github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient/regclient/types"
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
			]
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
		]
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
			]
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
			]
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
			]
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

var ()

func TestNewManifest(t *testing.T) {
	digestML := digest.FromBytes(rawDockerSchema2List)
	digestInvalid := digest.FromString("invalid")
	ref, _ := types.NewRef("localhost:5000/test:latest")
	var tests = []struct {
		name   string
		mt     string
		raw    []byte
		ref    types.Ref
		header http.Header
		wantE  error
	}{
		{
			name:  "Docker Schema 2 Manifest",
			mt:    MediaTypeDocker2Manifest,
			raw:   rawDockerSchema2,
			ref:   ref,
			wantE: nil,
		},
		{
			name: "Docker Schema 2 List from Http",
			header: http.Header{
				"Content-Type":          []string{MediaTypeDocker2ManifestList},
				"Docker-Content-Digest": []string{digestML.String()},
			},
			raw:   rawDockerSchema2List,
			ref:   ref,
			wantE: nil,
		},
		{
			name:  "Docker Schema 1 Signed",
			mt:    MediaTypeDocker1ManifestSigned,
			raw:   rawDockerSchema1Signed,
			ref:   ref,
			wantE: nil,
		},
		{
			name: "Invalid Http Digest",
			header: http.Header{
				"Content-Type":          []string{MediaTypeDocker2ManifestList},
				"Docker-Content-Digest": []string{digestInvalid.String()},
			},
			raw:   rawDockerSchema2List,
			ref:   ref,
			wantE: fmt.Errorf("digest mismatch, expected %s, found %s", digestInvalid, digestML),
		},
		{
			name:  "Ambiguous OCI Image",
			mt:    MediaTypeOCI1Manifest,
			raw:   rawAmbiguousOCI,
			ref:   ref,
			wantE: nil,
		},
		{
			name:  "Ambiguous OCI Index",
			mt:    MediaTypeOCI1ManifestList,
			raw:   rawAmbiguousOCI,
			ref:   ref,
			wantE: nil,
		},
		{
			name:  "Invalid OCI Index",
			mt:    MediaTypeOCI1ManifestList,
			raw:   rawOCIImage,
			ref:   ref,
			wantE: fmt.Errorf("manifest contains an unexpected media type: expected %s, received %s", MediaTypeOCI1ManifestList, MediaTypeOCI1Manifest),
		},
		{
			name:  "Invalid OCI Image",
			mt:    MediaTypeOCI1Manifest,
			raw:   rawOCIIndex,
			ref:   ref,
			wantE: fmt.Errorf("manifest contains an unexpected media type: expected %s, received %s", MediaTypeOCI1Manifest, MediaTypeOCI1ManifestList),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.mt, tt.raw, tt.ref, tt.header)
			if tt.wantE == nil && err != nil {
				t.Errorf("failed creating manifest, err: %v", err)
			} else if tt.wantE != nil && (err == nil || (tt.wantE != err && tt.wantE.Error() != err.Error())) {
				t.Errorf("expected error not received, expected %v, received %v", tt.wantE, err)
			}
		})
	}
}

func TestFromDescriptor(t *testing.T) {
	digestInvalid := digest.FromString("invalid")
	digestDockerSchema2 := digest.FromBytes(rawDockerSchema2)
	digestDockerSchema1Signed, err := digest.Parse("sha256:f3ef067962554c3352dc0c659ca563f73cc396fe0dea2a2c23a7964c6290f782")
	if err != nil {
		t.Fatalf("failed to parse docker schema1 signed digest string: %v", err)
	}
	digestOCIImage := digest.FromBytes(rawOCIImage)
	var tests = []struct {
		name  string
		desc  ociv1.Descriptor
		raw   []byte
		wantE error
	}{
		{
			name: "Docker Schema 2 Manifest",
			desc: ociv1.Descriptor{
				MediaType: MediaTypeDocker2Manifest,
				Digest:    digestDockerSchema2,
				Size:      int64(len(rawDockerSchema2)),
			},
			raw:   rawDockerSchema2,
			wantE: nil,
		},
		{
			name: "Docker Schema 1 Signed Manifest",
			desc: ociv1.Descriptor{
				MediaType: MediaTypeDocker1ManifestSigned,
				Digest:    digestDockerSchema1Signed,
				Size:      int64(len(rawDockerSchema1Signed)),
			},
			raw:   rawDockerSchema1Signed,
			wantE: nil,
		},
		{
			name: "Invalid digest",
			desc: ociv1.Descriptor{
				MediaType: MediaTypeDocker2Manifest,
				Digest:    digestInvalid,
				Size:      int64(len(rawDockerSchema2)),
			},
			raw:   rawDockerSchema2,
			wantE: fmt.Errorf("digest mismatch, expected %s, found %s", digestInvalid, digestDockerSchema2),
		},
		{
			name: "Invalid Media Type",
			desc: ociv1.Descriptor{
				MediaType: MediaTypeOCI1ManifestList,
				Digest:    digestOCIImage,
				Size:      int64(len(rawOCIImage)),
			},
			raw:   rawOCIImage,
			wantE: fmt.Errorf("manifest contains an unexpected media type: expected %s, received %s", MediaTypeOCI1ManifestList, MediaTypeOCI1Manifest),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := FromDescriptor(tt.desc, tt.raw)
			if tt.wantE == nil && err != nil {
				t.Errorf("failed creating manifest, err: %v", err)
			} else if tt.wantE != nil && (err == nil || (tt.wantE != err && tt.wantE.Error() != err.Error())) {
				t.Errorf("expected error not received, expected %v, received %v", tt.wantE, err)
			}
		})
	}
}

func TestFromOrig(t *testing.T) {
	var manifestDockerSchema2, manifestInvalid dockerSchema2.Manifest
	var manifestDockerSchema1Signed dockerSchema1.SignedManifest
	err := json.Unmarshal(rawDockerSchema2, &manifestDockerSchema2)
	if err != nil {
		t.Fatalf("failed to unmarshal docker schema2 json: %v", err)
	}
	err = json.Unmarshal(rawDockerSchema2, &manifestInvalid)
	if err != nil {
		t.Fatalf("failed to unmarshal docker schema2 json: %v", err)
	}
	manifestInvalid.MediaType = MediaTypeOCI1Manifest
	err = json.Unmarshal(rawDockerSchema1Signed, &manifestDockerSchema1Signed)
	var tests = []struct {
		name  string
		orig  interface{}
		wantE error
	}{
		{
			name:  "Nil interface",
			orig:  nil,
			wantE: fmt.Errorf("Unsupported type to convert to a manifest: %v", nil),
		},
		{
			name:  "Docker Schema2",
			orig:  manifestDockerSchema2,
			wantE: nil,
		},
		{
			name:  "Docker Schema1 Signed",
			orig:  manifestDockerSchema1Signed,
			wantE: nil,
		},
		{
			name:  "Invalid Media Type",
			orig:  manifestInvalid,
			wantE: fmt.Errorf("manifest contains an unexpected media type: expected %s, received %s", MediaTypeDocker2Manifest, MediaTypeOCI1Manifest),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := FromOrig(tt.orig)
			if tt.wantE == nil && err != nil {
				t.Errorf("failed creating manifest, err: %v", err)
			} else if tt.wantE != nil && (err == nil || (tt.wantE != err && tt.wantE.Error() != err.Error())) {
				t.Errorf("expected error not received, expected %v, received %v", tt.wantE, err)
			}
		})
	}
}
