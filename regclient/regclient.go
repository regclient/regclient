package regclient

import (
	"io/ioutil"
	"path/filepath"
	"strings"
	"sync"
	"time"

	// crypto libraries included for go-digest
	_ "crypto/sha256"
	_ "crypto/sha512"
	"fmt"
	"net/http"
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
	// DefaultRetryLimit sets how many retry attempts are made for non-fatal errors
	DefaultRetryLimit = 3
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
	// MediaTypeDocker1Manifest deprecated media type for docker schema1 manifests
	MediaTypeDocker1Manifest = "application/vnd.docker.distribution.manifest.v1+json"
	// MediaTypeDocker1ManifestSigned is a deprecated schema1 manifest with jws signing
	MediaTypeDocker1ManifestSigned = "application/vnd.docker.distribution.manifest.v1+prettyjws"
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
	// MediaTypeDocker2Layer is the default compressed layer for docker schema2
	MediaTypeDocker2Layer = dockerSchema2.MediaTypeLayer
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

type regClient struct {
	certPaths      []string
	hosts          map[string]*regClientHost
	log            *logrus.Logger
	retryLimit     int
	retryDelayInit time.Duration
	retryDelayMax  time.Duration
	blobChunkSize  int64
	blobMaxPut     int64
	mu             sync.Mutex
	userAgent      string
}

type regClientHost struct {
	config    *ConfigHost
	retryable retryable.Retryable
}

// Opt functions are used to configure NewRegClient
type Opt func(*regClient)

// NewRegClient returns a registry client
func NewRegClient(opts ...Opt) RegClient {
	var rc = regClient{
		certPaths:     []string{},
		hosts:         map[string]*regClientHost{},
		retryLimit:    DefaultRetryLimit,
		userAgent:     DefaultUserAgent,
		blobChunkSize: 1024 * 1024,       // 1M chunks, this is allocated in a memory buffer
		blobMaxPut:    100 * 1024 * 1024, // 100M, switch to chunked above this threshold to avoid timeouts
		// logging is disabled by default
		log: &logrus.Logger{Out: ioutil.Discard},
	}

	// inject Docker Hub settings
	rc.hostSet(ConfigHost{
		Name:     DockerRegistry,
		TLS:      TLSEnabled,
		Hostname: DockerRegistryDNS,
	})

	for _, opt := range opts {
		opt(&rc)
	}

	rc.log.Debug("regclient initialized")

	return &rc
}

// WithCertDir adds a path of certificates to trust similar to Docker's /etc/docker/certs.d
func WithCertDir(path string) Opt {
	return func(rc *regClient) {
		rc.certPaths = append(rc.certPaths, path)
		return
	}
}

// WithDockerCerts adds certificates trusted by docker in /etc/docker/certs.d
func WithDockerCerts() Opt {
	return func(rc *regClient) {
		rc.certPaths = append(rc.certPaths, DockerCertDir)
		return
	}
}

// WithDockerCreds adds configuration from users docker config with registry logins
// This changes the default value from the config file, and should be added after the config file is loaded
func WithDockerCreds() Opt {
	return func(rc *regClient) {
		err := rc.loadDockerCreds()
		if err != nil {
			rc.log.WithFields(logrus.Fields{
				"err": err,
			}).Warn("Failed to load docker creds")
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
		for _, configHost := range configHosts {
			if configHost.Name == "" {
				continue
			}
			if configHost.Name == DockerRegistry || configHost.Name == DockerRegistryDNS || configHost.Name == DockerRegistryAuth {
				configHost.Name = DockerRegistry
				if configHost.Hostname == "" || configHost.Hostname == DockerRegistry || configHost.Hostname == DockerRegistryAuth {
					configHost.Hostname = DockerRegistryDNS
				}
			}
			tls, _ := configHost.TLS.MarshalText()
			rc.log.WithFields(logrus.Fields{
				"name":       configHost.Name,
				"user":       configHost.User,
				"hostname":   configHost.Hostname,
				"tls":        string(tls),
				"pathPrefix": configHost.PathPrefix,
				"mirrors":    configHost.Mirrors,
				"api":        configHost.API,
			}).Debug("Loading host config")
			err := rc.hostSet(configHost)
			if err != nil {
				rc.log.WithFields(logrus.Fields{
					"host":  configHost.Name,
					"user":  configHost.User,
					"error": err,
				}).Warn("Failed to update host config")
			}
		}
		return
	}
}

// WithConfigHost adds config host settings
func WithConfigHost(configHost ConfigHost) Opt {
	return WithConfigHosts([]ConfigHost{configHost})
}

// WithBlobSize overrides default blob sizes
func WithBlobSize(chunk, max int64) Opt {
	return func(rc *regClient) {
		if chunk > 0 {
			rc.blobChunkSize = chunk
		}
		if max > 0 {
			rc.blobMaxPut = max
		}
	}
}

// WithLog overrides default logrus Logger
func WithLog(log *logrus.Logger) Opt {
	return func(rc *regClient) {
		rc.log = log
	}
}

// WithRetryDelay specifies the time permitted for retry delays
func WithRetryDelay(delayInit, delayMax time.Duration) Opt {
	return func(rc *regClient) {
		rc.retryDelayInit = delayInit
		rc.retryDelayMax = delayMax
	}
}

// WithRetryLimit specifies the number of retries for non-fatal errors
func WithRetryLimit(retryLimit int) Opt {
	return func(rc *regClient) {
		rc.retryLimit = retryLimit
	}
}

// WithUserAgent specifies the User-Agent http header
func WithUserAgent(ua string) Opt {
	return func(rc *regClient) {
		rc.userAgent = ua
	}
}

func (rc *regClient) loadDockerCreds() error {
	conffile := dockercfg.LoadDefaultConfigFile(os.Stderr)
	creds, err := conffile.GetAllCredentials()
	if err != nil {
		return fmt.Errorf("Failed to load docker creds %s", err)
	}
	for name, cred := range creds {
		if (cred.Username == "" || cred.Password == "") && cred.IdentityToken == "" {
			rc.log.WithFields(logrus.Fields{
				"host": cred.ServerAddress,
			}).Debug("Docker cred: Skipping empty pass and token")
			continue
		}
		if cred.ServerAddress == "" {
			cred.ServerAddress = name
		}
		// Docker Hub is a special case
		if name == DockerRegistryAuth {
			name = DockerRegistry
			cred.ServerAddress = DockerRegistryDNS
		}
		rc.log.WithFields(logrus.Fields{
			"name":      name,
			"host":      cred.ServerAddress,
			"user":      cred.Username,
			"pass-set":  cred.Password != "",
			"token-set": cred.IdentityToken != "",
		}).Debug("Loading docker cred")
		err = rc.hostSet(ConfigHost{
			Name:     name,
			Hostname: cred.ServerAddress,
			User:     cred.Username,
			Pass:     cred.Password,
			Token:    cred.IdentityToken, // TODO: verify token can be used
		})
		if err != nil {
			// treat each of these as non-fatal
			rc.log.WithFields(logrus.Fields{
				"registry": name,
				"user":     cred.Username,
				"error":    err,
			}).Warn("Failed to use docker credential")
		}
	}
	return nil
}

func (rc *regClient) hostGet(hostname string) *ConfigHost {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if _, ok := rc.hosts[hostname]; !ok {
		rc.hosts[hostname] = &regClientHost{}
	}
	if rc.hosts[hostname].config == nil {
		rc.hosts[hostname].config = ConfigHostNewName(hostname)
	}
	return rc.hosts[hostname].config
}

func (rc *regClient) hostSet(newHost ConfigHost) error {
	name := newHost.Name
	if _, ok := rc.hosts[name]; !ok {
		// merge newHost with default host settings
		mergedHost := rc.mergeConfigHost(*ConfigHostNewName(name), newHost, false)
		rc.hosts[name] = &regClientHost{config: &mergedHost}
	} else {
		mergedHost := rc.mergeConfigHost(*rc.hosts[name].config, newHost, true)
		rc.hosts[name].config = &mergedHost
	}
	return nil
}

func (rc *regClient) getRetryable(host *ConfigHost) retryable.Retryable {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if _, ok := rc.hosts[host.Name]; !ok {
		rc.hosts[host.Name] = &regClientHost{config: ConfigHostNewName(host.Name)}
	}
	if rc.hosts[host.Name].retryable == nil {
		c := &http.Client{}
		a := auth.NewAuth(
			auth.WithLog(rc.log),
			auth.WithHTTPClient(c),
			auth.WithCreds(host.authCreds()),
			auth.WithClientID(rc.userAgent),
		)
		rOpts := []retryable.Opts{
			retryable.WithLog(rc.log),
			retryable.WithHTTPClient(c),
			retryable.WithAuth(a),
			retryable.WithUserAgent(rc.userAgent),
		}
		if certs := rc.getCerts(host); len(certs) > 0 {
			rOpts = append(rOpts, retryable.WithCertFiles(certs))
		}
		if host.RegCert != "" {
			rOpts = append(rOpts, retryable.WithCerts([][]byte{[]byte(host.RegCert)}))
		}
		if rc.retryLimit > 0 {
			rOpts = append(rOpts, retryable.WithLimit(rc.retryLimit))
		}
		if rc.retryDelayInit > 0 || rc.retryDelayMax > 0 {
			rOpts = append(rOpts, retryable.WithDelay(rc.retryDelayInit, rc.retryDelayMax))
		}
		r := retryable.NewRetryable(rOpts...)
		rc.hosts[host.Name].retryable = r
	}
	return rc.hosts[host.Name].retryable
}

func (host *ConfigHost) authCreds() func(h string) auth.Cred {
	return func(h string) auth.Cred {
		return auth.Cred{User: host.User, Password: host.Pass, Token: host.Token}
	}
}

func (rc *regClient) getCerts(host *ConfigHost) []string {
	var certs []string

	for _, certPath := range rc.certPaths {
		hostDir := filepath.Join(certPath, host.Hostname)
		files, err := ioutil.ReadDir(hostDir)
		if err != nil {
			if !os.IsNotExist(err) {
				rc.log.WithFields(logrus.Fields{
					"err": err,
					"dir": hostDir,
				}).Warn("Failed to open docker cert dir")
			}
			continue
		}
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			if strings.HasSuffix(f.Name(), ".crt") {
				certs = append(certs, filepath.Join(hostDir, f.Name()))
			}
		}
	}
	return certs
}
