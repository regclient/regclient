package config

import (
	"os"
	"testing"
)

func TestCredHelper(t *testing.T) {
	tests := []struct {
		name        string
		host        string
		credhelper  string
		expectUser  string
		expectPass  string
		expectToken string
		expectErr   bool
	}{
		{
			name:       "user/pass",
			host:       "testhost.example.com",
			credhelper: "docker-credential-test",
			expectUser: "hello",
			expectPass: "world",
		},
		{
			name:        "token",
			host:        "testtoken.example.com",
			credhelper:  "docker-credential-test",
			expectToken: "deadbeefcafe",
		},
		{
			name:       "missing helper",
			credhelper: "./testdata/docker-credential-missing",
			expectErr:  true,
		},
	}
	curPath := os.Getenv("PATH")
	os.Setenv("PATH", "testdata"+string(os.PathListSeparator)+curPath)
	defer os.Setenv("PATH", curPath)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := HostNewName(tt.host)
			h.CredHelper = tt.credhelper
			ch := newCredHelper(tt.credhelper, map[string]string{})
			err := ch.get(h)
			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error not encountered")
				}
				return
			}
			if err != nil {
				t.Errorf("error running get: %v", err)
			}
			if tt.expectUser != h.User {
				t.Errorf("user mismatch: expected %s, received %s", tt.expectUser, h.User)
			}
			if tt.expectPass != h.Pass {
				t.Errorf("password mismatch: expected %s, received %s", tt.expectPass, h.Pass)
			}
			if tt.expectToken != h.Token {
				t.Errorf("token mismatch: expected %s, received %s", tt.expectToken, h.Token)
			}
		})
	}
}
