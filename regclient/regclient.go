package regclient

import (
	"bytes"
	"context"
	"strings"

	// crypto libraries included for go-digest
	_ "crypto/sha256"
	_ "crypto/sha512"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	dockercfg "github.com/docker/cli/cli/config"
	"github.com/docker/distribution/reference"
	"github.com/docker/docker/pkg/archive"
	digest "github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

type tlsConf int

var (
	// MediaTypeDocker2Manifest is the media type when pulling manifests from a v2 registry
	MediaTypeDocker2Manifest     = "application/vnd.docker.distribution.manifest.v2+json"
	MediaTypeDocker2ManifestList = "application/vnd.docker.distribution.manifest.list.v2+json"
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
	BlobGet(ctx context.Context, ref Ref, d string, accepts []string) (io.ReadCloser, *http.Response, error)
	ImageCopy(ctx context.Context, refSrc Ref, refTgt Ref) error
	ImageExport(ctx context.Context, ref Ref, outStream io.Writer) error
	ImageInspect(ctx context.Context, ref Ref) (ociv1.Image, error)
	ManifestGet(ctx context.Context, ref Ref) (Manifest, error)
	TagsList(ctx context.Context, ref Ref) (TagList, error)
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
	hosts      map[string]*regHost
	auth       AuthClient
	retryLimit int
}

type regHost struct {
	scheme    string
	tls       tlsConf
	dnsNames  []string
	transport *http.Transport
}

// used by image import/export to match docker tar expected format
type dockerTarManifest struct {
	Config   string
	RepoTags []string
	Layers   []string
}

// Opt functions are used to configure NewRegClient
type Opt func(*regClient)

// NewRegClient returns a registry client
func NewRegClient(opts ...Opt) RegClient {
	var rc regClient

	// TODO: move hardcoded host references into vars defined in another file
	rc.hosts = map[string]*regHost{"docker.io": {scheme: "https", tls: tlsEnabled, dnsNames: []string{"registry-1.docker.io"}}}
	rc.auth = NewAuthClient()
	rc.retryLimit = 5

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

// WithRegClientConf adds configuration from regcli configuration file (yml?)
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

// CommonName outputs a parsable name from a reference
func (r Ref) CommonName() string {
	cn := ""
	if r.Registry != "" {
		cn = r.Registry + "/"
	}
	if r.Repository == "" {
		return ""
	}
	cn = cn + r.Repository
	if r.Tag != "" {
		cn = cn + ":" + r.Tag
	}
	if r.Digest != "" {
		cn = cn + "@" + r.Digest
	}
	return cn
}

func (rc *regClient) Auth() AuthClient {
	return rc.auth
}

func (rc *regClient) BlobCopy(ctx context.Context, refSrc Ref, refTgt Ref, d string) error {
	// for the same repository, there's nothing to copy
	if refSrc.Repository == refTgt.Repository {
		return nil
	}
	// check if layer already exists
	if err := rc.BlobHead(ctx, refTgt, d); err == nil {
		return nil
	}
	// try mounting blob from the source repo is the registry is the same
	if refSrc.Registry == refTgt.Registry {
		err := rc.BlobMount(ctx, refSrc, refTgt, d)
		if err == nil {
			return nil
		}
		fmt.Fprintf(os.Stderr, "Failed to mount blob: %s\n", err)
	}
	// fast options failed, download layer from source and push to target
	blobIO, layerResp, err := rc.BlobGet(ctx, refSrc, d, []string{})
	if err != nil {
		return err
	}
	if err := rc.BlobPut(ctx, refTgt, d, blobIO, layerResp.Header.Get("Content-Type"), layerResp.ContentLength); err != nil {
		blobIO.Close()
		return err
	}
	return nil
}

func (rc *regClient) BlobGet(ctx context.Context, ref Ref, d string, accepts []string) (io.ReadCloser, *http.Response, error) {
	host := rc.getHost(ref.Registry)

	blobURL := url.URL{
		Scheme: host.scheme,
		Host:   host.dnsNames[0],
		Path:   "/v2/" + ref.Repository + "/blobs/" + d,
	}

	req, err := http.NewRequest("GET", blobURL.String(), nil)
	if err != nil {
		return nil, nil, err
	}
	for _, accept := range accepts {
		req.Header.Add("Accept", accept)
	}

	rty := rc.newRetryableForHost(host)
	resp, err := rty.Req(ctx, rc, req)
	if err != nil {
		return nil, nil, err
	}
	return resp.Body, resp, nil
}

func (rc *regClient) BlobHead(ctx context.Context, ref Ref, d string) error {
	host := rc.getHost(ref.Registry)

	blobURL := url.URL{
		Scheme: host.scheme,
		Host:   host.dnsNames[0],
		Path:   "/v2/" + ref.Repository + "/blobs/" + d,
	}

	req, err := http.NewRequest("HEAD", blobURL.String(), nil)
	if err != nil {
		return err
	}
	rty := rc.newRetryableForHost(host)
	resp, err := rty.Req(ctx, rc, req)
	if err != nil {
		return err
	}
	resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return ErrNotFound
	}

	return nil
}

func (rc *regClient) BlobMount(ctx context.Context, refSrc Ref, refTgt Ref, d string) error {
	if refSrc.Registry != refTgt.Registry {
		return fmt.Errorf("Registry must match for blob mount")
	}

	host := rc.getHost(refTgt.Registry)
	mountURL := url.URL{
		Scheme:   host.scheme,
		Host:     host.dnsNames[0],
		Path:     "/v2/" + refTgt.Repository + "/blobs/uploads/",
		RawQuery: "mount=" + d + "&from=" + refSrc.Repository,
	}

	req, err := http.NewRequest("POST", mountURL.String(), nil)
	if err != nil {
		return fmt.Errorf("Error creating manifest put request: %w", err)
	}

	rty := rc.newRetryableForHost(host)
	resp, err := rty.Req(ctx, rc, req)
	if err != nil {
		return fmt.Errorf("Error calling blob mount request: %w\nRequest object: %v\nResponse object: %v", err, req, resp)
	}
	if resp.StatusCode != 201 {
		return fmt.Errorf("Blob mount did not return a 201 status, status code: %d\nRequest object: %v\nResponse object: %v", resp.StatusCode, req, resp)
	}

	return nil
}

func (rc *regClient) BlobPut(ctx context.Context, ref Ref, d string, rdr io.ReadCloser, ct string, cl int64) error {
	if ct == "" {
		ct = "application/octet-stream"
	}
	if cl == 0 {
		cl = -1
	}

	host := rc.getHost(ref.Registry)

	// request an upload location
	uploadURL := url.URL{
		Scheme: host.scheme,
		Host:   host.dnsNames[0],
		Path:   "/v2/" + ref.Repository + "/blobs/uploads/",
	}
	req, err := http.NewRequest("POST", uploadURL.String(), nil)
	if err != nil {
		return fmt.Errorf("Error creating manifest put request: %w", err)
	}
	rty := rc.newRetryableForHost(host)
	resp, err := rty.Req(ctx, rc, req)
	if err != nil {
		return err
	}
	if resp.StatusCode != 202 {
		return fmt.Errorf("Blob upload request did not return a 202 status, status code: %d\nRequest object: %v\nResponse object: %v", resp.StatusCode, req, resp)
	}

	// extract the location into a new putURL based on whether it's relative, fqdn with a scheme, or without a scheme
	location := resp.Header.Get("Location")
	fmt.Fprintf(os.Stderr, "Upload location received: %s", location)
	var putURL *url.URL
	if strings.HasPrefix(location, "/") {
		location = host.scheme + "://" + host.dnsNames[0] + location
	} else if !strings.Contains(location, "://") {
		location = host.scheme + "://" + location
	}
	putURL, err = url.Parse(location)
	if err != nil {
		return err
	}

	// append digest to request to use the monolithic upload option
	if putURL.RawQuery != "" {
		putURL.RawQuery = putURL.RawQuery + "&digest=" + d
	} else {
		putURL.RawQuery = "digest=" + d
	}

	// send the blob
	req, err = http.NewRequest("PUT", putURL.String(), nil)
	if err != nil {
		return fmt.Errorf("Error creating manifest put request: %w", err)
	}
	req.Body = rdr
	req.ContentLength = cl
	req.Header.Set("Content-Type", ct)
	resp, err = rty.Req(ctx, rc, req)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("Blob put request status code: %d\nRequest object: %v\nResponse object: %v", resp.StatusCode, req, resp)
	}

	return nil
}

