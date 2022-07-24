package diff

// Recommended reading:
// https://blog.jcoglan.com/2017/02/17/the-myers-diff-algorithm-part-3/
// https://www.codeproject.com/Articles/42279/%2FArticles%2F42279%2FInvestigating-Myers-diff-algorithm-Part-1-of-2
// https://cs.opensource.google/go/x/tools/+/refs/tags/v0.1.11:internal/lsp/diff/myers/diff.go;l=19

// myersOperations returns the list of operations to convert a into b.
// This consolidates operations for multiple lines and skips equal lines.
func myersOperations(a, b []string) []*operation {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	trace, offset := myersShortestSeq(a, b)
	snakes := myersBacktrack(trace, len(a), len(b), offset)
	M, N := len(a), len(b)
	var i int
	solution := make([]*operation, len(a)+len(b))
	add := func(op *operation, x2, y2 int) {
		if op == nil {
			return
		}
		if i > 0 && solution[i-1].Kind == op.Kind && solution[i-1].X2 == op.X1 && solution[i-1].Y2 == op.Y1 {
			// extend add/delete from previous entry
			solution[i-1].X2 = x2
			solution[i-1].Y2 = y2
		} else {
			// add a new operation
			op.X2 = x2
			op.Y2 = y2
			solution[i] = op
			i++
		}
	}
	x, y := 0, 0
	for _, snake := range snakes {
		if len(snake) < 2 {
			continue
		}
		if snake[0]-snake[1] > x-y {
			// delete (horizontal)
			op := &operation{
				Kind: OpDelete,
				X1:   x,
				Y1:   y,
			}
			x++
			if x <= M {
				add(op, x, y)
			}
		} else if snake[0]-snake[1] < x-y {
			// insert (vertical)
			op := &operation{
				Kind: OpInsert,
				X1:   x,
				Y1:   y,
			}
			y++
			if y <= N {
				add(op, x, y)
			}
		}
		// equal (diagonal)
		for x < snake[0] {
			x++
			y++
		}
		if x >= M && y >= N {
			break
		}
	}
	return solution[:i]
}

// myersBacktrack returns a list of "snakes" for a given trace.
// A "snake" is a single deletion or insertion followed by zero or more diagonals.
// snakes[d] is the x,y coordinate of the best position on the best path at distance d.
func myersBacktrack(trace [][]int, x, y, offset int) [][]int {
	snakes := make([][]int, len(trace))
	d := len(trace) - 1
	for ; x >= 0 && y >= 0 && d > 0; d-- {
		V := trace[d]
		if len(V) == 0 {
			continue
		}
		snakes[d] = []int{x, y}

		k := x - y

		var kPrev int
		if k == -d || (k != d && V[k-1+offset] < V[k+1+offset]) {
			kPrev = k + 1
		} else {
			kPrev = k - 1
		}

		x = V[kPrev+offset]
		y = x - kPrev
	}
	if x < 0 || y < 0 {
		return snakes
	}
	snakes[d] = []int{x, y}
	return snakes
}

// myersShortestSeq returns the shortest edit sequence that converts a into b.
// M and N, length of a and b respectively.
// x: index of a, x+1 moves right, indicating deletion from a.
// y: index of b, y+1 moves down, indicating insertion from b.
// k: diagonals represented by the equation y = x - k. If inserts==deletes, k=0.
// V[k]=x: best values of x for each k diagonal.
// d: distance, sum of inserts/deletes.
// trace[d]=V, best values for x for each k diagonal and distance d.
// return is the trace and offset
func myersShortestSeq(a, b []string) ([][]int, int) {
	M, N := len(a), len(b)
	V := make([]int, 2*(N+M)+1)
	offset := N + M
	trace := make([][]int, N+M+1)
	// iterate up to the maximum possible length
	for d := 0; d <= N+M; d++ {
		newV := make([]int, len(V))
		// move in increments of 2 because end points for even d are on even k lines
		for k := -d; k <= d; k += 2 {
			// At each point, we either go down or to the right.
			// We go down if k == -d, and we go to the right if k == d.
			// We also prioritize the maximum x value, because we prefer deletions to insertions.
			var x int
			if k == -d || (k != d && V[k-1+offset] < V[k+1+offset]) {
				x = V[k+1+offset] // down
			} else {
				x = V[k-1+offset] + 1 // right
			}
			y := x - k
			// Diagonal moves while we have equal contents.
			for x < M && y < N && a[x] == b[y] {
				x++
				y++
			}
			V[k+offset] = x
			// Return if we've exceeded the maximum values.
			if x == M && y == N {
				// Makes sure to save the state of the array before returning.
				copy(newV, V)
				trace[d] = newV
				return trace, offset
			}
		}
		// Save the state of the array.
		copy(newV, V)
		trace[d] = newV
	}
	return nil, 0
}
