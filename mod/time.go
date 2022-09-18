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
