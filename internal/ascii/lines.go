package ascii

import (
	"bytes"
	"fmt"
	"io"

	"golang.org/x/term"
)

type Lines struct {
	atStart bool
	buf     []byte
	lines   int
	out     io.Writer
	width   int
}

func NewLines(w io.Writer) *Lines {
	width := 0
	if wFd, ok := w.(interface{ Fd() uintptr }); ok && term.IsTerminal(int(wFd.Fd())) {
		w, _, err := term.GetSize(int(wFd.Fd()))
		if err == nil {
			width = w
		}
	}

	return &Lines{
		buf:   []byte{},
		out:   w,
		width: width,
	}
}

func (b *Lines) Add(add []byte) {
	b.buf = append(b.buf, add...)
}

func (b *Lines) Del() {
	b.buf = b.buf[:0]
}

func (b *Lines) Flush() {
	b.Clear()
	_, err := b.out.Write(b.buf)
	if err != nil {
		return
	}
	b.lines = bytes.Count(b.buf, []byte("\n"))
	if b.width > 0 {
		for _, line := range bytes.Split(b.buf, []byte("\n")) {
			if len(line) > b.width {
				b.lines += (len(line) - 1) / b.width
			}
		}
	}
	b.buf = b.buf[:0]
	b.atStart = false
}

func (b *Lines) Clear() {
	if !b.atStart {
		b.Return()
	}
	fmt.Fprintf(b.out, "\033[0J")
	b.atStart = true
	b.lines = 0
}

func (b *Lines) Return() {
	if !b.atStart && b.lines > 0 {
		fmt.Fprintf(b.out, "\033[%dF", b.lines)
	}
	b.atStart = true
}
