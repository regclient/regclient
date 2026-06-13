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

func TestIsWriterTerminal(t *testing.T) {
	t.Parallel()
	b := make([]byte, 10)
	buf := bytes.NewBuffer(b)
	if IsWriterTerminal(buf) {
		t.Errorf("buffer should not be a terminal")
	}
}
