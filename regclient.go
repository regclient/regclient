package regclient

import (
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"io/ioutil"
	"strings"
	"sync"
	"time"

	// crypto libraries included for go-digest
	_ "crypto/sha256"
	_ "crypto/sha512"
	"fmt"
	"os"

	dockercfg "github.com/docker/cli/cli/config"
	"github.com/regclient/regclient/config"
	"github.com/regclient/regclient/internal/rwfs"
	"github.com/regclient/regclient/scheme"
	"github.com/regclient/regclient/scheme/ocidir"
	"github.com/regclient/regclient/scheme/reg"
	"github.com/regclient/regclient/types"
	"github.com/sirupsen/logrus"
)

const (
	// DefaultUserAgent sets the header on http requests
	DefaultUserAgent = "regclient/regclient"
	// DockerCertDir default location for docker certs
	DockerCertDir      = "/etc/docker/certs.d"
	DockerRegistry     = config.DockerRegistry
	DockerRegistryAuth = config.DockerRegistryAuth
	DockerRegistryDNS  = config.DockerRegistryDNS
)

//go:embed embed/*
var embedFS embed.FS

var (
	// VCSRef and VCSTag are populated from an embed at build time
	// These are used to version the UserAgent header
	VCSRef = ""
	VCSTag = ""
)

func init() {
	setupVCSVars()
}

type RegClient struct {
	certPaths []string
	hosts     map[string]*config.Host
	log       *logrus.Logger
	mu        sync.Mutex
	regOpts   []reg.Opts
	schemes   map[string]scheme.SchemeAPI
	userAgent string
}

// Opt functions are used to configure NewRegClient
type Opt func(*RegClient)

// New returns a registry client
func New(opts ...Opt) *RegClient {
	var rc = RegClient{
		certPaths: []string{},
		hosts:     map[string]*config.Host{},
		userAgent: DefaultUserAgent,
		// logging is disabled by default
		log:     &logrus.Logger{Out: ioutil.Discard},
		regOpts: []reg.Opts{},
		schemes: map[string]scheme.SchemeAPI{},
	}
	if VCSTag != "" {
		rc.userAgent = fmt.Sprintf("%s (%s)", rc.userAgent, VCSTag)
	} else if VCSRef != "" {
		rc.userAgent = fmt.Sprintf("%s (%s)", rc.userAgent, VCSRef)
	}

	// inject Docker Hub settings
	rc.hostSet(config.Host{
		Name:     DockerRegistry,
		TLS:      config.TLSEnabled,
		Hostname: DockerRegistryDNS,
	})

	for _, opt := range opts {
		opt(&rc)
	}

	// configure regOpts
	if len(rc.certPaths) > 0 {
		rc.regOpts = append(rc.regOpts, reg.WithCertFiles(rc.certPaths))
	}
	hostList := []*config.Host{}
	for _, h := range rc.hosts {
		hostList = append(hostList, h)
	}
	rc.regOpts = append(rc.regOpts,
		reg.WithConfigHosts(hostList),
		reg.WithLog(rc.log),
		reg.WithUserAgent(rc.userAgent),
	)

	// setup scheme's
	rc.schemes["reg"] = reg.New(rc.regOpts...)
	rc.schemes["ocidir"] = ocidir.New(
		ocidir.WithLog(rc.log),
		ocidir.WithFS(rwfs.OSNew("")),
	)

	rc.log.Debug("regclient initialized")

	return &rc
}

// WithCertDir adds a path of certificates to trust similar to Docker's /etc/docker/certs.d
func WithCertDir(path ...string) Opt {
	return func(rc *RegClient) {
		rc.regOpts = append(rc.regOpts, reg.WithCertDirs(path))
		return
	}
}

// WithDockerCerts adds certificates trusted by docker in /etc/docker/certs.d
func WithDockerCerts() Opt {
	return func(rc *RegClient) {
		rc.certPaths = append(rc.certPaths, DockerCertDir)
		return
	}
}

