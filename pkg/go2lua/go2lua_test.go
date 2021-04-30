package go2lua

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"
)

type testStructNest struct {
	I8      int8
	BP      *bool
	private string
}

type testStruct struct {
	I       int `json:"i,omitempty"`
	F       float64
	S       string
	B       bool
	SP      *testStructNest
	SL      []uint
	UI      uint
	NP      *bool
	TD      time.Duration
	TN      *time.Time
	private string
}

func TestExportImport(t *testing.T) {
	ls := lua.NewState()
	b := false
	dur, _ := time.ParseDuration("1h")
	now := time.Now()
	tsIn := testStruct{
		I: 42,
		F: 3.14159,
		S: "hello world",
		B: true,
		SP: &testStructNest{
			I8: 8,
			BP: &b,
		},
		SL:      []uint{2, 4, 6, 8},
		UI:      256,
		TD:      dur,
		TN:      &now,
		private: "hidden",
	}

	jsonIn, err := json.Marshal(tsIn)
	if err != nil {
		t.Errorf("Failed to marshal test struct in: %v", err)
	}
	lv := Export(ls, tsIn)

	tsOut := &testStruct{}
	err = Import(ls, lv, tsOut, &tsIn)
	if err != nil {
		t.Errorf("Import failed: %v", err)
	}

	jsonOut, err := json.Marshal(tsOut)
	if err != nil {
		t.Errorf("Failed to marshal test struct out: %v", err)
	}

	if bytes.Compare(jsonIn, jsonOut) != 0 {
		t.Errorf("Test structs do not match: %s != %s", jsonIn, jsonOut)
	}

	t.Logf("Resulting json: %s", jsonOut)
}
