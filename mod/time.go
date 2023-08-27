package mod

import (
	"errors"
	"os"
	"strconv"
	"time"
)

const epocEnv = "SOURCE_DATE_EPOC"

var (
	errInvalidEpoc = errors.New("invalid epoc var")
	timeStart      = timeNow()
)

func timeNow() time.Time {
	now, err := timeEpocEnv()
	if err == nil {
		return now
	}
	return time.Now()
}

func timeEpocEnv() (time.Time, error) {
	sec := os.Getenv(epocEnv)
	if sec == "" {
		return time.Time{}, errInvalidEpoc
	}
	secI, err := strconv.ParseInt(sec, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(secI, 0), nil
}

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
