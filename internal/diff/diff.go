// Package diff computes the efficient set of changes (insert/delete) between two arrays of strings
package diff

import "fmt"

// opKind is used to denote the type of operation a line represents.
type opKind int

const (
	// OpDelete is the operation kind for a line that is present in the input
	// but not in the output.
	OpDelete opKind = iota
	// OpInsert is the operation kind for a line that is new in the output.
	OpInsert
)

type operation struct {
	Kind   opKind
	X1, X2 int // indices of the line in a
	Y1, Y2 int // indices of the line in b
}

type Opt func(*conf)

type conf struct {
	contextA    int
	contextB    int
	contextFull bool
}

func WithContext(a, b int) func(*conf) {
	return func(c *conf) {
		c.contextA = a
		c.contextB = b
	}
}

func WithFullContext() func(*conf) {
	return func(c *conf) {
		c.contextFull = true
	}
}

// Diff returns the difference between two strings
func Diff(a, b []string, opts ...Opt) []string {
	c := conf{}
	for _, fn := range opts {
		fn(&c)
	}

	diffLines := []string{}
	setLines := []string{}
	ops := myersOperations(a, b)
	sX1, sX2, sY1, sY2 := -1, -1, -1, -1
	addSet := func() {
		if len(setLines) == 0 {
			return
		}
		// calculate how many lines of context to add
		cA, cB := c.contextA, c.contextB
		if sX1-cA < 0 || c.contextFull {
			cA = sX1
		}
		if sX2+cB > len(a) || c.contextFull {
			cB = len(a) - sX2
		}
		// add header
		diffLines = append(diffLines, fmt.Sprintf("@@ -%d,%d +%d,%d @@", sX1-cA+1, sX2+cA+cB-sX1, sY1-cA+1, sY2+cA+cB-sY1))
		// add context before, the change set, and context after
		if cA > 0 {
			for _, line := range a[sX1-cA : sX1] {
				diffLines = append(diffLines, "  "+line)
			}
		}
		diffLines = append(diffLines, setLines...)
		setLines = []string{} // reset the setLines to a new array
		if cB > 0 {
			for _, line := range a[sX2 : sX2+cB] {
				diffLines = append(diffLines, "  "+line)
			}
		}
	}
	for _, op := range ops {
		// compare from last set
		dX, dY := op.X1-sX2, op.Y1-sY2
		if dX != dY || (dX > c.contextA && dX > c.contextB && !c.contextFull) {
			// unexpected diff lines or gap exceeds context limits, create a new set
			addSet()
			sX1, sY1 = op.X1, op.Y1
		} else if dX > 0 {
			// add common lines between two diffs
			for _, line := range a[sX2+1 : op.X1] {
				setLines = append(setLines, "  "+line)
			}
		}
		// add entries to this set, either delete or add
		switch op.Kind {
		case OpDelete:
			for _, line := range a[op.X1:op.X2] {
				setLines = append(setLines, "- "+line)
			}
		case OpInsert:
			for _, line := range b[op.Y1:op.Y2] {
				setLines = append(setLines, "+ "+line)
			}
		}
		// update end of set
		sX2, sY2 = op.X2, op.Y2
	}
	addSet()
	return diffLines
}
