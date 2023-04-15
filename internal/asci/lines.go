package asci

import (
	"bytes"
	"fmt"
	"io"
)

type Lines struct {
	atStart bool
	buf     []byte
	lines   int
	out     io.Writer
}

func NewLines(w io.Writer) *Lines {
	return &Lines{
		buf: []byte{},
		out: w,
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
	b.out.Write(b.buf)
	b.lines = bytes.Count(b.buf, []byte("\n"))
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
