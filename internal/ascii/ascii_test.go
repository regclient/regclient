package ascii

import (
	"bytes"
	"testing"
)

func TestIsWriterTerminal(t *testing.T) {
	b := make([]byte, 10)
	buf := bytes.NewBuffer(b)
	if IsWriterTerminal(buf) {
		t.Errorf("buffer should not be a terminal")
	}
}
