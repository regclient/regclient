// Copyright the regclient contributors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ascii

import (
	"bytes"
	"testing"
)

func TestProgress(t *testing.T) {
	t.Parallel()
	// TODO: test scenarios and compare output
	b := make([]byte, 0, 1024)
	buf := bytes.NewBuffer(b)
	bar := NewProgressBar(buf)
	bar.Width = 30
	bar.Max = 20
	tt := []struct {
		name      string
		bar       *ProgressBar
		pct       float64
		pre, post string
		expect    []byte
	}{
		{
			name:   "zero",
			bar:    bar,
			pct:    0,
			expect: []byte("[>                   ]\n"),
		},
		{
			name:   "pre/post",
			bar:    bar,
			pct:    0,
			pre:    "Prefix ",
			post:   " Suffix",
			expect: []byte("Prefix [>             ] Suffix\n"),
		},
		{
			name:   "10",
			bar:    bar,
			pct:    0.1,
			expect: []byte("[==>                 ]\n"),
		},
		{
			name:   "99",
			bar:    bar,
			pct:    0.99,
			expect: []byte("[===================>]\n"),
		},
		{
			name:   "100",
			bar:    bar,
			pct:    1.0,
			expect: []byte("[====================]\n"),
		},
		{
			name:   "negative",
			bar:    bar,
			pct:    -1,
			expect: []byte("[>                   ]\n"),
		},
		{
			name:   "200",
			bar:    bar,
			pct:    2.0,
			expect: []byte("[====================]\n"),
		},
		{
			name: "zero width",
			bar: &ProgressBar{
				Min:     10,
				Max:     20,
				Out:     buf,
				Start:   '[',
				End:     ']',
				Done:    '=',
				Active:  '>',
				Pending: ' ',
			},
			pct:    0,
			expect: []byte("[>         ]\n"),
		},
		{
			name: "change vars",
			bar: &ProgressBar{
				Min:     10,
				Max:     20,
				Out:     buf,
				Start:   '<',
				End:     '>',
				Done:    'O',
				Active:  '+',
				Pending: '-',
			},
			pct:    0.5,
			expect: []byte("<OOOOO+---->\n"),
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.bar.Generate(tc.pct, tc.pre, tc.post)
			if !bytes.Equal(result, tc.expect) {
				t.Errorf("expected %s, received %s", tc.expect, result)
			}
		})
	}
}
