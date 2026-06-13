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

package reproducible

import (
	"fmt"
	"testing"
	"time"
)

func TestTimeNow(t *testing.T) {
	t.Run("NoEnv", func(t *testing.T) {
		t.Setenv(EpocEnv, "")
		curTimeNow := TimeNow()
		if curTimeNow.After(time.Now()) {
			t.Error("timeNow reported a time after OS time now")
		}
	})
	t.Run("WithEnv", func(t *testing.T) {
		timePrev := time.Now().Add(-1 * time.Hour).Round(time.Second)
		timeSec := fmt.Sprintf("%d", timePrev.Unix())
		t.Setenv(EpocEnv, timeSec)
		curTimeNow := TimeNow()
		if !curTimeNow.Equal(timePrev) {
			t.Errorf("timeNow did not use the epoc, expected %d, received %d", timePrev.Unix(), curTimeNow.Unix())
		}
	})
}