func (rc *regClient) ImageCopy(ctx context.Context, refSrc Ref, refTgt Ref) error {
	// get the manifest for the source
	m, err := rc.ManifestGet(ctx, refSrc)
	if err != nil {
		return err
	}

	// transfer the config
	cd, err := m.GetConfigDigest()
	if err != nil {
		return err
	}
	if err := rc.BlobCopy(ctx, refSrc, refTgt, cd.String()); err != nil {
		return err
	}

	// for each layer from the source
	l, err := m.GetLayers()
	if err != nil {
		return err
	}
	for _, layerSrc := range l {
		if err := rc.BlobCopy(ctx, refSrc, refTgt, layerSrc.Digest.String()); err != nil {
			return err
		}
	}

	// push manifest to target
	if err := rc.ManifestPut(ctx, refTgt, m); err != nil {
		return err
	}

	return nil
}

func (rc *regClient) ImageExport(ctx context.Context, ref Ref, outStream io.Writer) error {
	if ref.CommonName() == "" {
		return ErrNotFound
	}

	expManifest := dockerTarManifest{}
	expManifest.RepoTags = append(expManifest.RepoTags, ref.CommonName())

	m, err := rc.ManifestGet(ctx, ref)
	if err != nil {
		return err
	}

	// write to a temp directory
	tempDir, err := ioutil.TempDir("", "regcli-export-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	fmt.Fprintf(os.Stderr, "Debug: Using temp directory for export \"%s\"\n", tempDir)

	// retrieve the config blob
	cd, err := m.GetConfigDigest()
	if err != nil {
		return err
	}
	confio, _, err := rc.BlobGet(ctx, ref, cd.String(), []string{MediaTypeDocker2ImageConfig, ociv1.MediaTypeImageConfig})
	if err != nil {
		return err
	}
	confstr, err := ioutil.ReadAll(confio)
	if err != nil {
		return err
	}
	confDigest := digest.FromBytes(confstr)
	if cd != confDigest {
		fmt.Fprintf(os.Stderr, "Warning: digest for image config does not match, pulled %s, calculated %s\n", cd.String(), confDigest.String())
	}
	conf := ociv1.Image{}
	err = json.Unmarshal(confstr, &conf)
	if err != nil {
		return err
	}
	// reset the rootfs DiffIDs and recalculate them as layers are downloaded from the manifest
	// layer digest will change when decompressed and docker load expects layers as tar files
	conf.RootFS.DiffIDs = []digest.Digest{}

	l, err := m.GetLayers()
	if err != nil {
		return err
	}
	for _, layerDesc := range l {
		// TODO: wrap layer download in a concurrency throttled goroutine
		// create tempdir for layer
		layerDir, err := ioutil.TempDir(tempDir, "layer-*")
		if err != nil {
			return err
		}
		// no need to defer remove of layerDir, it is inside of tempDir

		// request layer
		layerRComp, _, err := rc.BlobGet(ctx, ref, layerDesc.Digest.String(), []string{})
		if err != nil {
			return err
		}
		// handle any failures before reading to a file
		defer layerRComp.Close()
		// gather digest of compressed stream to verify downloaded blob
		digestComp := digest.Canonical.Digester()
		trComp := io.TeeReader(layerRComp, digestComp.Hash())
		// decompress layer
		layerTarStream, err := archive.DecompressStream(trComp)
		if err != nil {
			return err
		}
		// generate digest of decompressed layer
		digestTar := digest.Canonical.Digester()
		tr := io.TeeReader(layerTarStream, digestTar.Hash())

		// download to a temp location
		layerTarFile := filepath.Join(layerDir, "layer.tar")
		lf, err := os.OpenFile(layerTarFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return err
		}
		_, err = io.Copy(lf, tr)
		if err != nil {
			return err
		}
		lf.Close()

		// verify digests
		if layerDesc.Digest != digestComp.Digest() {
			fmt.Fprintf(os.Stderr, "Warning: digest for layer does not match, pulled %s, calculated %s\n", layerDesc.Digest.String(), digestComp.Digest().String())
		}

		// update references to uncompressed tar digest in the filesystem, manifest, and image config
		digestFull := digestTar.Digest()
		digestHex := digestFull.Encoded()
		digestDir := filepath.Join(tempDir, digestHex)
		digestFile := filepath.Join(digestHex, "layer.tar")
		digestFileFull := filepath.Join(tempDir, digestFile)
		if err := os.Rename(layerDir, digestDir); err != nil {
			return err
		}
		if err := os.Chtimes(digestFileFull, *conf.Created, *conf.Created); err != nil {
			return err
		}
		expManifest.Layers = append(expManifest.Layers, digestFile)
		conf.RootFS.DiffIDs = append(conf.RootFS.DiffIDs, digestFull)
	}
	// TODO: if using goroutines, wait for all layers to finish

	// calc config digest and write to file
	confstr, err = json.Marshal(conf)
	if err != nil {
		return err
	}
	confDigest = digest.Canonical.FromBytes(confstr)
	confFile := confDigest.Encoded() + ".json"
	confFileFull := filepath.Join(tempDir, confFile)
	if err := ioutil.WriteFile(confFileFull, confstr, 0644); err != nil {
		return err
	}
	if err := os.Chtimes(confFileFull, *conf.Created, *conf.Created); err != nil {
		return err
	}
	expManifest.Config = confFile

	// convert to list and write manifest
	ml := []dockerTarManifest{expManifest}
	mlj, err := json.Marshal(ml)
	if err != nil {
		return err
	}
	manifestFile := filepath.Join(tempDir, "manifest.json")
	if err := ioutil.WriteFile(manifestFile, mlj, 0644); err != nil {
		return err
	}
	if err := os.Chtimes(manifestFile, time.Unix(0, 0), time.Unix(0, 0)); err != nil {
		return err
	}

	// package in tar file
	fs, err := archive.Tar(tempDir, archive.Uncompressed)
	if err != nil {
		return err
	}
	defer fs.Close()

	_, err = io.Copy(outStream, fs)

	return nil
}

