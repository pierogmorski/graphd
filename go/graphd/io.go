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

// The io portion of the graphd package facilitates IO operations against a graphd database.

package graphd

import (
	"bufio"
	"errors"
	"fmt"
	"strings"
)

// readReponse returns a Response pointer read from an established connection to a graphd database
// and a nil error on success.  On failure, a zero-value Response pointer and error are returned.
// readResponse must be called with g.conn locked.
func (g *graphd) readResponse() (*Response, error) {
	reader := bufio.NewReader(g.conn.netConn)
	str, err := reader.ReadString('\n')
	if err != nil {
		return NewResponse(""), err
	}
	return NewResponse(strings.Trim(str, "\n")), err
}

// Query attempts to send the Request to the graphd database to which this instance of the library
// is connected.  If no connection is currently present, or if the established connection is stale,
// Query will trigger a Redial.  On success, a Response pointer containing a response from the
// graphd database is returned along with a nil error.  On failure, a zero-value Response pointer is
// returned along with an error.  Query locks the connection, allowing only one thread to Query at a
// time.
func (g *graphd) Query(req *Request) (*Response, error) {
	g.conn.Lock()

	g.LogDebugf("attempting to send '%v'", req)

	sent := false
	retries := 2
	for sent == false {
		var err error
		var errStr string

		switch g.conn.existsConnection() {
		// An established connection is present, try to send.
		case true:
			// Queries to graphd are new line termianted.
			_, err = fmt.Fprintf(g.conn.netConn, "%v\n", req)
			if err != nil {
				// Set base error for failed send.
				errStr = fmt.Sprintf("failed to send '%v': %v", req, err)
				retries--
				// If we've exhausted our retries, log and return error.
				if retries == 0 {
					g.LogErr(errStr)
					g.conn.Unlock()
					return NewResponse(""), errors.New(errStr)
				}
				// We can still retry, so try a Redial.  If it fails, append the error message to
				// the base error, log and return the error.
				g.conn.Unlock()
				err = g.Redial(0)
				if err != nil {
					errStr = fmt.Sprintf("%v: %v", errStr, err)
					g.LogErr(errStr)
					return NewResponse(""), errors.New(errStr)
				}
				// OK, we've redialed.  Lock the connection and let's try that send again.
				g.conn.Lock()
				g.LogErrf("%v: retrying (%v retries left)", errStr, retries)
			} else {
				// We've successfully sent.
				g.LogDebugf("successfully sent '%v'", req)
				sent = true
			}

		// No connection present, try to Dial.
		case false:
			g.conn.Unlock()
			err = g.Dial(0)
			if err != nil {
				errStr = fmt.Sprintf("failed to send '%v': %v", req, err)
				g.LogErr(errStr)
				return NewResponse(""), errors.New(errStr)
			}
			g.conn.Lock()
		}
	}

	// We've successfully sent a query, now grab the response and return it.
	res, err := g.readResponse()
	if err != nil {
		errStr := fmt.Sprintf("failed to receive response to '%v': %v", req, err)
		g.LogErr(errStr)
		err = errors.New(errStr)
	} else {
		g.LogDebugf("received response '%v' to query '%v'", res, req)
	}
	g.conn.Unlock()
	return res, err
}
