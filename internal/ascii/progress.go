package ascii

import (
	"fmt"
	"io"

	"golang.org/x/term"
)

type ProgressBar struct {
	Width, Min, Max                   int
	Start, Done, Active, Pending, End byte
	Out                               io.Writer
}

func NewProgressBar(w io.Writer) *ProgressBar {
	width := 0
	if wFd, ok := w.(interface{ Fd() uintptr }); ok && term.IsTerminal(int(wFd.Fd())) {
		w, _, err := term.GetSize(int(wFd.Fd()))
		if err == nil {
			width = w
		}
	}

	return &ProgressBar{
		Width:   width,
		Min:     10,
		Max:     40,
		Out:     w,
		Start:   '[',
		Done:    '=',
		Active:  '>',
		Pending: ' ',
		End:     ']',
	}
}

func (p *ProgressBar) Generate(pct float64, pre, post string) []byte {
	if pct < 0 {
		pct = 0
	} else if pct > 1 {
		pct = 1
	}
	curWidth := p.Width - (len(pre) + len(post) + 2)
	curWidth = min(max(curWidth, p.Min), p.Max)
	buf := make([]byte, curWidth)

	doneLen := int(float64(curWidth) * pct)
	for i := range doneLen {
		buf[i] = p.Done
	}
	if doneLen < curWidth {
		buf[doneLen] = p.Active
	}
	for i := doneLen + 1; i < curWidth; i++ {
		buf[i] = p.Pending
	}
	return fmt.Appendf(nil, "%s%c%s%c%s\n", pre, p.Start, buf, p.End, post)
}
