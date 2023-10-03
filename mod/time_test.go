package mod

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestTimeNow(t *testing.T) {
	t.Parallel()
	curEnv, envIsSet := os.LookupEnv(epocEnv)
	defer func() {
		if envIsSet {
			os.Setenv(epocEnv, curEnv)
		} else {
			os.Unsetenv(epocEnv)
		}
	}()

	t.Run("NoEnv", func(t *testing.T) {
		err := os.Unsetenv(epocEnv)
		if err != nil {
			t.Errorf("failed to unset %s", epocEnv)
			return
		}
		curTimeNow := timeNow()
		if curTimeNow.After(time.Now()) {
			t.Error("timeNow reported a time after OS time now")
		}
	})
	t.Run("WithEnv", func(t *testing.T) {
		timePrev := time.Now().Add(-1 * time.Hour).Round(time.Second)
		timeSec := fmt.Sprintf("%d", timePrev.Unix())
		err := os.Setenv(epocEnv, timeSec)
		if err != nil {
			t.Errorf("failed to set %s", epocEnv)
			return
		}
		curTimeNow := timeNow()
		if !curTimeNow.Equal(timePrev) {
			t.Errorf("timeNow did not use the epoc, expected %d, received %d", timePrev.Unix(), curTimeNow.Unix())
		}
	})
}
