package ascii

import (
	"bytes"
	"io"
	"testing"
)

func TestLinesWidthZero(t *testing.T) {
	t.Parallel()
	b := make([]byte, 0, 1024)
	buf := bytes.NewBuffer(b)
	l := NewLines(buf)
	l.width = 0
	// test clear
	l.Clear()
	out, err := io.ReadAll(buf)
	if err != nil {
		t.Fatalf("failed to read buffer: %v", err)
	}
	expect := []byte("\033[0J")
	if !bytes.Equal(out, expect) {
		t.Errorf("initial clear, expected %q, received %q", expect, out)
	}
	// test flush
	l.Flush()
	out, err = io.ReadAll(buf)
	if err != nil {
		t.Fatalf("failed to read buffer: %v", err)
	}
	expect = []byte("\033[0J")
	if !bytes.Equal(out, expect) {
		t.Errorf("empty flush, expected %q, received %q", expect, out)
	}
	// test add*2 + flush
	l.Add([]byte("hello\n"))
	l.Add([]byte("world\n"))
	expect = []byte("\033[0Jhello\nworld\n")
	l.Flush()
	out, err = io.ReadAll(buf)
	if err != nil {
		t.Fatalf("failed to read buffer: %v", err)
	}
	if !bytes.Equal(out, expect) {
		t.Errorf("two adds + flush, expected %q, received %q", expect, out)
	}
	// test another add + flush
	expect = []byte("foo\nbar\n")
	l.Add(expect)
	expect = append([]byte("\033[2F\033[0J"), expect...)
	l.Flush()
	out, err = io.ReadAll(buf)
	if err != nil {
		t.Fatalf("failed to read buffer: %v", err)
	}
	if !bytes.Equal(out, expect) {
		t.Errorf("another add + flush, expected %q, received %q", expect, out)
	}
	// test add + delete
	l.Add([]byte("bar\nbaz\n"))
	l.Del()
	l.Flush()
	out, err = io.ReadAll(buf)
	if err != nil {
		t.Fatalf("failed to read buffer: %v", err)
	}
	expect = []byte("\033[2F\033[0J")
	if !bytes.Equal(out, expect) {
		t.Errorf("add + del + flush, expected %q, received %q", expect, out)
	}
}

func TestLinesWidthSet(t *testing.T) {
	t.Parallel()
	b := make([]byte, 0, 1024)
	buf := bytes.NewBuffer(b)
	l := NewLines(buf)
	l.width = 10
	// test clear
	l.Clear()
	out, err := io.ReadAll(buf)
	if err != nil {
		t.Fatalf("failed to read buffer: %v", err)
	}
	expect := []byte("\033[0J")
	if !bytes.Equal(out, expect) {
		t.Errorf("initial clear, expected %q, received %q", expect, out)
	}
	// test flush
	l.Flush()
	out, err = io.ReadAll(buf)
	if err != nil {
		t.Fatalf("failed to read buffer: %v", err)
	}
	expect = []byte("\033[0J")
	if !bytes.Equal(out, expect) {
		t.Errorf("empty flush, expected %q, received %q", expect, out)
	}
	// test add*2 + flush
	l.Add([]byte("hello\n"))
	l.Add([]byte("world this is a long line\n"))
	expect = []byte("\033[0Jhello\nworld this is a long line\n")
	l.Flush()
	out, err = io.ReadAll(buf)
	if err != nil {
		t.Fatalf("failed to read buffer: %v", err)
	}
	if !bytes.Equal(out, expect) {
		t.Errorf("two adds + flush, expected %q, received %q", expect, out)
	}
	// test another add + flush
	expect = []byte("foo\nbar to ten\n")
	l.Add(expect)
	expect = append([]byte("\033[4F\033[0J"), expect...)
	l.Flush()
	out, err = io.ReadAll(buf)
	if err != nil {
		t.Fatalf("failed to read buffer: %v", err)
	}
	if !bytes.Equal(out, expect) {
		t.Errorf("another add + flush, expected %q, received %q", expect, out)
	}
	// test add + delete
	l.Add([]byte("bar\nbaz\n"))
	l.Del()
	l.Flush()
	out, err = io.ReadAll(buf)
	if err != nil {
		t.Fatalf("failed to read buffer: %v", err)
	}
	expect = []byte("\033[2F\033[0J")
	if !bytes.Equal(out, expect) {
		t.Errorf("add + del + flush, expected %q, received %q", expect, out)
	}
}
