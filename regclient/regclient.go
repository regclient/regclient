package regclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"

	dockercfg "github.com/docker/cli/cli/config"
	"github.com/docker/distribution/reference"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

type tlsConf int

var (
	// MediaTypeDocker2Manifest is the media type when pulling manifests from a v2 registry
	MediaTypeDocker2Manifest = "application/vnd.docker.distribution.manifest.v2+json"
	// MediaTypeDocker2ImageConfig is for the configuration json object
	MediaTypeDocker2ImageConfig = "application/vnd.docker.container.image.v1+json"
)

const (
	tlsEnabled tlsConf = iota
	tlsInsecure
	tlsDisabled
)

// RegClient provides an interfaces to working with registries
type RegClient interface {
	Auth() AuthClient
	RepoList(ctx context.Context, ref Ref) (TagList, error)
	ImageInspect(ctx context.Context, ref Ref) (ociv1.Image, error)
}

// TagList comes from github.com/opencontainers/distribution-spec,
// switch to their implementation when it becomes stable
type TagList struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

// Ref reference to a registry/repository
// If the tag or digest is available, it's also included in the reference.
// Reference itself is the unparsed string.
// While this is currently a struct, that may change in the future and access
// to contents should not be assumed/used.
type Ref struct {
	Reference, Registry, Repository, Tag, Digest string
}

type regClient struct {
	hosts map[string]*regHost
	auth  AuthClient
}

type regHost struct {
	scheme   string
	tls      tlsConf
	dnsNames []string
}

// Opt functions are used to configure NewRegClient
type Opt func(*regClient)

// NewRegClient returns a registry client
func NewRegClient(opts ...Opt) RegClient {
	var rc regClient

	rc.hosts = map[string]*regHost{"docker.io": {scheme: "https", tls: tlsEnabled, dnsNames: []string{"registry-1.docker.io"}}}

	rc.auth = NewAuthClient()

	for _, opt := range opts {
		opt(&rc)
	}

	return &rc
}

// WithDockerCerts adds certificates trusted by docker in /etc/docker/certs.d
func WithDockerCerts() Opt {
	return func(rc *regClient) {
		return
	}
}

// WithDockerCreds adds configuration from users docker config with registry logins
func WithDockerCreds() Opt {
	return func(rc *regClient) {
		conffile := dockercfg.LoadDefaultConfigFile(os.Stderr)
		creds, err := conffile.GetAllCredentials()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load docker creds %s\n", err)
			return
		}
		for _, cred := range creds {
			// fmt.Printf("Processing cred %v\n", cred)
			// TODO: clean this up, get index and registry-1 from variables
			if cred.ServerAddress == "https://index.docker.io/v1/" && cred.Username != "" && cred.Password != "" {
				rc.auth.Set("registry-1.docker.io", cred.Username, cred.Password)
			} else if cred.ServerAddress != "" && cred.Username != "" && cred.Password != "" {
				rc.auth.Set(cred.ServerAddress, cred.Username, cred.Password)
			}
		}
		return
	}
}

// WithRegClientConf adds configuration from regclient
func WithRegClientConf() Opt {
	return func(rc *regClient) {
		return
	}
}

// NewRef returns a repository reference including a registry, repository (path), digest, and tag
func NewRef(ref string) (Ref, error) {
	parsed, err := reference.ParseNormalizedNamed(ref)

	var ret Ref
	ret.Reference = ref

	if err != nil {
		return ret, err
	}

	ret.Registry = reference.Domain(parsed)
	ret.Repository = reference.Path(parsed)

	if canonical, ok := parsed.(reference.Canonical); ok {
		ret.Digest = canonical.Digest().String()
	}

	if tagged, ok := parsed.(reference.Tagged); ok {
		ret.Tag = tagged.Tag()
	}

	return ret, nil
}

func (rc *regClient) Auth() AuthClient {
	return rc.auth
}

func (rc *regClient) RepoList(ctx context.Context, ref Ref) (TagList, error) {
	tl := TagList{}
	host, ok := rc.hosts[ref.Registry]
	if !ok {
		host = &regHost{scheme: "https", tls: tlsEnabled, dnsNames: []string{ref.Registry}}
		rc.hosts[ref.Registry] = host
	}
	repoURL := url.URL{
		Scheme: host.scheme,
		Host:   host.dnsNames[0],
		Path:   "/v2/" + ref.Repository + "/tags/list",
	}

	req, err := http.NewRequest("GET", repoURL.String(), nil)
	if err != nil {
		return tl, err
	}
	rty := NewRetryable(RetryWithLimit(3))
	resp, err := rty.Req(ctx, rc, req)
	if err != nil {
		return tl, err
	}
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return tl, err
	}
	err = json.Unmarshal(respBody, &tl)
	if err != nil {
		return tl, err
	}

	return tl, nil
}

func (rc *regClient) ImageInspect(ctx context.Context, ref Ref) (ociv1.Image, error) {
	m := ociv1.Manifest{}
	img := ociv1.Image{}

	host, ok := rc.hosts[ref.Registry]
	if !ok {
		host = &regHost{scheme: "https", tls: tlsEnabled, dnsNames: []string{ref.Registry}}
		rc.hosts[ref.Registry] = host
	}
	var tagOrDigest string
	if ref.Digest != "" {
		tagOrDigest = ref.Digest
	} else if ref.Tag != "" {
		tagOrDigest = ref.Tag
	} else {
		return img, ErrMissingTag
	}

	manfURL := url.URL{
		Scheme: host.scheme,
		Host:   host.dnsNames[0],
		Path:   "/v2/" + ref.Repository + "/manifests/" + tagOrDigest,
	}

	req, err := http.NewRequest("GET", manfURL.String(), nil)
	if err != nil {
		return img, err
	}
	req.Header.Add("Accept", MediaTypeDocker2Manifest)
	req.Header.Add("Accept", ociv1.MediaTypeImageManifest)

	rty := NewRetryable(RetryWithLimit(3))
	resp, err := rty.Req(ctx, rc, req)
	if err != nil {
		return img, err
	}
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return img, err
	}
	err = json.Unmarshal(respBody, &m)
	if err != nil {
		return img, err
	}

	confURL := url.URL{
		Scheme: host.scheme,
		Host:   host.dnsNames[0],
		Path:   "/v2/" + ref.Repository + "/blobs/" + m.Config.Digest.String(),
	}

	req, err = http.NewRequest("GET", confURL.String(), nil)
	if err != nil {
		return img, err
	}
	req.Header.Add("Accept", MediaTypeDocker2ImageConfig)
	req.Header.Add("Accept", ociv1.MediaTypeImageConfig)
	resp, err = rty.Req(ctx, rc, req)
	if err != nil {
		return img, err
	}
	// fmt.Printf("Headers: %v\n", resp.Header)
	respBody, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return img, err
	}
	// fmt.Printf("Body:\n%s\n", respBody)
	err = json.Unmarshal(respBody, &img)
	if err != nil {
		return img, err
	}
	return img, nil
}
