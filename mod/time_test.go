package mod

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestTimeNow(t *testing.T) {
	t.Run("NoEnv", func(t *testing.T) {
		curEnv, envIsSet := os.LookupEnv(epocEnv)
		if envIsSet {
			err := os.Unsetenv(epocEnv)
			if err != nil {
				t.Errorf("failed to unset %s", epocEnv)
				return
			}
			defer os.Setenv(epocEnv, curEnv)
		}
		curTimeNow := timeNow()
		if curTimeNow.After(time.Now()) {
			t.Error("timeNow reported a time after OS time now")
		}
	})
	t.Run("WithEnv", func(t *testing.T) {
		timePrev := time.Now().Add(-1 * time.Hour).Round(time.Second)
		timeSec := fmt.Sprintf("%d", timePrev.Unix())
		t.Setenv(epocEnv, timeSec)
		curTimeNow := timeNow()
		if !curTimeNow.Equal(timePrev) {
			t.Errorf("timeNow did not use the epoc, expected %d, received %d", timePrev.Unix(), curTimeNow.Unix())
		}
	})
}
