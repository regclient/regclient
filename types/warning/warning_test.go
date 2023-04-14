package warning

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestWarning(t *testing.T) {
	msg1 := "test 1"
	msg2 := "test 2"
	ctxBase := context.Background()
	bufBase := &bytes.Buffer{}
	logBase := logrus.New()
	logBase.SetOutput(bufBase)
	logBase.SetLevel(logrus.InfoLevel)
	bufWarn := &bytes.Buffer{}
	logWarn := logrus.New()
	logWarn.SetOutput(bufWarn)
	logWarn.SetLevel(logrus.InfoLevel)
	wWarn := &Warning{Hook: NewHook(logWarn)}
	ctxWarn := NewContext(ctxBase, wWarn)
	bufEmpty := &bytes.Buffer{}
	logEmpty := logrus.New()
	logEmpty.SetOutput(bufEmpty)
	logEmpty.SetLevel(logrus.InfoLevel)

	// run without context
	Handle(ctxBase, logBase, msg1)
	Handle(ctxBase, logBase, msg2)
	Handle(ctxBase, logBase, msg1)
	Handle(ctxBase, logBase, msg2)

	// run with context
	Handle(ctxWarn, logEmpty, msg1)
	Handle(ctxWarn, logEmpty, msg2)
	Handle(ctxWarn, logEmpty, msg1)
	Handle(ctxWarn, logEmpty, msg2)

	// check content of base buf and warn buf
	linesBase := strings.Split(strings.TrimSpace(bufBase.String()), "\n")
	if len(linesBase) != 4 {
		t.Errorf("base logs expected 4 entries, received %d: %v", len(linesBase), linesBase)
	} else {
		if !strings.Contains(linesBase[0], msg1) {
			t.Errorf("base log message 1, expected %s, received %s", msg1, linesBase[0])
		}
		if !strings.Contains(linesBase[1], msg2) {
			t.Errorf("base log message 2, expected %s, received %s", msg2, linesBase[1])
		}
	}
	linesWarn := strings.Split(strings.TrimSpace(bufWarn.String()), "\n")
	if len(linesWarn) != 2 {
		t.Errorf("base logs expected 2 entries, received %d: %v", len(linesWarn), linesWarn)
	} else {
		if !strings.Contains(linesWarn[0], msg1) {
			t.Errorf("warn log message 1, expected %s, received %s", msg1, linesWarn[0])
		}
		if !strings.Contains(linesWarn[1], msg2) {
			t.Errorf("warn log message 2, expected %s, received %s", msg2, linesWarn[1])
		}
	}
	if bufEmpty.Len() != 0 {
		t.Errorf("warn wrote to log instead of handle: %v", bufEmpty.String())
	}

	// check warn list
	if len(wWarn.List) != 2 {
		t.Errorf("warn list did not contain 2 entries: %v", wWarn.List)
	} else {
		if wWarn.List[0] != msg1 {
			t.Errorf("warn list message 1, expected %s, received %s", msg1, wWarn.List[0])
		}
		if wWarn.List[1] != msg2 {
			t.Errorf("warn list message 2, expected %s, received %s", msg2, wWarn.List[1])
		}
	}
}
