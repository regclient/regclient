package tag

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/regclient/regclient/types/errs"
	"github.com/regclient/regclient/types/ref"
)

func TestNew(t *testing.T) {
	t.Parallel()
	emptyRaw := []byte("{}")
	registryTags := []string{"cache", "edge", "edge-alpine", "alpine", "latest"}
	reqURL, err := url.Parse("http://localhost:5000/v2/regclient/test/tag/list")
	if err != nil {
		t.Fatalf("failed to parse URL: %v", err)
	}
	registryRef, _ := ref.New("localhost:5000/regclient/test")
	registryRepoName := "regclient/test"
	registryRaw := []byte(fmt.Sprintf(`{"name":"%s","tags":["%s"]}`, registryRepoName,
		strings.Join(registryTags, `","`)))
	registryMediaType := "application/json"
	registryHeaders := http.Header{
		"Content-Type": {registryMediaType},
	}
	gcrChild := []string{"ci", "ci-builds", "cosign"}
	gcrManifests := map[string]GCRManifestInfo{
		"sha256:135d8c5e27bdc917f04b415fc947d7d5b1137f99bb8fa00bffc3eca1856e9c52": {
			Size:      22713600,
			MediaType: "application/vnd.docker.distribution.manifest.v2+json",
			Tags:      []string{"v0.3.0"},
			Created:   fromUnixMs(0),
			Uploaded:  fromUnixMs(1618865456521),
		},
		"sha256:149a9c738a03c211fb1a11e33624a5038522020610bba10509bfe2ed02487d18": {
			Size:      43769862,
			MediaType: "application/vnd.docker.distribution.manifest.v2+json",
			Tags:      []string{},
			Created:   fromUnixMs(0),
			Uploaded:  fromUnixMs(1638834901158),
		},
		"sha256:15c805a1ffce32704589ff2ec34604a66cd304d829d7422d36291a84c58eb0f8": {
			Size:      296,
			MediaType: "application/vnd.oci.image.manifest.v1+json",
			Tags:      []string{},
			Created:   fromUnixMs(-62135596800000),
			Uploaded:  fromUnixMs(1631661258978),
		},
		"sha256:1674ddbc630554763fcd62fee8d6fe37a39354b7be9812c32112f936301d3030": {
			Size:      557,
			MediaType: "application/vnd.oci.image.manifest.v1+json",
			Tags: []string{
				"sha256-96ef6fb02c5a56901dc3c2e0ca34eec9ed926ab8d936ea30ec38f9ec9db017a5.sig",
			},
			Created:  fromUnixMs(-62135596800000),
			Uploaded: fromUnixMs(1631661262320),
		},
	}
	gcrManifestRaw := `
		"sha256:135d8c5e27bdc917f04b415fc947d7d5b1137f99bb8fa00bffc3eca1856e9c52": {
			"imageSizeBytes": "22713600",
			"layerId": "",
			"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
			"tag": [
				"v0.3.0"
			],
			"timeCreatedMs": "0",
			"timeUploadedMs": "1618865456521"
		},
		"sha256:149a9c738a03c211fb1a11e33624a5038522020610bba10509bfe2ed02487d18": {
			"imageSizeBytes": "43769862",
			"layerId": "",
			"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
			"tag": [],
			"timeCreatedMs": "0",
			"timeUploadedMs": "1638834901158"
		},
		"sha256:15c805a1ffce32704589ff2ec34604a66cd304d829d7422d36291a84c58eb0f8": {
			"imageSizeBytes": "296",
			"layerId": "",
			"mediaType": "application/vnd.oci.image.manifest.v1+json",
			"tag": [],
			"timeCreatedMs": "-62135596800000",
			"timeUploadedMs": "1631661258978"
		},
		"sha256:1674ddbc630554763fcd62fee8d6fe37a39354b7be9812c32112f936301d3030": {
			"imageSizeBytes": "557",
			"layerId": "",
			"mediaType": "application/vnd.oci.image.manifest.v1+json",
			"tag": [
				"sha256-96ef6fb02c5a56901dc3c2e0ca34eec9ed926ab8d936ea30ec38f9ec9db017a5.sig"
			],
			"timeCreatedMs": "-62135596800000",
			"timeUploadedMs": "1631661262320"
		}
	`
	gcrTags := []string{"v0.3.0", "sha256-96ef6fb02c5a56901dc3c2e0ca34eec9ed926ab8d936ea30ec38f9ec9db017a5.sig"}
	gcrRef, _ := ref.New("gcr.io/example/test")
	gcrRepoName := "example/test"
	gcrRaw := []byte(fmt.Sprintf(`{"child": ["%s"], "manifest":{%s}, "name":"%s", "tags":["%s"]}`,
		strings.Join(gcrChild, `","`),
		gcrManifestRaw,
		gcrRepoName,
		strings.Join(gcrTags, `","`)))
	gcrMediaType := "application/json"
	gcrHeaders := http.Header{
		"Content-Type": {gcrMediaType},
	}
	tests := []struct {
		name string
		opts []Opts
		// all remaining fields are expected results from creating a tag with opts
		err error
		raw []byte
		// ref          types.Ref
		// mt           string
		repoName     string
		tags         []string
		gcrChildren  []string
		gcrManifests map[string]GCRManifestInfo
	}{
		{
			name: "Empty",
			opts: []Opts{
				WithRaw(emptyRaw),
			},
			raw: emptyRaw,
		},
		{
			name: "Registry",
			opts: []Opts{
				WithRef(registryRef),
				WithRaw(registryRaw),
				WithHeaders(registryHeaders),
				WithMT(registryMediaType),
			},
			raw:      registryRaw,
			repoName: registryRepoName,
			tags:     registryTags,
		},
		{
			name: "Registry with HTTP Response",
			opts: []Opts{
				WithRef(registryRef),
				WithRaw(registryRaw),
				WithResp(&http.Response{
					Header: registryHeaders,
					Request: &http.Request{
						URL: reqURL,
					},
				}),
			},
			raw:      registryRaw,
			repoName: registryRepoName,
			tags:     registryTags,
		},
		{
			name: "Registry with HTTP Response and Body",
			opts: []Opts{
				WithRef(registryRef),
				WithResp(&http.Response{
					Header:        registryHeaders,
					Body:          io.NopCloser(bytes.NewReader(registryRaw)),
					ContentLength: int64(len(registryRaw)),
					Request: &http.Request{
						URL: reqURL,
					},
				}),
			},
			raw:      registryRaw,
			repoName: registryRepoName,
			tags:     registryTags,
		},
		{
			name: "GCR",
			opts: []Opts{
				WithRef(gcrRef),
				WithRaw(gcrRaw),
				WithHeaders(gcrHeaders),
				WithMT(gcrMediaType),
			},
			raw:          gcrRaw,
			repoName:     gcrRepoName,
			tags:         gcrTags,
			gcrChildren:  gcrChild,
			gcrManifests: gcrManifests,
		},
		{
			name: "Unknown MT",
			opts: []Opts{
				WithRef(registryRef),
				WithRaw(registryRaw),
				WithHeaders(registryHeaders),
				WithMT("application/unknown"),
			},
			err: errs.ErrUnsupportedMediaType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tl, err := New(tt.opts...)
			if tt.err != nil {
				if err == nil || !errors.Is(err, tt.err) {
					t.Errorf("expected error not found, expected %v, received %v", tt.err, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("error creating tag list: %v", err)
			}
			raw, err := tl.RawBody()
			if err != nil {
				t.Errorf("error from RawBody: %v", err)
			} else if !bytes.Equal(tt.raw, raw) {
				t.Errorf("unexpected raw body: expected %s, received %s", tt.raw, raw)
			}
			if tt.repoName != tl.Name {
				t.Errorf("unexpected repo name: expected %s, received %s", tt.repoName, tl.Name)
			}
			tags, err := tl.GetTags()
			if err != nil {
				t.Fatalf("error from GetTags: %v", err)
			}
			if cmpSliceString(tt.tags, tags) == false {
				t.Errorf("unexpected tag list: expected %v, received %v", tt.tags, tags)
			}
			if cmpSliceString(tt.gcrChildren, tl.Children) == false {
				t.Errorf("unexpected gcr children: expected %v, received %v", tt.gcrChildren, tl.Children)
			}
			if cmpManifestInfos(tt.gcrManifests, tl.Manifests) == false {
				t.Errorf("unexpected gcr manifest: expected %v, received %v", tt.gcrManifests, tl.Manifests)
			}
		})
	}
}

func TestAppend(t *testing.T) {
	t.Parallel()
	expectTags := []string{"1", "2", "3", "4", "5", "6"}
	tl1, err := New(
		WithTags(expectTags[:3]),
	)
	if err != nil {
		t.Fatalf("failed to build tag list 1: %v", err)
	}
	tl2, err := New(
		WithTags(expectTags[3:]),
	)
	if err != nil {
		t.Fatalf("failed to build tag list 1: %v", err)
	}
	err = tl1.Append(tl2)
	if err != nil {
		t.Fatalf("failed to append tags: %v", err)
	}
	if !cmpSliceString(tl1.Tags, expectTags) {
		t.Errorf("tags mismatch, expected: %v, received %v", expectTags, tl1.Tags)
	}
}

func cmpSliceString(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func cmpManifestInfos(a, b map[string]GCRManifestInfo) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if _, ok := b[i]; !ok {
			return false
		}
		if a[i].Created != b[i].Created ||
			a[i].Uploaded != b[i].Uploaded ||
			a[i].MediaType != b[i].MediaType ||
			a[i].Size != b[i].Size ||
			cmpSliceString(a[i].Tags, b[i].Tags) == false {
			return false
		}
	}
	return true
}
