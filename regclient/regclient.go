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
	"crypto/tls"
	"fmt"
	"net/http"
	"os"

	dockercfg "github.com/docker/cli/cli/config"
	"github.com/regclient/regclient/internal/auth"
	"github.com/regclient/regclient/internal/retryable"
	"github.com/regclient/regclient/regclient/config"
	"github.com/sirupsen/logrus"
)

const (
	// DefaultBlobChunk 1M chunks, this is allocated in a memory buffer
	DefaultBlobChunk = 1024 * 1024
	// DefaultBlobMax is disabled to support registries without chunked upload support
	DefaultBlobMax = -1
	// DefaultRetryLimit sets how many retry attempts are made for non-fatal errors
	DefaultRetryLimit = 3
	// DefaultUserAgent sets the header on http requests
	DefaultUserAgent = "regclient/regclient"
	// DockerCertDir default location for docker certs
	DockerCertDir      = "/etc/docker/certs.d"
	DockerRegistry     = config.DockerRegistry
	DockerRegistryAuth = config.DockerRegistryAuth
	DockerRegistryDNS  = config.DockerRegistryDNS
)

var (
	// VCSRef is injected from a build flag, used to version the UserAgent header
	VCSRef = "unknown"
)

// RegClient remains for backwards compatibility
type RegClient = *Client

type ociAPI interface {
	ociTagAPI
	ociManifestAPI
	ociBlobAPI
}

type Client struct {
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
	config    *config.Host
	retryable retryable.Retryable
}

// Opt functions are used to configure NewRegClient
type Opt func(*Client)

// NewRegClient returns a registry client
func NewRegClient(opts ...Opt) *Client {
	var rc = Client{
		certPaths:     []string{},
		hosts:         map[string]*regClientHost{},
		retryLimit:    DefaultRetryLimit,
		userAgent:     DefaultUserAgent,
		blobChunkSize: DefaultBlobChunk,
		blobMaxPut:    DefaultBlobMax,
		// logging is disabled by default
		log: &logrus.Logger{Out: ioutil.Discard},
	}

	// inject Docker Hub settings
	rc.hostSet(config.Host{
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
	return func(rc *Client) {
		rc.certPaths = append(rc.certPaths, path)
		return
	}
}

// WithDockerCerts adds certificates trusted by docker in /etc/docker/certs.d
func WithDockerCerts() Opt {
	return func(rc *Client) {
		rc.certPaths = append(rc.certPaths, DockerCertDir)
		return
	}
}

// WithDockerCreds adds configuration from users docker config with registry logins
// This changes the default value from the config file, and should be added after the config file is loaded
func WithDockerCreds() Opt {
	return func(rc *Client) {
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
func WithConfigHosts(configHosts []config.Host) Opt {
	return func(rc *Client) {
		if configHosts == nil || len(configHosts) == 0 {
			return
		}
		for _, configHost := range configHosts {
			if configHost.Name == "" {
				// TODO: should this error or warn?
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
				"blobMax":    configHost.BlobMax,
				"blobChunk":  configHost.BlobChunk,
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
func WithConfigHost(configHost config.Host) Opt {
	return WithConfigHosts([]config.Host{configHost})
}

// WithBlobSize overrides default blob sizes
func WithBlobSize(chunk, max int64) Opt {
	return func(rc *Client) {
		if chunk > 0 {
			rc.blobChunkSize = chunk
		}
		if max != 0 {
			rc.blobMaxPut = max
		}
	}
}

// WithLog overrides default logrus Logger
func WithLog(log *logrus.Logger) Opt {
	return func(rc *Client) {
		rc.log = log
	}
}

// WithRetryDelay specifies the time permitted for retry delays
func WithRetryDelay(delayInit, delayMax time.Duration) Opt {
	return func(rc *Client) {
		rc.retryDelayInit = delayInit
		rc.retryDelayMax = delayMax
	}
}

// WithRetryLimit specifies the number of retries for non-fatal errors
func WithRetryLimit(retryLimit int) Opt {
	return func(rc *Client) {
		rc.retryLimit = retryLimit
	}
}

// WithUserAgent specifies the User-Agent http header
func WithUserAgent(ua string) Opt {
	return func(rc *Client) {
		rc.userAgent = ua
	}
}

func (rc *Client) loadDockerCreds() error {
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
		err = rc.hostSet(config.Host{
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

func (rc *Client) hostGet(hostname string) *config.Host {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if _, ok := rc.hosts[hostname]; !ok {
		rc.hosts[hostname] = &regClientHost{}
	}
	if rc.hosts[hostname].config == nil {
		rc.hosts[hostname].config = config.HostNewName(hostname)
	}
	return rc.hosts[hostname].config
}

func (rc *Client) hostSet(newHost config.Host) error {
	name := newHost.Name
	if _, ok := rc.hosts[name]; !ok {
		// merge newHost with default host settings
		curHost := config.HostNewName(name)
		err := curHost.Merge(newHost, nil)
		if err != nil {
			return err
		}
		rc.hosts[name] = &regClientHost{config: curHost}
	} else {
		err := rc.hosts[name].config.Merge(newHost, rc.log)
		if err != nil {
			return err
		}
	}
	return nil
}

func (rc *Client) getRetryable(host *config.Host) retryable.Retryable {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if _, ok := rc.hosts[host.Name]; !ok {
		rc.hosts[host.Name] = &regClientHost{config: config.HostNewName(host.Name)}
	}
	if rc.hosts[host.Name].retryable == nil {
		c := &http.Client{}
		if host.TLS == TLSInsecure {
			c.Transport = &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			}
		}
		a := auth.NewAuth(
			auth.WithLog(rc.log),
			auth.WithHTTPClient(c),
			auth.WithCreds(host.AuthCreds()),
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

func (rc *Client) getCerts(host *config.Host) []string {
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