// WithDockerCreds adds configuration from users docker config with registry logins
// This changes the default value from the config file, and should be added after the config file is loaded
func WithDockerCreds() Opt {
	return func(rc *RegClient) {
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
	return func(rc *RegClient) {
		if configHosts == nil || len(configHosts) == 0 {
			return
		}
		for _, configHost := range configHosts {
			if configHost.Name == "" {
				// TODO: should this error, warn, or fall back to hostname?
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
	return func(rc *RegClient) {
		rc.regOpts = append(rc.regOpts, reg.WithBlobSize(chunk, max))
	}
}

// WithLog overrides default logrus Logger
func WithLog(log *logrus.Logger) Opt {
	return func(rc *RegClient) {
		rc.log = log
	}
}

// WithRetryDelay specifies the time permitted for retry delays
func WithRetryDelay(delayInit, delayMax time.Duration) Opt {
	return func(rc *RegClient) {
		rc.regOpts = append(rc.regOpts, reg.WithDelay(delayInit, delayMax))
	}
}

// WithRetryLimit specifies the number of retries for non-fatal errors
func WithRetryLimit(retryLimit int) Opt {
	return func(rc *RegClient) {
		rc.regOpts = append(rc.regOpts, reg.WithRetryLimit(retryLimit))
	}
}

// WithUserAgent specifies the User-Agent http header
func WithUserAgent(ua string) Opt {
	return func(rc *RegClient) {
		rc.userAgent = ua
	}
}

func (rc *RegClient) loadDockerCreds() error {
	conffile := dockercfg.LoadDefaultConfigFile(os.Stderr)
	creds, err := conffile.GetAllCredentials()
	if err != nil {
		return fmt.Errorf("Failed to load docker creds %s", err)
	}
	for name, cred := range creds {
		if (cred.Username == "" || cred.Password == "") && cred.IdentityToken == "" {
			rc.log.WithFields(logrus.Fields{
				"name": name,
			}).Debug("Docker cred: Skipping empty pass and token")
			continue
		}
		// Docker Hub is a special case
		if name == DockerRegistryAuth {
			name = DockerRegistry
			cred.ServerAddress = DockerRegistryDNS
		}
		// handle names with a scheme included (https://registry.example.com)
		tls := config.TLSEnabled
		i := strings.Index(name, "://")
		if i > 0 {
			scheme := name[:i]
			if name == cred.ServerAddress {
				cred.ServerAddress = name[i+3:]
			}
			name = name[i+3:]
			if scheme == "http" {
				tls = config.TLSDisabled
			}
		}
		if cred.ServerAddress == "" {
			cred.ServerAddress = name
		}
		tlsB, _ := tls.MarshalText()
		rc.log.WithFields(logrus.Fields{
			"name":      name,
			"host":      cred.ServerAddress,
			"tls":       string(tlsB),
			"user":      cred.Username,
			"pass-set":  cred.Password != "",
			"token-set": cred.IdentityToken != "",
		}).Debug("Loading docker cred")
		err = rc.hostSet(config.Host{
			Name:     name,
			Hostname: cred.ServerAddress,
			TLS:      tls,
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

func (rc *RegClient) getScheme(scheme string) (scheme.SchemeAPI, error) {
	s, ok := rc.schemes[scheme]
	if !ok {
		return nil, fmt.Errorf("%w: unknown scheme \"%s\"", types.ErrNotImplemented, scheme)
	}
	return s, nil
}

func (rc *RegClient) hostSet(newHost config.Host) error {
	name := newHost.Name
	var err error
	if _, ok := rc.hosts[name]; !ok {
		// merge newHost with default host settings
		rc.hosts[name] = config.HostNewName(name)
		err = rc.hosts[name].Merge(newHost, nil)
	} else {
		// merge newHost with existing settings
		err = rc.hosts[name].Merge(newHost, rc.log)
	}
	if err != nil {
		return err
	}
	return nil
}

func setupVCSVars() {
	verS := struct {
		VCSRef string
		VCSTag string
	}{}

	// regclient only looks for releases, individual binaries will look at their local directories
	verB, err := embedFS.ReadFile("embed/release.json")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return
	}

	if len(verB) > 0 {
		err = json.Unmarshal(verB, &verS)
		if err != nil {
			return
		}
	}

	if verS.VCSRef != "" {
		VCSRef = verS.VCSRef
	}
	if verS.VCSTag != "" {
		VCSTag = verS.VCSTag
	}
}
