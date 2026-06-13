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

//go:build legacy
// +build legacy

package retryable

import "errors"

var (
	// ErrAllRequestsFailed when there are no mirrors left to try
	ErrAllRequestsFailed = errors.New("all requests failed")
	// ErrBackoffLimit maximum backoff attempts reached
	ErrBackoffLimit = errors.New("backoff limit reached")
	// ErrCanceled if the context was canceled
	ErrCanceled = errors.New("context was canceled")
	// ErrDigestMismatch if the expected digest wasn't received
	ErrDigestMismatch = errors.New("digest mismatch")
	// ErrNotFound isn't there, search for your value elsewhere
	ErrNotFound = errors.New("not found")
	// ErrNotImplemented returned when method has not been implemented yet
	ErrNotImplemented = errors.New("not implemented")
	// ErrRetryNeeded indicates a request needs to be retried
	ErrRetryNeeded = errors.New("retry needed")
	// ErrUnauthorized request was not authorized
	ErrUnauthorized = errors.New("unauthorized")
)
