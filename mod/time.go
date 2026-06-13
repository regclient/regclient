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

package mod

import (
	"time"
)

// timeModOpt adjusts time t according to the opts.
// The bool indicates if the time was changed.
func timeModOpt(t time.Time, opt OptTime) (time.Time, bool) {
	if !opt.After.IsZero() && !t.After(opt.After) {
		return t, false
	}
	if !t.Equal(opt.Set) {
		return opt.Set, true
	}
	return t, false
}
