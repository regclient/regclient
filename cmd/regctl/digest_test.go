package main

import (
	"strings"
	"testing"

	"github.com/regclient/regclient/pkg/archive"
)

func TestDigestDecompress(t *testing.T) {
	// Run to confirm:  echo -n "hello" | gzip | gzcat | shasum -a 256
	expectedChecksum := "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	compressionTypes := []archive.CompressType{
		// archive.CompressBzip2,
		archive.CompressZstd,
		archive.CompressGzip,
		archive.CompressXz,
	}
	for _, compressionType := range compressionTypes {
		reader, err := archive.Compress(strings.NewReader("hello"), compressionType)
		if err != nil {
			t.Fatalf("failed to compress test data: %v", err)
		}
		checksum, err := cobraTest(t, &cobraTestOpts{
			stdin: reader,
		}, "digest", "--decompress")
		if err != nil {
			t.Fatalf("failed to run digest: %v", err)
		}
		if checksum != expectedChecksum {
			t.Errorf("unexpected output: %v %v", checksum, expectedChecksum)
		}
	}
}
