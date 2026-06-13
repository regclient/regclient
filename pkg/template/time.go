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
	"time"
)

// // TimeFunc provides the "time" template, returning a struct with methods
// func TimeFunc() *TimeFuncs {
// 	return &TimeFuncs{}
// }

// TimeFuncs wraps all time based templates
type TimeFuncs struct{}

// Now returns current time
func (t *TimeFuncs) Now() time.Time {
	return time.Now()
}

// Parse parses the current time according to layout
func (t *TimeFuncs) Parse(layout string, value string) (time.Time, error) {
	return time.Parse(layout, value)
}
