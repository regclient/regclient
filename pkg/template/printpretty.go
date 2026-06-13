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

package template

import (
	"bytes"
	"encoding/json"
)

type prettyPrinter interface {
	MarshalPretty() ([]byte, error)
}

func printPretty(v any) string {
	if pp, ok := v.(prettyPrinter); ok {
		b, err := pp.MarshalPretty()
		if err != nil {
			return ""
		}
		return string(b)
	}
	// fall through for objects without a prettyPrinter interface
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
	return buf.String()
}
