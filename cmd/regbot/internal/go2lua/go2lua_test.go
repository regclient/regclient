// Copyright the regclient contributors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package go2lua

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"
)

type testStructNest struct {
	I8 int8
	BP *bool
	//lint:ignore U1000 intentional test of an unexported field
	private string
}

type AnonType struct {
	Ver int
}
type testStruct struct {
	AnonType
	I  int `json:"i,omitempty"`
	F  float64
	S  string
	B  bool
	SP *testStructNest
	SL []uint
	MS map[string]string
	UI uint
	NP *bool
	TD time.Duration
	TN *time.Time
	//lint:ignore U1000 intentional test of an unexported field
	private string
}

func TestExportImport(t *testing.T) {
	ls := lua.NewState()
	b := false
	dur, _ := time.ParseDuration("1h")
	now := time.Now()
	tsIn := testStruct{
		AnonType: AnonType{Ver: 2},
		I:        42,
		F:        3.14159,
		S:        "hello world",
		B:        true,
		SP: &testStructNest{
			I8: 8,
			BP: &b,
		},
		SL: []uint{2, 4, 6, 8},
		MS: map[string]string{
			"hello": "world",
			"foo":   "bar",
			"bin":   "baz",
		},
		UI:      256,
		TD:      dur,
		TN:      &now,
		private: "hidden",
	}

	jsonIn, err := json.Marshal(tsIn)
	if err != nil {
		t.Fatalf("Failed to marshal test struct in: %v", err)
	}
	lv := Export(ls, tsIn)

	// tsOut := &testStruct{}
	tsOut := reflect.New(reflect.ValueOf(tsIn).Type()).Interface()
	err = Import(ls, lv, tsOut, tsIn)
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	jsonOut, err := json.Marshal(tsOut)
	if err != nil {
		t.Fatalf("Failed to marshal test struct out: %v", err)
	}

	if !bytes.Equal(jsonIn, jsonOut) {
		t.Errorf("Test structs do not match: %s != %s", jsonIn, jsonOut)
	}

	t.Logf("Resulting json: %s", jsonOut)
}
