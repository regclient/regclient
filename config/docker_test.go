package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/regclient/regclient/internal/conffile"
)

func TestDocker(t *testing.T) {
	curPath := os.Getenv("PATH")
	pwd, err := os.Getwd()
	if err != nil {
		t.Errorf("failed to get working dir: %v", err)
		return
	}
	os.Setenv("PATH", filepath.Join(pwd, "testdata")+string(os.PathListSeparator)+curPath)
	defer os.Setenv("PATH", curPath)
	curDockerConf := os.Getenv(dockerEnv)
	os.Setenv(dockerEnv, "testdata")
	if curDockerConf != "" {
		defer os.Setenv(dockerEnv, curDockerConf)
	} else {
		defer os.Unsetenv(dockerEnv)
	}
	hosts, err := DockerLoad()
	if err != nil {
		t.Errorf("error loading docker credentials: %v", err)
		return
	}
	hostMap := map[string]*Host{}
	for _, h := range hosts {
		h := h // shadow h for unique var/pointer
		hostMap[h.Name] = &h
	}
	tests := []struct {
		name             string
		hostname         string
		expectUser       string
		expectPass       string
		expectCredHelper string
		expectTLS        TLSConf
		expectHostname   string
		expectCredHost   string
	}{
		{
			name:             "testhost",
			hostname:         "testhost.example.com",
			expectCredHelper: "docker-credential-test",
			expectHostname:   "testhost.example.com",
			expectTLS:        TLSEnabled,
		},
		{
			name:           "localhost:5001",
			hostname:       "localhost:5001",
			expectUser:     "hello",
			expectPass:     "docker",
			expectHostname: "localhost:5001",
			expectTLS:      TLSEnabled,
		},
		{
			name:             "docker.io",
			hostname:         DockerRegistry,
			expectCredHelper: "docker-credential-test",
			expectHostname:   DockerRegistryDNS,
			expectTLS:        TLSEnabled,
			expectCredHost:   DockerRegistryAuth,
		},
		{
			name:             "http.example.com",
			hostname:         "http.example.com",
			expectCredHelper: "docker-credential-test",
			expectHostname:   "http.example.com",
			expectTLS:        TLSDisabled,
			expectCredHost:   "http://http.example.com/",
		},
		{
			name:             "storehost",
			hostname:         "storehost.example.com",
			expectUser:       "hello",
			expectCredHelper: "docker-credential-teststore",
			expectHostname:   "storehost.example.com",
			expectTLS:        TLSEnabled,
		},
		{
			name:             "storehttp.example.com",
			hostname:         "storehttp.example.com",
			expectUser:       "hello",
			expectCredHelper: "docker-credential-teststore",
			expectHostname:   "storehttp.example.com",
			expectTLS:        TLSDisabled,
			expectCredHost:   "http://storehttp.example.com/",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, ok := hostMap[tt.hostname]
			if !ok {
				t.Errorf("host not found: %s", tt.hostname)
				return
			}
			if tt.expectUser != h.User {
				t.Errorf("user mismatch, expect %s, received %s", tt.expectUser, h.User)
			}
			if tt.expectPass != h.Pass {
				t.Errorf("pass mismatch, expect %s, received %s", tt.expectPass, h.Pass)
			}
			if tt.expectTLS != h.TLS {
				eTLS, _ := tt.expectTLS.MarshalText()
				hTLS, _ := h.TLS.MarshalText()
				t.Errorf("tls mismatch, expect %s, received %s", eTLS, hTLS)
			}
			if tt.expectCredHelper != h.CredHelper {
				t.Errorf("cred helper mismatch, expect %s, received %s", tt.expectCredHelper, h.CredHelper)
			}
			if tt.expectCredHost != h.CredHost {
				t.Errorf("cred host mismatch, expect %s, received %s", tt.expectCredHost, h.CredHost)
			}
		})
	}
}

func TestLoadMissing(t *testing.T) {
	cf := conffile.New(conffile.WithFullname("testdata/missing.json"))
	h, err := dockerParse(cf)
	if err != nil {
		t.Errorf("error encountered when parsing missing file: %v", err)
	}
	if len(h) > 0 {
		t.Errorf("hosts returned from missing file")
	}
}
