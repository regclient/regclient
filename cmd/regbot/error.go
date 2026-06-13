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

package main

import "errors"

var (
	// ErrCanceled is used when context is canceled before task completes
	ErrCanceled = errors.New("task was canceled")
	// ErrInvalidInput indicates a required field is invalid
	ErrInvalidInput = errors.New("invalid input")
	// ErrMissingInput indicates a required field is missing
	ErrMissingInput = errors.New("required input missing")
	// ErrNotImplemented returned when method has not been implemented yet
	ErrNotImplemented = errors.New("not implemented")
	// ErrNotFound when anything else isn't found
	ErrNotFound = errors.New("not found")
	// ErrScriptFailed when the script fails to run
	ErrScriptFailed = errors.New("failure in user script")
	// ErrUnsupportedConfigVersion happens when config file version is greater than this command supports
	ErrUnsupportedConfigVersion = errors.New("unsupported config version")
)
