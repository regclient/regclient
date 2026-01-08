package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/regclient/regclient/internal/timejson"
)

func TestConfig(t *testing.T) {
	// cannot run cred helper in parallel because of OS working directory race conditions
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed checking current directory: %v", err)
	}
	curPath := os.Getenv("PATH")
	t.Setenv("PATH", filepath.Join(cwd, "testdata")+string(os.PathListSeparator)+curPath)

	// generate new/blank
	newHostP := HostNew()

	// generate new/hostname
	newHostNameP := HostNewName("host.example.org")

	defMirror := Host{
		Mirrors: []string{"mirror.example.org"},
	}
	defCredHelper := Host{
		CredHelper: "docker-credential-test",
	}

	newHostDefNil := HostNewDefName(nil, "host.example.org")
	newHostDefMirror := HostNewDefName(&defMirror, "host.example.org")
	newHostDefCredHelper := HostNewDefName(&defCredHelper, "host.example.org")

	caCert := string(`-----BEGIN CERTIFICATE-----
	MIIC/zCCAeegAwIBAgIUPrFPsUzINvS75tp6kIdsycXrrSQwDQYJKoZIhvcNAQEL
	BQAwDzENMAsGA1UEAwwERGVtbzAeFw0yMzA1MzEwMDI0NDJaFw0zMzA1MjgwMDI0
	NDJaMA8xDTALBgNVBAMMBERlbW8wggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEK
	AoIBAQDWdtttrOqNS9WhwhL+6G4annBVLP1Eis+pH5sXL1O71lXAWUSXYTqEgLlB
	g5Id8vAvS4bz2ogPnOURTsEwHp/vfPpMs1mHd71apd0b4aDNThvVK4t0y9KrMZ9I
	cVyX/tkoR/CIEkmVqiUxiG2hfZTUTuO7pKkjZHV7DOSCBp7QOVhl16grEXOCWp8X
	DAKl90WowMmtXBLX11/n9KWlwE2PaVPTp/4B4z4E44sBFATWfezDTv5ieTaKvLAN
	SGEa9cA4eqjSA/mJAxlsEOW5IZRfqNskTwpRCMzdQ0UtyvLUlWqXdPdN07RbnT08
	FipckYLaT8YtipA/Pgg1CGJLwBxRAgMBAAGjUzBRMB0GA1UdDgQWBBR6w/+PiaNa
	F9vTVx5Xob/kYfRFEDAfBgNVHSMEGDAWgBR6w/+PiaNaF9vTVx5Xob/kYfRFEDAP
	BgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3DQEBCwUAA4IBAQCuoCA/3wZuMgT9fYCK
	+inOPi0no+sB+l8GCx0lYAkjIPyJISqvixfHbgXg5zKubgHyDXziUpKFsvF8kloo
	7KIjWsWi7R8mONWKIc+f1WsVbFzheS6hqg+YyPwN2Kws7YDhQ3cbeajByHLNzEYm
	gVtTz6wFP+B3IMGH4yeghGMHi7PGPrtj93uhCLUHswlEEFBHE+Kzn3AcJzpmY+M5
	9T4x+na+bdlNEKuBqRYNxrNexQ1Nb82JxeR89RnPXXwdWBDw9UhiztRPWNA8nlJr
	s1j+J2mbMDUuG2N+ndivBimxP1y8bEYeHPtzskqECj08ul97hsi2ihGJUBpEjEca
	ZFjP
	-----END CERTIFICATE-----
	`)
	clientCert := string(`-----BEGIN CERTIFICATE-----
	MIICpzCCAY8CFAx1ZpY9FPZJ1zmdMkpdDa3S7gq3MA0GCSqGSIb3DQEBCwUAMA8x
	DTALBgNVBAMMBERlbW8wHhcNMjMwNTMxMDAzMzI0WhcNMjQwNTMwMDAzMzI0WjAR
	MQ8wDQYDVQQDDAZjbGllbnQwggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIB
	AQDilia59g7DkNqZ4gUC3WLZEtVyt2JIzeeNFy/wyhCzEJQhMlaW+lEMnGa9fgpo
	w9d4bIl1El6VtM1+/KXqTpGJqrvSMsNFVHifVAWuHYTuqC7oG+T4DIyjR/NlDYWA
	y/WpUhKIY6YLmx/CrbqzGR0QUCkv2kbQufEGSZHRLGGc2kkUMD+P4PlHv0ao2NR3
	sbpK0IA1bzSsNGQK1LIBDw4pWjJY2Mzrl9it1acYUSvAPPxoX5FAFjTuYyMumzvf
	kkwk5UsjPMO+m+xgUz0FVBKSUZi7E03ucl6R/hrwN3ADfK14SrL3JkzkWtzlkRGa
	d0CMcR8q2l8w+WDpyA7hg4gLAgMBAAEwDQYJKoZIhvcNAQELBQADggEBAEmmzAHb
	HamtaCp20VHmGrIRC3TRtxMnCqDf/FK4ersUeBwmyPogbUll421dDHp1BpgBx2NN
	wwuoxjDd+saHOSkj/ueLPEql87xos4H6/0JLMssp5SBeO/U1a5mV8Ufnv54Ya055
	c/GBLlwx1+P22hzPOu8gzHJyVJ2ZMesSQYLi1upBrPPGKu3TU+0QV+OnpBJ4pH+j
	x9GXt5iEVR0c0ela+7VKm+DRgKPlzoAoCKkMpSv/LqCITAkL3pcQG1XFv8N4nuJj
	0d3wwBhuPfJxpy7flA0JJMXFjx7EcoQ+yYXd55TtcKKEnC+vZTeZSh3geHw2fdWE
	V+eBX1Ya5MHFmDs=
	-----END CERTIFICATE-----
	`)
	clientKey := string(`-----BEGIN RSA PRIVATE KEY-----
	MIIEpgIBAAKCAQEA4pYmufYOw5DameIFAt1i2RLVcrdiSM3njRcv8MoQsxCUITJW
	lvpRDJxmvX4KaMPXeGyJdRJelbTNfvyl6k6Riaq70jLDRVR4n1QFrh2E7qgu6Bvk
	+AyMo0fzZQ2FgMv1qVISiGOmC5sfwq26sxkdEFApL9pG0LnxBkmR0SxhnNpJFDA/
	j+D5R79GqNjUd7G6StCANW80rDRkCtSyAQ8OKVoyWNjM65fYrdWnGFErwDz8aF+R
	QBY07mMjLps735JMJOVLIzzDvpvsYFM9BVQSklGYuxNN7nJekf4a8DdwA3yteEqy
	9yZM5Frc5ZERmndAjHEfKtpfMPlg6cgO4YOICwIDAQABAoIBAQCACYreoE0dc3gj
	ZpWgVctqkHru9PNj4n5KuuSLMxOWq/KYg6JsdAxijOp9f4CQTMIwOVy/O98Yx28r
	p8Z1jWouGb1CfQ7c2WvD1K3VArdASOcgn8qV5DmAdsLxwl9DNX2e7VKtoWmNu12K
	G7OZSsKimjl74eMMRVYOUHpGccbC44IMVAC11NA5/dLon+oQZAcCDDs7SHCX+TaV
	zaQknJSJBiJvpCmai1eXaZPuqyjoAqcrhAj4H/Os406qo8VRxJ/UjaBq6mbgrxQj
	tWe7j4LCDMKDWUI8R0Z+pGV714YhCRqSnpwukNGahGeSjuHLYQjN52Vdfjj568/N
	OMUNWE+pAoGBAPbCdPgb/r8O/tkGBFikZj0daG3Mh9kVHhB7QVeHPZNR+RLJ5zGM
	+Mfo5Rv6qeBeZgGZCsWL6himMkmfR+mLcS0Rmvzk5v3SahKNKOlYFkUWn45gF0b5
	gOfthx7JDG/N8n9UJgS42aHktw/Ucg4qvf8Rrj5MikXiMN0tLf2mec2nAoGBAOsS
	ToN6LXcm7F+SibJfLkQZJoe4+fe/FfMCowZmtNihH5uhSTz6XbYd/REjevWA/d6g
	G9odpAcDNyoNZbIrlF3enKaLEyR9DwaQ0B6J4048e07sGTyG5UOV3aU5NdAENzWL
	8aUophOLdQdbAGMybfCn8tLJs7AKEmu29QJ1D6b9AoGBALxXtDvj8k8WPQKdCxg1
	cyvWlGyqHk5dRfNCgJ80RJV7jeb/YI17ki/T3Xu7mYn9w1IY5BXgMy/ZOqzi/FqP
	6jSCKZA5ju3RetDqGX3xlB3rpKFhSqMLsY5UyDuBLRLxWNRDADm+da6SCf/1IZEa
	oqZbcmlutmOcv7sxzta6CGIlAoGBALXqE+KBgX/NGm2XvIHSUL6YbA3qY1+LfBP0
	fW7tupROlGRe+4t6AV13dal2uKgW6+AGLaes+owGvAEKHyIzwXynUrk7tVOuiBs/
	pB+N+99GxPI9mgYSKogUCVPcoz1YldUVeKqke2lyqd1IWlNp6lSr1Cm1uB3KnZjI
	HHGLX9KNAoGBANPV1Gi5bU9SgSnq+jv49VNIp9tAwIQh1Be4EVaVFdEieFdXrcYc
	XWLKsAchSad8ruRsY+cCk6SYwMQtKE0vnEWZi8jaRq1RwGUfRBaoU7of+eR/WK77
	8/Ke5Y97bV67PT3UdEec54fNmhl+vHEPH2knvnbQQQ9iY42f+VGIB0Kn
	-----END RSA PRIVATE KEY-----
	`)
	caCert = strings.ReplaceAll(caCert, "\t", "")
	clientCert = strings.ReplaceAll(clientCert, "\t", "")
	clientKey = strings.ReplaceAll(clientKey, "\t", "")

	// parse json
	exJSON := `
	{
	  "tls": "enabled",
		"hostname": "host.example.com",
		"user": "user-ex",
		"pass": "secret",
		"pathPrefix": "hub",
		"mirrors": ["host1.example.com","host2.example.com"],
		"priority": 42,
		"apiOpts": {"disableHead": "true"},
		"blobChunk": 123456,
		"blobMax": 999999
	}
	`
	exJSON2 := `
	{
	  "tls": "disabled",
		"hostname": "host2.example.com",
		"user": "user-ex3",
		"pass": "secret3",
		"regcert": "` + strings.ReplaceAll(caCert, "\n", "\\n") + `",
		"clientCert": "` + strings.ReplaceAll(clientCert, "\n", "\\n") + `",
		"clientKey": "` + strings.ReplaceAll(clientKey, "\n", "\\n") + `",
		"pathPrefix": "hub3",
		"mirrors": ["testhost.example.com"],
		"priority": 42,
		"apiOpts": {"disableHead": "false", "unknownOpt": "3"},
		"blobChunk": 333333,
		"blobMax": 333333
	}
	`
	exJSONCredHelper := `
	{
	  "tls": "insecure",
		"hostname": "testhost.example.com",
		"credHelper": "docker-credential-test",
		"credExpire": "1h0m0s"
	}
	`
	var exHost, exHost2, exHostCredHelper Host
	err = json.Unmarshal([]byte(exJSON), &exHost)
	if err != nil {
		t.Errorf("failed unmarshaling exJson: %v", err)
	}
	err = json.Unmarshal([]byte(exJSON2), &exHost2)
	if err != nil {
		t.Errorf("failed unmarshaling exJson2: %v", err)
	}
	err = json.Unmarshal([]byte(exJSONCredHelper), &exHostCredHelper)
	if err != nil {
		t.Errorf("failed unmarshaling exJsonCredHelper: %v", err)
	}

	// merge blank with json
	exMergeBlank := *newHostP
	err = (&exMergeBlank).Merge(exHost, nil)
	if err != nil {
		t.Errorf("failed to merge blank host with exHost: %v", err)
	}
	exMergeHost2 := exHost
	err = (&exMergeHost2).Merge(exHost2, nil)
	if err != nil {
		t.Errorf("failed to merge ex host with exHost2: %v", err)
	}
	exMergeCredHelper := *newHostP
	err = (&exMergeCredHelper).Merge(exHostCredHelper, nil)
	if err != nil {
		t.Errorf("failed to merge blank host with exHostCredHelper: %v", err)
	}
	exMergeHostHelper := exHost
	err = (&exMergeHostHelper).Merge(exHostCredHelper, nil)
	if err != nil {
		t.Errorf("failed to merge ex host with cred helper: %v", err)
	}
	exMergeHelperHost := exHostCredHelper
	err = (&exMergeHelperHost).Merge(exHost, nil)
	if err != nil {
		t.Errorf("failed to merge ex cred helper with host: %v", err)
	}

	// verify fields in each
	tt := []struct {
		name       string
		host       Host
		hostExpect Host
		credExpect Cred
		isZero     bool
	}{
		{
			name:       "empty",
			host:       Host{},
			hostExpect: Host{},
			credExpect: Cred{},
			isZero:     true,
		},
		{
			name: "new",
			host: *newHostP,
			hostExpect: Host{
				TLS:     TLSEnabled,
				APIOpts: map[string]string{},
			},
			credExpect: Cred{},
			isZero:     true,
		},
		{
			name: "new-name",
			host: *newHostNameP,
			hostExpect: Host{
				Name:     "host.example.org",
				TLS:      TLSEnabled,
				Hostname: "host.example.org",
				APIOpts:  map[string]string{},
			},
			credExpect: Cred{},
			isZero:     true,
		},
		{
			name: "new-default-nil",
			host: *newHostDefNil,
			hostExpect: Host{
				Name:     "host.example.org",
				TLS:      TLSEnabled,
				Hostname: "host.example.org",
				APIOpts:  map[string]string{},
			},
			credExpect: Cred{},
			isZero:     true,
		},
		{
			name: "new-default-mirror",
			host: *newHostDefMirror,
			hostExpect: Host{
				TLS:      TLSEnabled,
				Hostname: "host.example.org",
				APIOpts:  map[string]string{},
				Mirrors:  []string{"mirror.example.org"},
			},
			credExpect: Cred{},
		},
		{
			name: "new-default-cred-helper",
			host: *newHostDefCredHelper,
			hostExpect: Host{
				TLS:        TLSEnabled,
				Hostname:   "host.example.org",
				APIOpts:    map[string]string{},
				CredHelper: "docker-credential-test",
			},
			credExpect: Cred{
				User:     "hello",
				Password: "world",
			},
		},
		{
			name: "exHost",
			host: exHost,
			hostExpect: Host{
				TLS:        TLSEnabled,
				Hostname:   "host.example.com",
				User:       "user-ex",
				Pass:       "secret",
				Priority:   42,
				BlobChunk:  123456,
				BlobMax:    999999,
				APIOpts:    map[string]string{"disableHead": "true"},
				PathPrefix: "hub",
				Mirrors:    []string{"host1.example.com", "host2.example.com"},
			},
			credExpect: Cred{
				User:     "user-ex",
				Password: "secret",
			},
		},
		{
			name: "exHost2",
			host: exHost2,
			hostExpect: Host{
				TLS:        TLSDisabled,
				Hostname:   "host2.example.com",
				User:       "user-ex3",
				Pass:       "secret3",
				RegCert:    caCert,
				ClientCert: clientCert,
				ClientKey:  clientKey,
				PathPrefix: "hub3",
				Mirrors:    []string{"testhost.example.com"},
				Priority:   42,
				APIOpts:    map[string]string{"disableHead": "false", "unknownOpt": "3"},
				BlobChunk:  333333,
				BlobMax:    333333,
			},
			credExpect: Cred{
				User:     "user-ex3",
				Password: "secret3",
			},
		},
		{
			name: "exHostCredHelper",
			host: exHostCredHelper,
			hostExpect: Host{
				TLS:        TLSInsecure,
				Hostname:   "testhost.example.com",
				CredHelper: "docker-credential-test",
				CredExpire: timejson.Duration(time.Hour),
				APIOpts:    map[string]string{},
			},
			credExpect: Cred{
				User:     "hello",
				Password: "world",
			},
		},
		{
			name: "mergeBlank",
			host: exMergeBlank,
			hostExpect: Host{
				TLS:        TLSEnabled,
				Hostname:   "host.example.com",
				User:       "user-ex",
				Pass:       "secret",
				Priority:   42,
				BlobChunk:  123456,
				BlobMax:    999999,
				APIOpts:    map[string]string{"disableHead": "true"},
				PathPrefix: "hub",
				Mirrors:    []string{"host1.example.com", "host2.example.com"},
			},
			credExpect: Cred{
				User:     "user-ex",
				Password: "secret",
			},
		},
		{
			name: "mergeHost2",
			host: exMergeHost2,
			hostExpect: Host{
				TLS:        TLSDisabled,
				Hostname:   "host2.example.com",
				User:       "user-ex3",
				Pass:       "secret3",
				RegCert:    caCert,
				ClientCert: clientCert,
				ClientKey:  clientKey,
				PathPrefix: "hub3",
				Mirrors:    []string{"testhost.example.com"},
				Priority:   42,
				APIOpts:    map[string]string{"disableHead": "false", "unknownOpt": "3"},
				BlobChunk:  333333,
				BlobMax:    333333,
			},
			credExpect: Cred{
				User:     "user-ex3",
				Password: "secret3",
			},
		},
		{
			name: "mergeHostCredHelper",
			host: exMergeCredHelper,
			hostExpect: Host{
				TLS:        TLSInsecure,
				Hostname:   "testhost.example.com",
				CredHelper: "docker-credential-test",
				CredExpire: timejson.Duration(time.Hour),
				APIOpts:    map[string]string{},
			},
			credExpect: Cred{
				User:     "hello",
				Password: "world",
			},
		},
		{
			name: "exMergeHostHelper",
			host: exMergeHostHelper,
			hostExpect: Host{
				TLS:        TLSInsecure,
				Hostname:   "testhost.example.com",
				CredHelper: "docker-credential-test",
				CredExpire: timejson.Duration(time.Hour),
				Priority:   42,
				BlobChunk:  123456,
				BlobMax:    999999,
				APIOpts:    map[string]string{"disableHead": "true"},
				PathPrefix: "hub",
				Mirrors:    []string{"host1.example.com", "host2.example.com"},
			},
			credExpect: Cred{
				User:     "hello",
				Password: "world",
			},
		},
		{
			name: "exMergeHelperHost",
			host: exMergeHelperHost,
			hostExpect: Host{
				TLS:        TLSEnabled,
				Hostname:   "host.example.com",
				User:       "user-ex",
				Pass:       "secret",
				Priority:   42,
				BlobChunk:  123456,
				BlobMax:    999999,
				APIOpts:    map[string]string{"disableHead": "true"},
				PathPrefix: "hub",
				Mirrors:    []string{"host1.example.com", "host2.example.com"},
			},
			credExpect: Cred{
				User:     "user-ex",
				Password: "secret",
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			if tc.host.IsZero() != tc.isZero {
				t.Errorf("IsZero did not return %t", tc.isZero)
			}
			// check each field
			if tc.host.TLS != tc.hostExpect.TLS {
				expect, _ := tc.hostExpect.TLS.MarshalText()
				found, _ := tc.host.TLS.MarshalText()
				t.Errorf("tls field mismatch, expected %s, found %s", expect, found)
			}
			if tc.host.RegCert != tc.hostExpect.RegCert {
				t.Errorf("regCert field mismatch, expected %s, found %s", tc.hostExpect.RegCert, tc.host.RegCert)
			}
			if tc.host.ClientCert != tc.hostExpect.ClientCert {
				t.Errorf("clientCert field mismatch, expected %s, found %s", tc.hostExpect.ClientCert, tc.host.ClientCert)
			}
			if tc.host.ClientKey != tc.hostExpect.ClientKey {
				t.Errorf("clientKey field mismatch, expected %s, found %s", tc.hostExpect.ClientKey, tc.host.ClientKey)
			}
			if tc.host.Hostname != tc.hostExpect.Hostname {
				t.Errorf("hostname field mismatch, expected %s, found %s", tc.hostExpect.Hostname, tc.host.Hostname)
			}
			if tc.host.User != tc.hostExpect.User {
				t.Errorf("user field mismatch, expected %s, found %s", tc.hostExpect.User, tc.host.User)
			}
			if tc.host.Pass != tc.hostExpect.Pass {
				t.Errorf("pass field mismatch, expected %s, found %s", tc.hostExpect.Pass, tc.host.Pass)
			}
			if tc.host.Token != tc.hostExpect.Token {
				t.Errorf("token field mismatch, expected %s, found %s", tc.hostExpect.Token, tc.host.Token)
			}
			if tc.host.CredHelper != tc.hostExpect.CredHelper {
				t.Errorf("credHelper field mismatch, expected %s, found %s", tc.hostExpect.CredHelper, tc.host.CredHelper)
			}
			if tc.host.CredExpire != tc.hostExpect.CredExpire {
				t.Errorf("credExCredExpire field mismatch, expected %s, found %s", time.Duration(tc.hostExpect.CredExpire).String(), time.Duration(tc.host.CredExpire).String())
			}
			if tc.host.PathPrefix != tc.hostExpect.PathPrefix {
				t.Errorf("pathPrefix field mismatch, expected %s, found %s", tc.hostExpect.PathPrefix, tc.host.PathPrefix)
			}
			if tc.host.Priority != tc.hostExpect.Priority {
				t.Errorf("priority field mismatch, expected %d, found %d", tc.hostExpect.Priority, tc.host.Priority)
			}
			if tc.host.BlobChunk != tc.hostExpect.BlobChunk {
				t.Errorf("blobChunk field mismatch, expected %d, found %d", tc.hostExpect.BlobChunk, tc.host.BlobChunk)
			}
			if tc.host.BlobMax != tc.hostExpect.BlobMax {
				t.Errorf("blobMax field mismatch, expected %d, found %d", tc.hostExpect.BlobMax, tc.host.BlobMax)
			}
			if len(tc.host.Mirrors) != len(tc.hostExpect.Mirrors) {
				t.Errorf("mirrors length mismatch, expected %v, found %v", tc.hostExpect.Mirrors, tc.host.Mirrors)
			} else {
				for i := range tc.host.Mirrors {
					if tc.host.Mirrors[i] != tc.hostExpect.Mirrors[i] {
						t.Errorf("mirrors field %d mismatch, expected %s, found %s", i, tc.hostExpect.Mirrors[i], tc.host.Mirrors[i])
					}
				}
			}
			if len(tc.host.APIOpts) != len(tc.hostExpect.APIOpts) {
				t.Errorf("apiOpts length mismatch, expected %v, found %v", tc.hostExpect.APIOpts, tc.host.APIOpts)
			} else {
				for i := range tc.host.APIOpts {
					if tc.host.APIOpts[i] != tc.hostExpect.APIOpts[i] {
						t.Errorf("apiOpts field %s mismatch, expected %s, found %s", i, tc.hostExpect.APIOpts[i], tc.host.APIOpts[i])
					}
				}
			}
			cred := tc.host.GetCred()
			if tc.credExpect.User != cred.User {
				t.Errorf("cred user field mismatch, expected %s, found %s", tc.credExpect.User, cred.User)
			}
			if tc.credExpect.Password != cred.Password {
				t.Errorf("cred password field mismatch, expected %s, found %s", tc.credExpect.Password, cred.Password)
			}
			if tc.credExpect.Token != cred.Token {
				t.Errorf("cred token field mismatch, expected %s, found %s", tc.credExpect.Token, cred.Token)
			}
		})
	}
}
