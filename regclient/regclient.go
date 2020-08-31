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

	cdauth "github.com/containerd/containerd/remotes/docker"
	dockercfg "github.com/docker/cli/cli/config"
	dockerDistribution "github.com/docker/distribution"
	dockerManifestList "github.com/docker/distribution/manifest/manifestlist"
	dockerSchema2 "github.com/docker/distribution/manifest/schema2"
	"github.com/docker/distribution/reference"
	"github.com/docker/docker/pkg/archive"
	digest "github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sudo-bmitch/regcli/pkg/auth"
	"github.com/sudo-bmitch/regcli/pkg/retryable"
)

var (
	// MediaTypeDocker2Manifest is the media type when pulling manifests from a v2 registry
	MediaTypeDocker2Manifest = dockerSchema2.MediaTypeManifest
	// MediaTypeDocker2ManifestList is the media type when pulling a manifest list from a v2 registry
	MediaTypeDocker2ManifestList = dockerManifestList.MediaTypeManifestList
	// MediaTypeDocker2ImageConfig is for the configuration json object media type
	MediaTypeDocker2ImageConfig = dockerSchema2.MediaTypeImageConfig
	// MediaTypeOCI1Manifest OCI v1 manifest media type
	MediaTypeOCI1Manifest = ociv1.MediaTypeImageManifest
	// MediaTypeOCI1ManifestList OCI v1 manifest list media type
	MediaTypeOCI1ManifestList = ociv1.MediaTypeImageIndex
	// MediaTypeOCI1ImageConfig OCI v1 configuration json object media type
	MediaTypeOCI1ImageConfig = ociv1.MediaTypeImageConfig
)

type tlsConf int

const (
	tlsUndefined tlsConf = iota
	tlsEnabled
	tlsInsecure
	tlsDisabled
)

func (t tlsConf) MarshalJSON() ([]byte, error) {
	s, err := t.MarshalText()
	if err != nil {
		return []byte(""), err
	}
	return json.Marshal(string(s))
}

func (t tlsConf) MarshalText() ([]byte, error) {
	var s string
	switch t {
	default:
		s = ""
	case tlsEnabled:
		s = "enabled"
	case tlsInsecure:
		s = "insecure"
	case tlsDisabled:
		s = "disabled"
	}
	return []byte(s), nil
}

func (t *tlsConf) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	return t.UnmarshalText([]byte(s))
}

func (t *tlsConf) UnmarshalText(b []byte) error {
	switch strings.ToLower(string(b)) {
	default:
		return fmt.Errorf("Unknown TLS value \"%s\"", b)
	case "":
		*t = tlsUndefined
	case "enabled":
		*t = tlsEnabled
	case "insecure":
		*t = tlsInsecure
	case "disabled":
		*t = tlsDisabled
	}
	return nil
}

// RegClient provides an interfaces to working with registries
type RegClient interface {
	Config() Config
	AddResp(ctx context.Context, resps []*http.Response) error
	AuthReq(ctx context.Context, req *http.Request) error
	BlobGet(ctx context.Context, ref Ref, d string, accepts []string) (io.ReadCloser, *http.Response, error)
	ImageCopy(ctx context.Context, refSrc Ref, refTgt Ref) error
	ImageExport(ctx context.Context, ref Ref, outStream io.Writer) error
	ImageInspect(ctx context.Context, ref Ref) (ociv1.Image, error)
	ManifestDigest(ctx context.Context, ref Ref) (digest.Digest, error)
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
	// hosts      map[string]*regHost
	// auth       AuthClient
	config     *Config
	retryLimit int
	transports map[string]*http.Transport
	authorizer cdauth.Authorizer
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
	/* 	rc.hosts = map[string]*regHost{"docker.io": {scheme: "https", tls: tlsEnabled, dnsNames: []string{"registry-1.docker.io"}}}
	   	rc.auth = NewAuthClient() */
	rc.retryLimit = 5
	rc.transports = map[string]*http.Transport{}
	rc.newAuth()

	for _, opt := range opts {
		opt(&rc)
	}

	if rc.config == nil {
		rc.config = ConfigNew()
	}

	// hard code docker hub host config
	// TODO: change to a global var? merge ConfigHost structs?
	if _, ok := rc.config.Hosts["docker.io"]; !ok {
		rc.config.Hosts["docker.io"] = &ConfigHost{}
	}
	rc.config.Hosts["docker.io"].Scheme = "https"
	rc.config.Hosts["docker.io"].TLS = tlsEnabled
	rc.config.Hosts["docker.io"].DNS = []string{"registry-1.docker.io"}

	return &rc
}

