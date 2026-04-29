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
