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

package reqmeta

import "testing"

func TestDataNext(t *testing.T) {
	t.Parallel()
	tt := []struct {
		name           string
		queued, active []*Data
		expect         int
	}{
		{
			name: "empty queued",
			active: []*Data{
				{Kind: Blob, Size: 1000},
			},
			expect: -1,
		},
		{
			name: "no active",
			queued: []*Data{
				{Kind: Blob, Size: 1000},
				{Kind: Blob, Size: 1000},
			},
			expect: 0,
		},
		{
			name: "one active need small",
			active: []*Data{
				{Kind: Blob, Size: 10000},
			},
			queued: []*Data{
				{Kind: Blob, Size: 1000},
				{Kind: Manifest, Size: 1000},
				{Kind: Blob, Size: 1000000},
			},
			expect: 1,
		},
		{
			name: "one active find first head",
			active: []*Data{
				{Kind: Blob, Size: 10000},
			},
			queued: []*Data{
				{Kind: Blob, Size: 1000},
				{Kind: Query, Size: 0},
				{Kind: Manifest, Size: 1000},
				{Kind: Unknown, Size: 0},
				{Kind: Head, Size: 0},
				{Kind: Head, Size: 0},
				{Kind: Blob, Size: 1000000},
			},
			expect: 4,
		},
		{
			name: "one active find smallest manifest",
			active: []*Data{
				{Kind: Blob, Size: 10000},
			},
			queued: []*Data{
				{Kind: Blob, Size: 1000},
				{Kind: Query, Size: 0},
				{Kind: Manifest, Size: 2000},
				{Kind: Manifest, Size: 1000},
				{Kind: Unknown, Size: 0},
				{Kind: Blob, Size: 1000000},
			},
			expect: 3,
		},
		{
			name: "one active ignore unknown size",
			active: []*Data{
				{Kind: Blob, Size: 10000},
			},
			queued: []*Data{
				{Kind: Blob, Size: 1000},
				{Kind: Query, Size: 0},
				{Kind: Manifest, Size: 2000},
				{Kind: Manifest, Size: 1000},
				{Kind: Manifest, Size: 0},
				{Kind: Unknown, Size: 0},
				{Kind: Blob, Size: 1000000},
			},
			expect: 3,
		},
		{
			name: "one active need old",
			active: []*Data{
				{Kind: Manifest, Size: 1000},
			},
			queued: []*Data{
				{Kind: Blob, Size: 1000},
				{Kind: Manifest, Size: 1000},
				{Kind: Blob, Size: 1000000},
			},
			expect: 0,
		},
		{
			name: "two active need large",
			active: []*Data{
				{Kind: Manifest, Size: 1000},
				{Kind: Blob, Size: 100000},
			},
			queued: []*Data{
				{Kind: Blob, Size: 1000},
				{Kind: Manifest, Size: 1000},
				{Kind: Blob, Size: 1000000},
			},
			expect: 2,
		},
		{
			name: "three active pick large blob",
			active: []*Data{
				{Kind: Blob, Size: 1000000},
				{Kind: Blob, Size: 10000},
				{Kind: Manifest, Size: 10000},
			},
			queued: []*Data{
				{Kind: Blob, Size: 1000},
				{Kind: Manifest, Size: 1000},
				{Kind: Blob, Size: 200000000},
			},
			expect: 2,
		},
		{
			name: "three active need small",
			active: []*Data{
				{Kind: Blob, Size: 1000000},
				{Kind: Blob, Size: 10000},
				{Kind: Blob, Size: 20000},
			},
			queued: []*Data{
				{Kind: Blob, Size: 1000},
				{Kind: Manifest, Size: 1000},
				{Kind: Blob, Size: 200000000},
			},
			expect: 1,
		},
		{
			name: "three active pick largest",
			active: []*Data{
				{Kind: Blob, Size: 20000},
				{Kind: Blob, Size: 10000},
				{Kind: Manifest, Size: 10000},
			},
			queued: []*Data{
				{Kind: Blob, Size: 1000},
				{Kind: Manifest, Size: 1000},
				{Kind: Blob, Size: 1000000},
				{Kind: Blob, Size: 2000000},
				{Kind: Blob, Size: 1500000},
			},
			expect: 3,
		},
		{
			name: "three active no large",
			active: []*Data{
				{Kind: Blob, Size: 100000},
				{Kind: Blob, Size: 2000000},
				{Kind: Manifest, Size: 10000},
			},
			queued: []*Data{
				{Kind: Blob, Size: 1000},
				{Kind: Manifest, Size: 1000},
				{Kind: Blob, Size: 20000},
				{Kind: Blob, Size: 30000},
			},
			expect: 0,
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			result := DataNext(tc.queued, tc.active)
			if result != tc.expect {
				t.Errorf("unexpected result, expected %d, received %d", tc.expect, result)
			}
		})
	}
}
