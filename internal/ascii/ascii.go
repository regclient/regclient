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