func (rc *regClient) ImageInspect(ctx context.Context, ref Ref) (ociv1.Image, error) {
	img := ociv1.Image{}

	m, err := rc.ManifestGet(ctx, ref)
	if err != nil {
		return img, err
	}
	cd, err := m.GetConfigDigest()
	if err != nil {
		return img, err
	}
	imgIO, _, err := rc.BlobGet(ctx, ref, cd.String(), []string{MediaTypeDocker2ImageConfig, ociv1.MediaTypeImageConfig})
	if err != nil {
		return img, err
	}

	imgBody, err := ioutil.ReadAll(imgIO)
	if err != nil {
		return img, err
	}
	// fmt.Printf("Body:\n%s\n", respBody)
	err = json.Unmarshal(imgBody, &img)
	if err != nil {
		return img, err
	}
	return img, nil
}

func (rc *regClient) ManifestGet(ctx context.Context, ref Ref) (Manifest, error) {
	m := manifest{}

	host := rc.getHost(ref.Registry)
	var tagOrDigest string
	if ref.Digest != "" {
		tagOrDigest = ref.Digest
	} else if ref.Tag != "" {
		tagOrDigest = ref.Tag
	} else {
		return nil, ErrMissingTag
	}

	manfURL := url.URL{
		Scheme: host.scheme,
		Host:   host.dnsNames[0],
		Path:   "/v2/" + ref.Repository + "/manifests/" + tagOrDigest,
	}

	req, err := http.NewRequest("GET", manfURL.String(), nil)
	if err != nil {
		return nil, err
	}
	// accept either the manifest or manifest list (index in OCI terms)
	req.Header.Add("Accept", MediaTypeDocker2Manifest)
	req.Header.Add("Accept", ociv1.MediaTypeImageManifest)
	req.Header.Add("Accept", MediaTypeDocker2ManifestList)
	req.Header.Add("Accept", ociv1.MediaTypeImageIndex)

	rty := rc.newRetryableForHost(host)
	resp, err := rty.Req(ctx, rc, req)
	if err != nil {
		return nil, err
	}
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	m.mediatype = resp.Header.Get("Content-Type")
	switch m.mediatype {
	case MediaTypeDocker2Manifest:
		err = json.Unmarshal(respBody, &m.docker)
	case ociv1.MediaTypeImageManifest:
		err = json.Unmarshal(respBody, &m.oci)
	case MediaTypeDocker2ManifestList:
		// TODO
		return nil, fmt.Errorf("Unsupported manifest media type %s", m.mediatype)
	case ociv1.MediaTypeImageIndex:
		err = json.Unmarshal(respBody, &m.ociIndex)
	default:
		return nil, fmt.Errorf("Unknown manifest media type %s", m.mediatype)
	}
	err = json.Unmarshal(respBody, &m)
	if err != nil {
		return nil, err
	}

	return &m, nil
}

