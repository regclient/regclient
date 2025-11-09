package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDocker(t *testing.T) {
	// cannot run cred helper in parallel because of OS working directory race conditions
	t.Setenv(dockerEnvConfig, `{
  "auths": {
    "testenv.example.com": {
      "auth": "aGllbnY6dGVzdHBhc3M="
    }
  },
  "credHelpers": {
    "testenvhelper.example.com": "test"
  }
}`)
	pwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working dir: %v", err)
	}
	curPath := os.Getenv("PATH")
	t.Setenv("PATH", filepath.Join(pwd, "testdata")+string(os.PathListSeparator)+curPath)
	t.Setenv(dockerEnv, "testdata")
	hosts, err := DockerLoad()
	if err != nil {
		t.Fatalf("error loading docker credentials: %v", err)
	}
	hostMap := map[string]*Host{}
	for _, h := range hosts {
		hostMap[h.Name] = &h
	}
	tt := []struct {
		name             string
		hostname         string
		expectUser       string
		expectPass       string
		expectCredHelper string
		expectTLS        TLSConf
		expectHostname   string
		expectCredHost   string
		expectMissing    bool
	}{
		{
			name:             "testhost",
			hostname:         "testhost.example.com",
			expectCredHelper: "docker-credential-test",
			expectHostname:   "testhost.example.com",
			expectTLS:        TLSEnabled,
		},
		{
			name:           "testenv",
			hostname:       "testenv.example.com",
			expectUser:     "hienv",
			expectPass:     "testpass",
			expectHostname: "testenv.example.com",
			expectTLS:      TLSEnabled,
		},
		{
			name:             "testenvhelper",
			hostname:         "testenvhelper.example.com",
			expectCredHelper: "docker-credential-test",
			expectHostname:   "testenvhelper.example.com",
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
		{
			name:          "missing-from-repo.example.com", // entries with a repository are ignored
			hostname:      "missing-from-repo.example.com",
			expectMissing: true,
		},
		{
			name:          "index.docker.io", // verify access-token and refresh-token entries are ignored
			hostname:      "index.docker.io",
			expectMissing: true,
		},
		{
			name:          "https://index.docker.io/v1/access-token",
			hostname:      "https://index.docker.io/v1/access-token",
			expectMissing: true,
		},
		{
			name:          "https://index.docker.io/v1/test-token",
			hostname:      "https://index.docker.io/v1/test-token",
			expectMissing: true,
		},
		{
			name:          "https://index.docker.io/v1/helper-token",
			hostname:      "https://index.docker.io/v1/helper-token",
			expectMissing: true,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			h, ok := hostMap[tc.hostname]
			if tc.expectMissing {
				if ok {
					t.Fatalf("entry found that should be missing: %s", tc.hostname)
				}
				return
			}
			if !ok {
				t.Fatalf("host not found: %s", tc.hostname)
			}
			if tc.expectUser != h.User {
				t.Errorf("user mismatch, expect %s, received %s", tc.expectUser, h.User)
			}
			if tc.expectPass != h.Pass {
				t.Errorf("pass mismatch, expect %s, received %s", tc.expectPass, h.Pass)
			}
			if tc.expectTLS != h.TLS {
				eTLS, _ := tc.expectTLS.MarshalText()
				hTLS, _ := h.TLS.MarshalText()
				t.Errorf("tls mismatch, expect %s, received %s", eTLS, hTLS)
			}
			if tc.expectCredHelper != h.CredHelper {
				t.Errorf("cred helper mismatch, expect %s, received %s", tc.expectCredHelper, h.CredHelper)
			}
			if tc.expectCredHost != h.CredHost {
				t.Errorf("cred host mismatch, expect %s, received %s", tc.expectCredHost, h.CredHost)
			}
		})
	}
}

func TestLoadMissing(t *testing.T) {
	// cannot run cred helper in parallel because of OS working directory race conditions
	h, err := DockerLoadFile("testdata/missing.json")
	if err != nil {
		t.Errorf("error encountered when parsing missing file: %v", err)
	}
	if len(h) > 0 {
		t.Errorf("hosts returned from missing file")
	}
}
