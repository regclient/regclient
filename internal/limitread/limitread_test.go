package limitread

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/regclient/regclient/types"
)

func TestLimitRead(t *testing.T) {
	t.Parallel()
	byte0 := []byte("")
	byte5 := []byte("12345")
	byte10 := []byte("1234567890")
	tt := []struct {
		name        string
		limit       int64
		src         []byte
		try         int64
		expectBytes []byte
		expectLen   int
		expectErr   error
	}{
		{
			name:        "empty",
			limit:       0,
			src:         byte0,
			try:         0,
			expectBytes: byte0,
			expectLen:   0,
			expectErr:   io.EOF,
		},
		{
			name:        "exact length",
			limit:       5,
			src:         byte5,
			try:         5,
			expectBytes: byte5,
			expectLen:   5,
			expectErr:   nil,
		},
		{
			name:        "read less",
			limit:       5,
			src:         byte10,
			try:         5,
			expectBytes: byte5,
			expectLen:   5,
			expectErr:   nil,
		},
		{
			name:        "try more",
			limit:       5,
			src:         byte5,
			try:         10,
			expectBytes: byte5,
			expectLen:   5,
			expectErr:   io.EOF,
		},
		{
			name:      "read more",
			limit:     9,
			src:       byte10,
			try:       10,
			expectErr: types.ErrSizeLimitExceeded,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			lr := LimitRead{
				Reader: bytes.NewReader(tc.src),
				Limit:  tc.limit,
			}
			tgt := make([]byte, tc.try)
			result, err := lr.Read(tgt)
			// on a short read, try again for the EOF
			if err == nil && result < int(tc.try) {
				result2, err2 := lr.Read(tgt[result:])
				result += result2
				err = err2
			}
			if tc.expectErr != nil {
				if err == nil {
					t.Errorf("read did not fail")
				} else if tc.expectErr.Error() != err.Error() && !errors.Is(err, tc.expectErr) {
					t.Errorf("unexpected error, expected %v, received %v", tc.expectErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("read failed: %v", err)
			}
			if result != tc.expectLen {
				t.Errorf("read length mismatch, expected %d, received %d", tc.expectLen, result)
			}
			if !bytes.Equal(tgt[:result], tc.expectBytes) {
				t.Errorf("read bytes mismatch, expected %s, received %s", string(tc.expectBytes), string(tgt[:result]))
			}
		})
	}
}
