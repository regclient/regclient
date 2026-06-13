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

package units

import "testing"

func TestHuman(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		size   float64
		result string
	}{
		{
			name:   "zero",
			size:   0,
			result: "0.000B",
		},
		{
			name:   "1.024kB",
			size:   1024,
			result: "1.024kB",
		},
		{
			name:   "1MB",
			size:   1000099,
			result: "1.000MB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HumanSize(tt.size)
			if result != tt.result {
				t.Errorf("expected %s, received %s", tt.result, result)
			}
		})
	}
}
