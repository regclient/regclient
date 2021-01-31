package regclient

import (
	"io/ioutil"
	"path/filepath"
	"strings"
	"sync"

	// crypto libraries included for go-digest
	_ "crypto/sha256"
	_ "crypto/sha512"
	"fmt"
	"net/http"
	"net/url"
	"os"

	dockercfg "github.com/docker/cli/cli/config"
	dockerManifestList "github.com/docker/distribution/manifest/manifestlist"
	dockerSchema2 "github.com/docker/distribution/manifest/schema2"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/regclient/regclient/pkg/auth"
	"github.com/regclient/regclient/pkg/retryable"
	"github.com/sirupsen/logrus"
)

const (
	// DefaultUserAgent sets the header on http requests
	DefaultUserAgent = "regclient/regclient"
	// DockerCertDir default location for docker certs
	DockerCertDir = "/etc/docker/certs.d"
	// DockerRegistry is the name resolved in docker images on Hub
	DockerRegistry = "docker.io"
	// DockerRegistryAuth is the name provided in docker's config for Hub
	DockerRegistryAuth = "https://index.docker.io/v1/"
	// DockerRegistryDNS is the host to connect to for Hub
	DockerRegistryDNS = "registry-1.docker.io"
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

var (
	// VCSRef is injected from a build flag, used to version the UserAgent header
	VCSRef = "unknown"
)

// RegClient provides an interfaces to working with registries
type RegClient interface {
	RepoClient
	TagClient
	ManifestClient
	ImageClient
	BlobClient
}

// RateLimit is returned from some http requests
type RateLimit struct {
	Remain, Limit, Reset int
	Set                  bool
	Policies             []string
}

type regClient struct {
	config     *Config
	log        *logrus.Logger
	retryLimit int
	transports map[string]*http.Transport
	retryables map[string]retryable.Retryable
	mu         sync.Mutex
	useragent  string
}

// Opt functions are used to configure NewRegClient
type Opt func(*regClient)

// NewRegClient returns a registry client
func NewRegClient(opts ...Opt) RegClient {
	var rc regClient

	rc.retryLimit = 5
	rc.retryables = map[string]retryable.Retryable{}
	rc.transports = map[string]*http.Transport{}
	rc.useragent = DefaultUserAgent + " (" + VCSRef + ")"

	// logging is disabled by default
	rc.log = &logrus.Logger{
		Out: ioutil.Discard,
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
	rc.config.Hosts[DockerRegistry].TLS = TLSEnabled
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

// WithConfigHosts adds a list of config host settings
func WithConfigHosts(configHosts []ConfigHost) Opt {
	return func(rc *regClient) {
		if configHosts == nil || len(configHosts) == 0 {
			return
		}
		if rc.config == nil {
			rc.config = ConfigNew()
		}
		for i := range configHosts {
			configHost := configHosts[i]
			if configHost.Name == "" {
				continue
			}
			if configHost.Name == DockerRegistry || configHost.Name == DockerRegistryAuth {
				configHost.Name = DockerRegistryDNS
			}
			// merge updated host with original values
			if orig, ok := rc.config.Hosts[configHost.Name]; ok {
				if configHost.User == "" || configHost.Pass == "" {
					configHost.User = orig.User
					configHost.Pass = orig.Pass
				}
				if configHost.RegCert == "" {
					configHost.RegCert = orig.RegCert
				}
				if configHost.Scheme == "" {
					configHost.Scheme = orig.Scheme
				}
				if configHost.TLS == TLSUndefined {
					configHost.TLS = orig.TLS
				}
				if len(configHost.DNS) == 0 {
					configHost.DNS = orig.DNS
				}
			}
			if configHost.Scheme == "" {
				configHost.Scheme = "https"
			}
			if configHost.TLS == TLSUndefined {
				configHost.TLS = TLSEnabled
			}
			if len(configHost.DNS) == 0 {
				configHost.DNS = []string{configHost.Name}
			}
			rc.config.Hosts[configHost.Name] = &configHost
		}
		return
	}
}

// WithConfigHost adds config host settings
func WithConfigHost(configHost ConfigHost) Opt {
	return WithConfigHosts([]ConfigHost{configHost})
}

// WithLog overrides default logrus Logger
func WithLog(log *logrus.Logger) Opt {
	return func(rc *regClient) {
		rc.log = log
	}
}

// WithUserAgent specifies the User-Agent http header
func WithUserAgent(ua string) Opt {
	return func(rc *regClient) {
		rc.useragent = ua
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
				TLS:    TLSEnabled,
				User:   cred.Username,
				Pass:   cred.Password,
			}
			rc.config.Hosts[cred.ServerAddress] = &h
		} else if rc.config.Hosts[cred.ServerAddress].User != "" || rc.config.Hosts[cred.ServerAddress].Pass != "" {
			if rc.config.Hosts[cred.ServerAddress].User != cred.Username || rc.config.Hosts[cred.ServerAddress].Pass != cred.Password {
				rc.log.WithFields(logrus.Fields{
					"registry":    cred.ServerAddress,
					"docker-user": cred.Username,
					"config-user": rc.config.Hosts[cred.ServerAddress].User,
					"pass-same":   (cred.Password == rc.config.Hosts[cred.ServerAddress].Pass),
				}).Warn("Docker credentials mismatch")
			}
		} else {
			rc.config.Hosts[cred.ServerAddress].User = cred.Username
			rc.config.Hosts[cred.ServerAddress].Pass = cred.Password
		}
	}
	return nil
}

func (rc *regClient) getHost(hostname string) *ConfigHost {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	host, ok := rc.config.Hosts[hostname]
	if !ok {
		host = &ConfigHost{Scheme: "https", TLS: TLSEnabled, DNS: []string{hostname}}
		rc.config.Hosts[hostname] = host
	}
	return host
}

func (rc *regClient) getRetryable(host *ConfigHost) retryable.Retryable {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if _, ok := rc.retryables[host.Name]; !ok {
		c := &http.Client{}
		a := auth.NewAuth(auth.WithLog(rc.log), auth.WithHTTPClient(c), auth.WithCreds(rc.authCreds))
		rOpts := []retryable.Opts{
			retryable.WithLog(rc.log),
			retryable.WithHTTPClient(c),
			retryable.WithAuth(a),
			retryable.WithMirrors(rc.mirrorFunc(host)),
			retryable.WithUserAgent(rc.useragent),
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
