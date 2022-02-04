package tag

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/regclient/regclient/types"
	"github.com/regclient/regclient/types/ref"
)

// List contains a tag list
// Currently this is a struct but the underlying type could be changed to an interface in the future
// Using methods is recommended over directly accessing fields
type List struct {
	tagCommon
	DockerList
	GCRList
}

type tagCommon struct {
	r         ref.Ref
	mt        string
	orig      interface{}
	rawHeader http.Header
	rawBody   []byte
}

// DockerList is returned from registry/2.0 API's
type DockerList struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

// GCRList fields are from gcr.io
type GCRList struct {
	Children  []string                   `json:"child,omitempty"`
	Manifests map[string]GCRManifestInfo `json:"manifest,omitempty"`
}

type tagConfig struct {
	ref    ref.Ref
	mt     string
	raw    []byte
	header http.Header
	tags   []string
}

// Opts defines options for creating a new tag
type Opts func(*tagConfig)

// New creates a tag list from options
// Tags may be provided directly, or they will be parsed from the raw input based on the media type
func New(opts ...Opts) (*List, error) {
	conf := tagConfig{
		mt: "application/json",
	}
	for _, opt := range opts {
		opt(&conf)
	}
	tl := List{}
	tc := tagCommon{
		r:         conf.ref,
		mt:        conf.mt,
		rawHeader: conf.header,
		rawBody:   conf.raw,
	}
	if len(conf.tags) > 0 {
		tl.Tags = conf.tags
	}
	mt := strings.Split(conf.mt, ";")[0] // "application/json; charset=utf-8" -> "application/json"
	switch mt {
	case "application/json", "text/plain":
		err := json.Unmarshal(conf.raw, &tl)
		if err != nil {
			return nil, err
		}
	case types.MediaTypeOCI1ManifestList:
		// noop
	default:
		return nil, fmt.Errorf("%w: media type: %s, reference: %s", types.ErrUnsupportedMediaType, conf.mt, conf.ref.CommonName())
	}
	tl.tagCommon = tc

	return &tl, nil
}

// WithHeaders includes data from http headers when creating tag list
func WithHeaders(header http.Header) Opts {
	return func(tConf *tagConfig) {
		tConf.header = header
	}
}

// WithMT sets the returned media type on the tag list
func WithMT(mt string) Opts {
	return func(tConf *tagConfig) {
		tConf.mt = mt
	}
}

// WithRaw defines the raw response from the tag list request
func WithRaw(raw []byte) Opts {
	return func(tConf *tagConfig) {
		tConf.raw = raw
	}
}

// WithRef specifies the reference (repository) associated with the tag list
func WithRef(ref ref.Ref) Opts {
	return func(tConf *tagConfig) {
		tConf.ref = ref
	}
}

// WithTags provides the parsed tags for the tag list
func WithTags(tags []string) Opts {
	return func(tConf *tagConfig) {
		tConf.tags = tags
	}
}

// GetOrig returns the underlying tag data structure if defined
func (t tagCommon) GetOrig() interface{} {
	return t.orig
}

// MarshalJSON returns the tag list in json
func (t tagCommon) MarshalJSON() ([]byte, error) {
	if len(t.rawBody) > 0 {
		return t.rawBody, nil
	}

	if t.orig != nil {
		return json.Marshal((t.orig))
	}
	return []byte{}, fmt.Errorf("JSON marshalling failed: %w", types.ErrNotFound)
}

// RawBody returns the original tag list response
func (t tagCommon) RawBody() ([]byte, error) {
	return t.rawBody, nil
}

// RawHeaders returns the received http headers
func (t tagCommon) RawHeaders() (http.Header, error) {
	return t.rawHeader, nil
}

// GetTags returns the tags from a list
func (tl DockerList) GetTags() ([]string, error) {
	return tl.Tags, nil
}

// MarshalPretty is used for printPretty template formatting
func (tl DockerList) MarshalPretty() ([]byte, error) {
	sort.Slice(tl.Tags, func(i, j int) bool {
		return strings.Compare(tl.Tags[i], tl.Tags[j]) < 0
	})
	buf := &bytes.Buffer{}
	for _, tag := range tl.Tags {
		fmt.Fprintf(buf, "%s\n", tag)
	}
	return buf.Bytes(), nil
}
