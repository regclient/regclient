package conffile

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/regclient/regclient/internal/rwfs"
)

// test New
func TestNew(t *testing.T) {
	testEnvFileVar, testEnvFileVal := "TEST_CONFFILE_NEW", "./test-filename.json"
	os.Setenv(testEnvFileVar, testEnvFileVal)
	testEnvDirVar, testEnvDirVal := "TEST_CONFDIR_NEW", "./test-dirname"
	os.Setenv(testEnvFileVar, testEnvFileVal)
	os.Setenv(testEnvDirVar, testEnvDirVal)
	testEnvUnset := "TEST_CONFFILE_NEW_UNSET"
	os.Unsetenv(testEnvUnset)
	hd := homedir()
	tests := []struct {
		name       string
		opts       []Opt
		expectNil  bool
		expectName string
	}{
		{
			name:      "empty",
			expectNil: true,
		},
		{
			name: "fullname override",
			opts: []Opt{
				WithDirName(".config", "file.json"),
				WithEnvFile(testEnvFileVar),
				WithFullname("/tmp/conf.json"),
			},
			expectName: "/tmp/conf.json",
		},
		{
			name: "fullname only",
			opts: []Opt{
				WithFullname("/tmp/conf.json"),
			},
			expectName: "/tmp/conf.json",
		},
		{
			name: "env file override",
			opts: []Opt{
				WithDirName(".config", "file.json"),
				WithEnvFile(testEnvFileVar),
			},
			expectName: testEnvFileVal,
		},
		{
			name: "env dir override",
			opts: []Opt{
				WithDirName(".config", "file.json"),
				WithEnvDir(testEnvDirVar, "file.json"),
			},
			expectName: filepath.Join(testEnvDirVal, "file.json"),
		},
		{
			name: "dir name",
			opts: []Opt{
				WithDirName(".config", "file.json"),
			},
			expectName: filepath.Join(hd, ".config", "file.json"),
		},
		{
			name: "env unset",
			opts: []Opt{
				WithDirName(".config", "file.json"),
				WithEnvFile(testEnvUnset),
			},
			expectName: filepath.Join(hd, ".config", "file.json"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cf := New(tt.opts...)
			if tt.expectNil {
				if cf != nil {
					t.Errorf("result was not nil: %v", cf)
				}
				return
			}
			if cf == nil {
				t.Errorf("result was nil")
				return
			}
			if cf.Name() != tt.expectName {
				t.Errorf("fullname mismatch, expected %s, received %s", tt.expectName, cf.Name())
			}
		})
	}
}

// TestWriteOpen test Write and Open using MemFS
func TestWriteOpen(t *testing.T) {
	memfs := rwfs.MemNew()
	confContent := []byte("hello test")
	cf := New(WithFS(memfs), WithFullname("test.json"))
	err := cf.Write(bytes.NewReader(confContent))
	if err != nil {
		t.Errorf("failed to write config file: %v", err)
		return
	}
	rc, err := cf.Open()
	if err != nil {
		t.Errorf("failed to open config file: %v", err)
		return
	}
	defer rc.Close()
	readBytes, err := io.ReadAll(rc)
	if err != nil {
		t.Errorf("failed to read content: %v", err)
		return
	}
	if !bytes.Equal(readBytes, confContent) {
		t.Errorf("content mismatch, write: %s, read: %s", string(confContent), string(readBytes))
	}
}
