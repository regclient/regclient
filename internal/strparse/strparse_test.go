package strparse

import (
	"errors"
	"testing"

	"github.com/regclient/regclient/types"
)

func TestSplitCSKV(t *testing.T) {
	tt := []struct {
		name   string
		str    string
		result map[string]string
		err    error
	}{
		{
			name:   "empty",
			result: map[string]string{},
		},
		{
			name: "single",
			str:  "key=value",
			result: map[string]string{
				"key": "value",
			},
		},
		{
			name: "multiple",
			str:  "a=123,bcd=456",
			result: map[string]string{
				"a":   "123",
				"bcd": "456",
			},
		},
		{
			name: "quote",
			str:  `a="123,456",b=789`,
			result: map[string]string{
				"a": "123,456",
				"b": "789",
			},
		},
		{
			name: "escape",
			str:  `a\\d=123\,\"\\456,"b\\,=c"="7\\,89"`,
			result: map[string]string{
				"a\\d":   `123,"\456`,
				"b\\,=c": "7\\,89",
			},
		},
		{
			name: "noValue",
			str:  "a,b",
			result: map[string]string{
				"a": "",
				"b": "",
			},
		},
		{
			name: "errEscapeKey",
			str:  "a\\",
			err:  types.ErrParsingFailed,
		},
		{
			name: "errEscapeVal",
			str:  "a=x\\",
			err:  types.ErrParsingFailed,
		},
		{
			name: "errQuoteKey",
			str:  "a\"",
			err:  types.ErrParsingFailed,
		},
		{
			name: "errQuoteVal",
			str:  "a=b\"",
			err:  types.ErrParsingFailed,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			result, err := SplitCSKV(tc.str)
			if tc.err != nil {
				if err == nil {
					t.Errorf("did not fail")
				} else if err.Error() != tc.err.Error() && !errors.Is(err, tc.err) {
					t.Errorf("unexpected error, expected %v, received %v", tc.err, err)
				}
				return
			} else if err != nil {
				t.Fatalf("unexpected error, received %v", err)
			}
			for k, v := range tc.result {
				if result[k] != v {
					t.Errorf("unexpected result for key %s, expected %s, received %s", k, v, result[k])
				}
			}
			for k, v := range result {
				if _, ok := tc.result[k]; !ok {
					t.Errorf("unexpected key, %s = %s", k, v)
				}
			}
		})
	}
}
