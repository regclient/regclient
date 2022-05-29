package timejson

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestMarshal(t *testing.T) {
	tests := []struct {
		name   string
		d      Duration
		expect string
	}{
		{
			name:   "second",
			d:      Duration(time.Second),
			expect: `"1s"`,
		},
		{
			name:   "hour",
			d:      Duration(time.Hour),
			expect: `"1h0m0s"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := tt.d.MarshalJSON()
			if err != nil {
				t.Errorf("failed marshaling: %v", err)
				return
			}
			if !bytes.Equal(b, []byte(tt.expect)) {
				t.Errorf("mismatch, expected %s, received %s", tt.expect, string(b))
			}
		})
	}
}

func TestUnmarshal(t *testing.T) {
	tests := []struct {
		name   string
		str    string
		expect Duration
		expErr error
	}{
		{
			name:   "unquoted string",
			str:    `1s`,
			expErr: errors.New("invalid character 's' after top-level value"),
		},
		{
			name:   "bool",
			str:    `true`,
			expErr: errInvalid,
		},
		{
			name:   "invalid duration",
			str:    `"42 years"`,
			expErr: errors.New(`time: unknown unit " years" in duration "42 years"`),
		},
		{
			name:   "second",
			str:    `"1s"`,
			expect: Duration(time.Second),
			expErr: nil,
		},
		{
			name:   "hour",
			str:    `"1h"`,
			expect: Duration(time.Hour),
			expErr: nil,
		},
		{
			name:   "second float",
			str:    fmt.Sprintf("%d", time.Second),
			expect: Duration(time.Second),
			expErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var d Duration
			err := (&d).UnmarshalJSON([]byte(tt.str))
			if tt.expErr != nil {
				if err == nil {
					t.Errorf("error not encountered")
				} else if err != tt.expErr && err.Error() != tt.expErr.Error() {
					t.Errorf("error mismatch, expected %v, received %v", tt.expErr, err)
				}
				return
			}
			if err != nil {
				t.Errorf("failed unmarshaling: %v", err)
				return
			}
			if d != tt.expect {
				t.Errorf("duration mismatch, expected %s, received %s", time.Duration(tt.expect).String(), time.Duration(d).String())
			}
		})
	}
}
