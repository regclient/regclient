package regclient

import (
	"context"
	"strings"

	// crypto libraries included for go-digest
	_ "crypto/sha256"
	_ "crypto/sha512"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	dockercfg "github.com/docker/cli/cli/config"
	dockerManifestList "github.com/docker/distribution/manifest/manifestlist"
	dockerSchema2 "github.com/docker/distribution/manifest/schema2"
	"github.com/docker/distribution/reference"
	digest "github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
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

	// hard code docker hub host config
	// TODO: change to a global var? merge ConfigHost structs?
	if _, ok := rc.config.Hosts["docker.io"]; !ok {
		rc.config.Hosts["docker.io"] = &ConfigHost{}
	}
	rc.config.Hosts["docker.io"].Name = "docker.io"
	rc.config.Hosts["docker.io"].Scheme = "https"
	rc.config.Hosts["docker.io"].TLS = tlsEnabled
	rc.config.Hosts["docker.io"].DNS = []string{"registry-1.docker.io"}

	rc.log.Debug("regclient initialized")

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
			if cred.ServerAddress == "" || cred.Username == "" || cred.Password == "" {
				continue
			}
			// TODO: move these hostnames into a const (possibly pull from distribution repo)
			if cred.ServerAddress == "https://index.docker.io/v1/" {
				cred.ServerAddress = "registry-1.docker.io"
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
		return
	}
}

// WithLog overrides default logrus Logger
func WithLog(log *logrus.Logger) Opt {
	return func(rc *regClient) {
		rc.log = log
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
		a := auth.NewAuth(auth.WithLog(rc.log), auth.WithCreds(rc.authCreds))
		r := retryable.NewRetryable(retryable.WithLog(rc.log), retryable.WithAuth(a))
		rc.retryables[host.Name] = r
	}
	return rc.retryables[host.Name]
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
