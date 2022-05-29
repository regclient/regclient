package config

import (
	"os"
	"testing"
)

func TestCredHelper(t *testing.T) {
	testHost := "testhost.example.com"
	expectUser, expectPass := "hello", "world"
	curPath := os.Getenv("PATH")
	os.Setenv("PATH", "testdata"+string(os.PathListSeparator)+curPath)
	defer os.Setenv("PATH", curPath)
	ch := newCredHelper("docker-credential-test", map[string]string{})
	h := HostNewName(testHost)
	h.CredHelper = "docker-credential-test"
	err := ch.get(h)
	if err != nil {
		t.Errorf("error running get: %v", err)
	}
	if expectUser != h.User {
		t.Errorf("user mismatch: expected %s, received %s", expectUser, h.User)
	}
	if expectPass != h.Pass {
		t.Errorf("password mismatch: expected %s, received %s", expectPass, h.Pass)
	}
}
