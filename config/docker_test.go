package config

import (
	"os"
	"testing"
)

func TestDocker(t *testing.T) {
	curPath := os.Getenv("PATH")
	os.Setenv("PATH", "testdata"+string(os.PathListSeparator)+curPath)
	defer os.Setenv("PATH", curPath)
	curDockerConf := os.Getenv(dockerEnv)
	os.Setenv(dockerEnv, "testdata/docker-config.json")
	if curDockerConf != "" {
		defer os.Setenv(dockerEnv, curDockerConf)
	} else {
		defer os.Unsetenv(dockerEnv)
	}
	hosts, err := DockerLoad()
	if err != nil {
		t.Errorf("error loading docker credentials: %v", err)
	}
	hostMap := map[string]*Host{}
	for _, h := range hosts {
		h := h // shadow h for unique var/pointer
		hostMap[h.Name] = &h
	}
	tests := []struct {
		name       string
		hostname   string
		expectUser string
		expectPass string
	}{
		{
			name:       "testhost",
			hostname:   "testhost.example.com",
			expectUser: "hello",
			expectPass: "world",
		},
		{
			name:       "localhost:5001",
			hostname:   "localhost:5001",
			expectUser: "hello",
			expectPass: "docker",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, ok := hostMap[tt.hostname]
			if !ok {
				t.Errorf("host not found: %s", tt.hostname)
				return
			}
			cred := h.GetCred()
			if tt.expectUser != cred.User {
				t.Errorf("user mismatch, expect %s, received %s", tt.expectUser, cred.User)
			}
			if tt.expectPass != cred.Password {
				t.Errorf("user mismatch, expect %s, received %s", tt.expectPass, cred.Password)
			}
		})
	}
}
