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
