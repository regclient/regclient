package regclient

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"strings"

	// crypto libraries included for go-digest
	_ "crypto/sha256"
	_ "crypto/sha512"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	dockercfg "github.com/docker/cli/cli/config"
	dockerManifestList "github.com/docker/distribution/manifest/manifestlist"
	dockerSchema2 "github.com/docker/distribution/manifest/schema2"
	"github.com/docker/distribution/reference"
	digest "github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient/pkg/auth"
	"github.com/regclient/regclient/pkg/retryable"
	"github.com/sirupsen/logrus"
)

const (
	// DockerCertDir default location for docker certs
	DockerCertDir = "/etc/docker/certs.d"
	// DockerRegistry is the name resolved in docker images on Hub
	DockerRegistry = "docker.io"
	// DockerRegistryAuth is the name provided in docker's config for Hub
	DockerRegistryAuth = "https://index.docker.io/v1/"
	// DockerRegistryDNS is the host to connect to for Hub
	DockerRegistryDNS = "registry-1.docker.io"
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

// RegClient provides an interfaces to working with registries
type RegClient interface {
	Config() Config
	BlobGet(ctx context.Context, ref Ref, d string, accepts []string) (io.ReadCloser, *http.Response, error)
	ImageCopy(ctx context.Context, refSrc Ref, refTgt Ref) error
	ImageExport(ctx context.Context, ref Ref, outStream io.Writer) error
	ImageGetConfig(ctx context.Context, ref Ref, d string) (ociv1.Image, error)
	ManifestDelete(ctx context.Context, ref Ref) error
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
	log        *logrus.Logger
	retryLimit int
	transports map[string]*http.Transport
	retryables map[string]retryable.Retryable
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

	rc.retryLimit = 5
	rc.retryables = map[string]retryable.Retryable{}
	rc.transports = map[string]*http.Transport{}

	// configure default logging
	rc.log = &logrus.Logger{
		Out:       os.Stderr,
		Formatter: new(logrus.TextFormatter),
		Hooks:     make(logrus.LevelHooks),
		Level:     logrus.WarnLevel,
	}

	for _, opt := range opts {
		opt(&rc)
	}

	if rc.config == nil {
		rc.config = ConfigNew()
	}

	// inject Docker Hub settings
	if _, ok := rc.config.Hosts[DockerRegistry]; !ok {
		rc.config.Hosts[DockerRegistry] = &ConfigHost{}
	}
	rc.config.Hosts[DockerRegistry].Name = DockerRegistry
	rc.config.Hosts[DockerRegistry].Scheme = "https"
	rc.config.Hosts[DockerRegistry].TLS = tlsEnabled
	rc.config.Hosts[DockerRegistry].DNS = []string{DockerRegistryDNS}

	// load docker creds/certs if configured
	if rc.config.IncDockerCred != nil && *rc.config.IncDockerCred == true {
		if err := rc.loadDockerCreds(); err != nil {
			rc.log.WithFields(logrus.Fields{
				"err": err,
			}).Warn("Failed to load docker creds")
		} else {
			rc.log.Debug("Docker creds loaded")
		}
	}

	rc.log.Debug("regclient initialized")

	return &rc
}

// WithConfigDefault default config file
func WithConfigDefault() Opt {
	return func(rc *regClient) {
		config, err := ConfigLoadDefault()
		if err != nil {
			rc.log.WithFields(logrus.Fields{
				"err": err,
			}).Warn("Failed to load default config")
		} else {
			if rc.config != nil {
				rc.log.WithFields(logrus.Fields{
					"oldConfig": rc.config,
					"newConfig": config,
				}).Warn("Overwriting existing config")
			} else {
				rc.log.WithFields(logrus.Fields{
					"config": config,
				}).Debug("Loaded default config")
			}
			rc.config = config
		}
	}
}

// WithConfigFile parses a differently named config file
func WithConfigFile(filename string) Opt {
	return func(rc *regClient) {
		config, err := ConfigLoadFile(filename)
		if err != nil {
			rc.log.WithFields(logrus.Fields{
				"err":  err,
				"file": filename,
			}).Warn("Failed to load config")
		} else {
			rc.config = config
		}
	}
}

// WithDockerCerts adds certificates trusted by docker in /etc/docker/certs.d
func WithDockerCerts() Opt {
	return func(rc *regClient) {
		if rc.config == nil {
			rc.config = ConfigNew()
		}
		if rc.config.IncDockerCert == nil {
			enabled := true
			rc.config.IncDockerCert = &enabled
		}
		return
	}
}

// WithDockerCreds adds configuration from users docker config with registry logins
// This changes the default value from the config file, and should be added after the config file is loaded
func WithDockerCreds() Opt {
	return func(rc *regClient) {
		if rc.config == nil {
			rc.config = ConfigNew()
		}
		if rc.config.IncDockerCred == nil {
			enabled := true
			rc.config.IncDockerCred = &enabled
		}
		return
	}
}

// WithLog overrides default logrus Logger
func WithLog(log *logrus.Logger) Opt {
	return func(rc *regClient) {
		rc.log = log
	}
}

func (rc *regClient) loadDockerCreds() error {
	if rc.config == nil {
		rc.config = ConfigNew()
	}
	conffile := dockercfg.LoadDefaultConfigFile(os.Stderr)
	creds, err := conffile.GetAllCredentials()
	if err != nil {
		return fmt.Errorf("Failed to load docker creds %s", err)
	}
	for _, cred := range creds {
		rc.log.WithFields(logrus.Fields{
			"host": cred.ServerAddress,
			"user": cred.Username,
		}).Debug("Loading docker cred")
		if cred.ServerAddress == "" || cred.Username == "" || cred.Password == "" {
			continue
		}
		// Docker Hub is a special case
		if cred.ServerAddress == DockerRegistryAuth {
			cred.ServerAddress = DockerRegistryDNS
		}
		if _, ok := rc.config.Hosts[cred.ServerAddress]; !ok {
			h := ConfigHost{
				Name:   cred.ServerAddress,
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
	return nil
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

	if ret.Tag == "" && ret.Digest == "" {
		ret.Tag = "latest"
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

func (rc *regClient) Config() Config {
	return *rc.config
}

func (rc *regClient) getHost(hostname string) *ConfigHost {
	host, ok := rc.config.Hosts[hostname]
	if !ok {
		host = &ConfigHost{Scheme: "https", TLS: tlsEnabled, DNS: []string{hostname}}
		rc.config.Hosts[hostname] = host
	}
	return host
}

func (rc *regClient) getRetryable(host *ConfigHost) retryable.Retryable {
	if _, ok := rc.retryables[host.Name]; !ok {
		c := &http.Client{}
		a := auth.NewAuth(auth.WithLog(rc.log), auth.WithHTTPClient(c), auth.WithCreds(rc.authCreds))
		rOpts := []retryable.Opts{
			retryable.WithLog(rc.log),
			retryable.WithHTTPClient(c),
			retryable.WithAuth(a),
			retryable.WithMirrors(rc.mirrorFunc(host)),
		}
		if certs := rc.getCerts(host); len(certs) > 0 {
			rOpts = append(rOpts, retryable.WithCertFiles(certs))
		}
		if host.RegCert != "" {
			rOpts = append(rOpts, retryable.WithCerts([][]byte{[]byte(host.RegCert)}))
		}
		r := retryable.NewRetryable(rOpts...)
		rc.retryables[host.Name] = r
	}
	return rc.retryables[host.Name]
}

func (rc *regClient) authCreds(host string) (string, string) {
	if h, ok := rc.config.Hosts[host]; ok {
		rc.log.WithFields(logrus.Fields{
			"host": host,
			"user": h.User,
		}).Debug("Retrieved cred")
		return h.User, h.Pass
	}
	// default credentials are stored under a blank hostname
	if h, ok := rc.config.Hosts[""]; ok {
		return h.User, h.Pass
	}
	// anonymous request
	rc.log.WithFields(logrus.Fields{
		"host": host,
	}).Debug("No credentials found, defaulting to anonymous")
	return "", ""
}

func (rc *regClient) getCerts(host *ConfigHost) []string {
	var certs []string
	if rc.config.IncDockerCert == nil || *rc.config.IncDockerCert == false {
		return certs
	}
	hosts := []string{host.Name}
	if host.DNS != nil {
		hosts = host.DNS
	}
	for _, h := range hosts {
		dir := filepath.Join(DockerCertDir, h)
		files, err := ioutil.ReadDir(dir)
		if err != nil {
			if !os.IsNotExist(err) {
				rc.log.WithFields(logrus.Fields{
					"err": err,
					"dir": dir,
				}).Warn("Failed to open docker cert dir")
			}
			continue
		}
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			if strings.HasSuffix(f.Name(), ".crt") {
				certs = append(certs, filepath.Join(dir, f.Name()))
			}
		}
	}
	return certs
}

func (rc *regClient) mirrorFunc(host *ConfigHost) func(url.URL) ([]url.URL, error) {
	return func(u url.URL) ([]url.URL, error) {
		var ul []url.URL
		for _, m := range host.DNS {
			mu := u
			mu.Host = m
			ul = append(ul, mu)
		}
		return ul, nil
	}
}
