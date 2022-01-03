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

type TagList struct {
	tagCommon
	TagDockerList
	TagGCRList
}

type tagCommon struct {
	r         ref.Ref
	mt        string
	orig      interface{}
	rawHeader http.Header
	rawBody   []byte
}

// TagDockerList is returned from registry/2.0 API's
type TagDockerList struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

// TagGCRList fields are from gcr.io
type TagGCRList struct {
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

type Opts func(*tagConfig)

func New(opts ...Opts) (*TagList, error) {
	conf := tagConfig{
		mt: "application/json",
	}
	for _, opt := range opts {
		opt(&conf)
	}
	tl := TagList{}
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

func WithHeaders(header http.Header) Opts {
	return func(tConf *tagConfig) {
		tConf.header = header
	}
}
func WithMT(mt string) Opts {
	return func(tConf *tagConfig) {
		tConf.mt = mt
	}
}
func WithRaw(raw []byte) Opts {
	return func(tConf *tagConfig) {
		tConf.raw = raw
	}
}
func WithRef(ref ref.Ref) Opts {
	return func(tConf *tagConfig) {
		tConf.ref = ref
	}
}
func WithTags(tags []string) Opts {
	return func(tConf *tagConfig) {
		tConf.tags = tags
	}
}

func (t tagCommon) GetOrig() interface{} {
	return t.orig
}

func (t tagCommon) MarshalJSON() ([]byte, error) {
	if len(t.rawBody) > 0 {
		return t.rawBody, nil
	}

	if t.orig != nil {
		return json.Marshal((t.orig))
	}
	return []byte{}, fmt.Errorf("Json marshalling failed: %w", types.ErrNotFound)
}

func (t tagCommon) RawBody() ([]byte, error) {
	return t.rawBody, nil
}

func (t tagCommon) RawHeaders() (http.Header, error) {
	return t.rawHeader, nil
}

// GetTags returns the tags from a list
func (tl TagDockerList) GetTags() ([]string, error) {
	return tl.Tags, nil
}

// MarshalPretty is used for printPretty template formatting
func (tl TagDockerList) MarshalPretty() ([]byte, error) {
	sort.Slice(tl.Tags, func(i, j int) bool {
		if strings.Compare(tl.Tags[i], tl.Tags[j]) < 0 {
			return true
		}
		return false
	})
	buf := &bytes.Buffer{}
	for _, tag := range tl.Tags {
		fmt.Fprintf(buf, "%s\n", tag)
	}
	return buf.Bytes(), nil
}
