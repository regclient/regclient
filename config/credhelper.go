package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// cred helper wraps a command
type credHelper struct {
	prog string
	env  map[string]string
}

func newCredHelper(prog string, env map[string]string) *credHelper {
	return &credHelper{prog: prog, env: env}
}

func (ch *credHelper) run(arg string, input io.Reader) ([]byte, error) {
	cmd := exec.Command(ch.prog, arg)
	cmd.Env = os.Environ()
	if ch.env != nil {
		for k, v := range ch.env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}
	cmd.Stderr = os.Stderr
	cmd.Stdin = input
	return cmd.Output()
}

type credStore struct {
	ServerURL string `json:"ServerURL"`
	Username  string `json:"Username"`
	Secret    string `json:"Secret"`
}

func (ch *credHelper) get(host *Host) error {
	hostIn := strings.NewReader(host.Hostname)
	credOut := credStore{
		Username: host.User,
		Secret:   host.Pass,
	}
	outB, err := ch.run("get", hostIn)
	if err != nil {
		outS := strings.TrimSpace(string(outB))
		return fmt.Errorf("error getting credentials: %s: %v", outS, err)
	}
	err = json.NewDecoder(bytes.NewReader(outB)).Decode(&credOut)
	if err != nil {
		return fmt.Errorf("error reading credentials: %w", err)
	}
	host.User = credOut.Username
	host.Pass = credOut.Secret
	host.credRefresh = time.Now().Add(time.Duration(host.CredExpire))
	return nil
}

// store and list methods not implemented
