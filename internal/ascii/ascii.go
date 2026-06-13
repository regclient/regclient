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

// Package ascii is used to output ascii content to a terminal
package ascii

import (
	"io"
	"math"

	"golang.org/x/term"
)

func IsWriterTerminal(w io.Writer) bool {
	wFd, ok := w.(interface{ Fd() uintptr })
	if !ok {
		return false
	}
	//#nosec G115 false positive
	return wFd.Fd() <= math.MaxInt && term.IsTerminal(int(wFd.Fd()))
}