/* func (rc *regClient) ManifestListGet(ctx context.Context, ref Ref) (Manifest, error) {
	m := manifest{}

	host := rc.getHost(ref.Registry)
	var tagOrDigest string
	if ref.Digest != "" {
		tagOrDigest = ref.Digest
	} else if ref.Tag != "" {
		tagOrDigest = ref.Tag
	} else {
		return nil, ErrMissingTag
	}

	manfURL := url.URL{
		Scheme: host.scheme,
		Host:   host.dnsNames[0],
		Path:   "/v2/" + ref.Repository + "/manifests/" + tagOrDigest,
	}

	req, err := http.NewRequest("GET", manfURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Accept", MediaTypeDocker2ManifestList)
	req.Header.Add("Accept", ociv1.MediaTypeImageIndex)

	rty := rc.newRetryableForHost(host)
	resp, err := rty.Req(ctx, rc, req)
	if err != nil {
		return nil, err
	}

	// docker will respond for a manifestlist request with a manifest, so check the content type
	ct := resp.Header.Get("Content-Type")
	if ct != MediaTypeDocker2ManifestList && ct != ociv1.MediaTypeImageIndex {
		return nil, ErrNotFound
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(respBody, &ml)
	if err != nil {
		return nil, err
	}

	return ml, nil
}
*/
func (rc *regClient) ManifestPut(ctx context.Context, ref Ref, m Manifest) error {
	host := rc.getHost(ref.Registry)
	if ref.Tag == "" {
		return ErrMissingTag
	}

	manfURL := url.URL{
		Scheme: host.scheme,
		Host:   host.dnsNames[0],
		Path:   "/v2/" + ref.Repository + "/manifests/" + ref.Tag,
	}

	req, err := http.NewRequest("PUT", manfURL.String(), nil)
	if err != nil {
		return fmt.Errorf("Error creating manifest put request: %w", err)
	}

	// add body to request
	var mj []byte
	mt := m.GetMediaType()
	switch mt {
	case MediaTypeDocker2Manifest:
		mj, err = json.Marshal(m.GetDocker())
	case ociv1.MediaTypeImageManifest:
		mj, err = json.Marshal(m.GetOCI())
	case MediaTypeDocker2ManifestList:
		// TODO
		return fmt.Errorf("Unsupported manifest media type %s", mt)
	case ociv1.MediaTypeImageIndex:
		mj, err = json.Marshal(m.GetOCIIndex())
	default:
		return fmt.Errorf("Unknown manifest media type %s", mt)
	}
	if err != nil {
		return err
	}
	req.Body = ioutil.NopCloser(bytes.NewReader(mj))
	req.GetBody = func() (io.ReadCloser, error) {
		return ioutil.NopCloser(bytes.NewReader(mj)), nil
	}
	req.ContentLength = int64(len(mj))
	// req.Header.Set("Content-Type", MediaTypeDocker2Manifest)
	req.Header.Set("Content-Type", mt)

	rty := rc.newRetryableForHost(host)
	resp, err := rty.Req(ctx, rc, req)
	if err != nil {
		return fmt.Errorf("Error calling manifest put request: %w\nRequest object: %v\nResponse object: %v", err, req, resp)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("Unexpected status code on manifest put %d\nRequest object: %v\nResponse object: %v\nBody: %s", resp.StatusCode, req, resp, body)
	}

	return nil
}

