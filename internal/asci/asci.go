// Package asci is used to output asci content to a terminal
package asci

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