// WithConfigDefault default config file
func WithConfigDefault() Opt {
	return func(rc *regClient) {
		config, err := ConfigLoadDefault()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load default config: %s\n", err)
		} else {
			rc.config = config
		}
	}
}

// WithConfigFile parses a differently named config file
func WithConfigFile(filename string) Opt {
	return func(rc *regClient) {
		config, err := ConfigLoadFile(filename)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load config file %s: %s\n", filename, err)
		} else {
			rc.config = config
		}
	}
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
		if rc.config == nil {
			rc.config = ConfigNew()
		}
		conffile := dockercfg.LoadDefaultConfigFile(os.Stderr)
		creds, err := conffile.GetAllCredentials()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load docker creds %s\n", err)
			return
		}
		for _, cred := range creds {
			// TODO: remove rc.auth
			/* 			if cred.ServerAddress == "https://index.docker.io/v1/" && cred.Username != "" && cred.Password != "" {
			   				rc.auth.Set("registry-1.docker.io", cred.Username, cred.Password)
			   			} else if cred.ServerAddress != "" && cred.Username != "" && cred.Password != "" {
			   				rc.auth.Set(cred.ServerAddress, cred.Username, cred.Password)
			   			} */

			if cred.ServerAddress == "" || cred.Username == "" || cred.Password == "" {
				continue
			}
			// TODO: move these hostnames into a const (possibly pull from distribution repo)
			if cred.ServerAddress == "https://index.docker.io/v1/" {
				cred.ServerAddress = "registry-1.docker.io"
			}
			if _, ok := rc.config.Hosts[cred.ServerAddress]; !ok {
				h := ConfigHost{
					DNS:    []string{cred.ServerAddress},
					Scheme: "https",
					TLS:    tlsEnabled,
					User:   cred.Username,
					Pass:   cred.Password,
				}
				rc.config.Hosts[cred.ServerAddress] = &h
			} else if rc.config.Hosts[cred.ServerAddress].User != "" || rc.config.Hosts[cred.ServerAddress].Pass != "" {
				if rc.config.Hosts[cred.ServerAddress].User != cred.Username || rc.config.Hosts[cred.ServerAddress].Pass != cred.Password {
					fmt.Fprintf(os.Stderr, "Warning: credentials in docker do not match regcli credentials for registry %s\n", cred.ServerAddress)
				}
			} else {
				rc.config.Hosts[cred.ServerAddress].User = cred.Username
				rc.config.Hosts[cred.ServerAddress].Pass = cred.Password
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

/* func (rc *regClient) Auth() AuthClient {
	return rc.auth
}
*/
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
		Scheme: host.Scheme,
		Host:   host.DNS[0],
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
		Scheme: host.Scheme,
		Host:   host.DNS[0],
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
		Scheme:   host.Scheme,
		Host:     host.DNS[0],
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
		Scheme: host.Scheme,
		Host:   host.DNS[0],
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
		location = host.Scheme + "://" + host.DNS[0] + location
	} else if !strings.Contains(location, "://") {
		location = host.Scheme + "://" + location
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

func (rc *regClient) Config() Config {
	return *rc.config
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

func (rc *regClient) ManifestDigest(ctx context.Context, ref Ref) (digest.Digest, error) {
	host := rc.getHost(ref.Registry)
	var tagOrDigest string
	if ref.Digest != "" {
		tagOrDigest = ref.Digest
	} else if ref.Tag != "" {
		tagOrDigest = ref.Tag
	} else {
		return "", ErrMissingTag
	}

	manfURL := url.URL{
		Scheme: host.Scheme,
		Host:   host.DNS[0],
		Path:   "/v2/" + ref.Repository + "/manifests/" + tagOrDigest,
	}

	req, err := http.NewRequest("GET", manfURL.String(), nil)
	if err != nil {
		return "", err
	}
	// accept either the manifest or manifest list (index in OCI terms)
	req.Header.Add("Accept", MediaTypeDocker2Manifest)
	req.Header.Add("Accept", MediaTypeDocker2ManifestList)
	req.Header.Add("Accept", MediaTypeOCI1Manifest)
	req.Header.Add("Accept", MediaTypeOCI1ManifestList)

	rty := rc.newRetryableForHost(host)
	resp, err := rty.Req(ctx, rc, req)
	if err != nil {
		return "", err
	}
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return digest.FromBytes(respBody), nil
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
		Scheme: host.Scheme,
		Host:   host.DNS[0],
		Path:   "/v2/" + ref.Repository + "/manifests/" + tagOrDigest,
	}

	fmt.Fprintf(os.Stderr, "Debug: url is %s\n", manfURL.String())
	/* 	req, err := http.NewRequest("GET", manfURL.String(), nil)
	   	if err != nil {
	   		return nil, err
	   	}
	   	// accept either the manifest or manifest list (index in OCI terms)
	   	req.Header.Add("Accept", MediaTypeDocker2Manifest)
	   	req.Header.Add("Accept", MediaTypeDocker2ManifestList)
	   	req.Header.Add("Accept", MediaTypeOCI1Manifest)
	   	req.Header.Add("Accept", MediaTypeOCI1ManifestList)
	*/
	opts := []retryable.OptsReq{}
	opts = append(opts, retryable.WithHeader("Accept", []string{
		MediaTypeDocker2Manifest,
		MediaTypeDocker2ManifestList,
		MediaTypeOCI1Manifest,
		MediaTypeOCI1ManifestList,
	}))

	rty := rc.newRetryableHost(host)
	resp, err := rty.DoRequest(ctx, "GET", manfURL, opts...)
	if err != nil {
		return nil, err
	}
	respBody, err := ioutil.ReadAll(resp)
	if err != nil {
		return nil, err
	}
	m.mt = resp.HTTPResponse().Header.Get("Content-Type")
	switch m.mt {
	case MediaTypeDocker2Manifest:
		err = json.Unmarshal(respBody, &m.dockerM)
	case MediaTypeDocker2ManifestList:
		err = json.Unmarshal(respBody, &m.dockerML)
	case MediaTypeOCI1Manifest:
		err = json.Unmarshal(respBody, &m.ociM)
	case MediaTypeOCI1ManifestList:
		err = json.Unmarshal(respBody, &m.ociML)
	default:
		return nil, fmt.Errorf("Unknown manifest media type %s", m.mt)
	}
	err = json.Unmarshal(respBody, &m)
	if err != nil {
		return nil, err
	}

	return &m, nil
}

func (rc *regClient) ManifestPut(ctx context.Context, ref Ref, m Manifest) error {
	host := rc.getHost(ref.Registry)
	if ref.Tag == "" {
		return ErrMissingTag
	}

	manfURL := url.URL{
		Scheme: host.Scheme,
		Host:   host.DNS[0],
		Path:   "/v2/" + ref.Repository + "/manifests/" + ref.Tag,
	}

	req, err := http.NewRequest("PUT", manfURL.String(), nil)
	if err != nil {
		return fmt.Errorf("Error creating manifest put request: %w", err)
	}

	// add body to request
	var mj []byte
	mt := m.GetMediaType()
	mj, err = json.Marshal(m)
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
		Scheme: host.Scheme,
		Host:   host.DNS[0],
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

func (rc *regClient) getHost(hostname string) *ConfigHost {
	host, ok := rc.config.Hosts[hostname]
	if !ok {
		host = &ConfigHost{Scheme: "https", TLS: tlsEnabled, DNS: []string{hostname}}
		rc.config.Hosts[hostname] = host
	}
	return host
}

// TODO: rework, retryable should fall back to other DNS names, perhaps pass the transport map and host object
func (rc *regClient) newRetryableForHost(host *ConfigHost) Retryable {
	if _, ok := rc.transports[host.DNS[0]]; !ok {
		tlsc := &tls.Config{}
		if host.TLS == tlsInsecure {
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
		rc.transports[host.DNS[0]] = t
	}
	r := NewRetryable(RetryWithTransport(rc.transports[host.DNS[0]]), RetryWithLimit(rc.retryLimit))
	return r
}

func (rc *regClient) newRetryableHost(host *ConfigHost) retryable.Retryable {
	// TODO: include creds func
	a := auth.NewAuth(auth.WithCreds(rc.authCreds))
	r := retryable.NewRetryable(retryable.WithAuth(a))
	return r
}

func (rc *regClient) authCreds(host string) (string, string) {
	if h, ok := rc.config.Hosts[host]; ok {
		return h.User, h.Pass
	}
	// default credentials are stored under a blank hostname
	if h, ok := rc.config.Hosts[""]; ok {
		return h.User, h.Pass
	}
	fmt.Fprintf(os.Stderr, "No credentials found for %s\n", host)
	// anonymous request
	return "", ""
}

// TODO: temp hack, grab the proper manifest from github.com/docker/distribution/manifest/schema2
type dockerManifest struct {
	MediaType string `json:"mediaType,omitempty"`
	ociv1.Manifest
}

type manifest struct {
	dockerM  dockerSchema2.Manifest
	dockerML dockerManifestList.ManifestList
	mt       string
	ociM     ociv1.Manifest
	ociML    ociv1.Index
}

// Manifest abstracts the various types of manifests that are supported
type Manifest interface {
	GetConfigDigest() (digest.Digest, error)
	GetDockerManifest() dockerSchema2.Manifest
	GetDockerManifestList() dockerManifestList.ManifestList
	GetLayers() ([]ociv1.Descriptor, error)
	GetMediaType() string
	GetOCIManifest() ociv1.Manifest
	GetOCIManifestList() ociv1.Index
	MarshalJSON() ([]byte, error)
}

func (m *manifest) GetConfigDigest() (digest.Digest, error) {
	switch m.mt {
	case MediaTypeDocker2Manifest:
		return m.dockerM.Config.Digest, nil
	case ociv1.MediaTypeImageManifest:
		return m.ociM.Config.Digest, nil
	}
	// TODO: find config for current OS type?
	return "", fmt.Errorf("Unsupported manifest mediatype %s", m.mt)
}

func (m *manifest) GetDockerManifest() dockerSchema2.Manifest {
	return m.dockerM
}

func (m *manifest) GetDockerManifestList() dockerManifestList.ManifestList {
	return m.dockerML
}

func (m *manifest) GetLayers() ([]ociv1.Descriptor, error) {
	switch m.mt {
	case MediaTypeDocker2Manifest:
		return d2oDescriptorList(m.dockerM.Layers), nil
	case ociv1.MediaTypeImageManifest:
		return m.ociM.Layers, nil
	}
	return []ociv1.Descriptor{}, fmt.Errorf("Unsupported manifest mediatype %s", m.mt)
}

func d2oDescriptorList(src []dockerDistribution.Descriptor) []ociv1.Descriptor {
	var tgt []ociv1.Descriptor
	for _, sd := range src {
		td := ociv1.Descriptor{
			MediaType:   sd.MediaType,
			Digest:      sd.Digest,
			Size:        sd.Size,
			URLs:        sd.URLs,
			Annotations: sd.Annotations,
			Platform:    sd.Platform,
		}
		tgt = append(tgt, td)
	}
	return tgt
}

func (m *manifest) GetMediaType() string {
	return m.mt
}

func (m *manifest) GetOCIManifest() ociv1.Manifest {
	return m.ociM
}

func (m *manifest) GetOCIManifestList() ociv1.Index {
	return m.ociML
}

func (m *manifest) MarshalJSON() ([]byte, error) {
	switch m.mt {
	case MediaTypeDocker2Manifest:
		return json.Marshal(m.dockerM)
	case MediaTypeDocker2ManifestList:
		return json.Marshal(m.dockerML)
	case MediaTypeOCI1Manifest:
		return json.Marshal(m.ociM)
	case MediaTypeOCI1ManifestList:
		return json.Marshal(m.ociML)
	}
	return []byte{}, ErrUnsupportedMediaType
}