func (rc *regClient) TagsList(ctx context.Context, ref Ref) (TagList, error) {
	tl := TagList{}
	host := rc.getHost(ref.Registry)
	repoURL := url.URL{
		Scheme: host.scheme,
		Host:   host.dnsNames[0],
		Path:   "/v2/" + ref.Repository + "/tags/list",
	}

	req, err := http.NewRequest("GET", repoURL.String(), nil)
	if err != nil {
		return tl, err
	}
	rty := rc.newRetryableForHost(host)
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

func (rc *regClient) getHost(hostname string) *regHost {
	host, ok := rc.hosts[hostname]
	if !ok {
		host = &regHost{scheme: "https", tls: tlsEnabled, dnsNames: []string{hostname}}
		rc.hosts[hostname] = host
	}
	return host
}

func (rc *regClient) newRetryableForHost(host *regHost) Retryable {
	if host.transport == nil {
		tlsc := &tls.Config{}
		if host.tls == tlsInsecure {
			tlsc.InsecureSkipVerify = true
		}
		// TODO: update tlsc based on host config for host specific certs and client key/cert pair
		t := &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:       30 * time.Second,
				KeepAlive:     30 * time.Second,
				FallbackDelay: 300 * time.Millisecond,
			}).DialContext,
			MaxIdleConns:          10,
			IdleConnTimeout:       30 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			TLSClientConfig:       tlsc,
			ExpectContinueTimeout: 5 * time.Second,
		}
		host.transport = t
	}
	r := NewRetryable(RetryWithTransport(host.transport), RetryWithLimit(rc.retryLimit))
	return r
}

