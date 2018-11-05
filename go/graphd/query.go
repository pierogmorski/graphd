// Copyright 2018 Google Inc. All rights reserved.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// The query portion of the graphd package contains request and response structures used in IO
// operations.

package graphd

// A Request to be sent to a graphd database.
// Currently this is just a wrapper around a string.
type Request struct {
	body string
}

// String implements stringer interface for a Request.
func (r *Request) String() string {
	return r.body
}

// A Response received from a graphd database.
// Currently this is just a wrapper around a string.
type Response struct {
	body string
}

// String implements stringer interface for a Response.
func (r *Response) String() string {
	return r.body
}

// NewRequest returns a Request pointer initialized from the string parameter.  The parameter should
// represent one request to be sent to a graphd database.  Stringing together multiple requests is
// not currently supported.
func NewRequest(s string) *Request {
	return &Request{s}
}

// NewResponse returns a Response pointer initialized from the string parameter.  The parameter
// should represent one response received from a graphd database.
func NewResponse(s string) *Response {
	return &Response{s}
}
