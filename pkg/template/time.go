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