// TODO: temp hack, grab the proper manifest from github.com/docker/distribution/manifest/schema2
type dockerManifest struct {
	MediaType string `json:"mediaType,omitempty"`
	ociv1.Manifest
}

type manifest struct {
	mediatype string
	oci       ociv1.Manifest
	ociIndex  ociv1.Index
	docker    dockerManifest
	// TODO: include docker manifest list
}

// Manifest abstracts the various types of manifests that are supported
type Manifest interface {
	GetConfigDigest() (digest.Digest, error)
	GetDocker() dockerManifest
	GetLayers() ([]ociv1.Descriptor, error)
	GetMediaType() string
	GetOCI() ociv1.Manifest
	GetOCIIndex() ociv1.Index
}

func (m *manifest) GetConfigDigest() (digest.Digest, error) {
	switch m.mediatype {
	case MediaTypeDocker2Manifest:
		return m.docker.Config.Digest, nil
	case ociv1.MediaTypeImageManifest:
		return m.oci.Config.Digest, nil
	}
	return "", fmt.Errorf("Unsupported manifest mediatype %s", m.mediatype)
}

func (m *manifest) GetDocker() dockerManifest {
	return m.docker
}

func (m *manifest) GetLayers() ([]ociv1.Descriptor, error) {
	switch m.mediatype {
	case MediaTypeDocker2Manifest:
		return m.docker.Layers, nil
	case ociv1.MediaTypeImageManifest:
		return m.oci.Layers, nil
	}
	return []ociv1.Descriptor{}, fmt.Errorf("Unsupported manifest mediatype %s", m.mediatype)
}

func (m *manifest) GetMediaType() string {
	return m.mediatype
}

func (m *manifest) GetOCI() ociv1.Manifest {
	return m.oci
}

func (m *manifest) GetOCIIndex() ociv1.Index {
	return m.ociIndex
}
