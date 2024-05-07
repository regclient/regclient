// Package ascii is used to output ascii content to a terminal
package ascii

import (
	"io"

	"golang.org/x/term"
)

func IsWriterTerminal(w io.Writer) bool {
	wFd, ok := w.(interface{ Fd() uintptr })
	if !ok {
		return false
	}
	return term.IsTerminal(int(wFd.Fd()))
}
