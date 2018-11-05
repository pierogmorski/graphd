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

// The io portion of the graphd package facilitates read/operations against a graphd database.

package graphd

import (
	"bufio"
	"errors"
	"fmt"
	"sync"
)

// Requests and Responses are currently just strings.
type Request struct {
	body string
}
type Response struct {
	body string
}

// Request queue.
type requestQueue struct {
	sync.Mutex
	requests    []Request
	numRequests uint
}

// Response queue.
type responseQueue struct {
	sync.Mutex
	responses    []Response
	numResponses uint
}

// append appends a Request r to the requests queue.
// Must hold request queue lock to invoke.
func (r *requestQueue) append(req Request) {
	r.requests = append(r.requests, req)
	r.numRequests++
}

// pop removes and returns the first Request from the requests queue, along with a nil error
// code.  If no requests are present, a zero-value Request and a non-nil error are returned.
// Must hold request queue lock to invoke.
func (r *requestQueue) pop() (Request, error) {
	var req Request
	if r.numRequests == 0 {
		return req, errors.New("no requests in requests queue")
	}

	req = r.requests[0]
	r.requests = r.requests[1:]
	r.numRequests--

	return req, nil
}

// append appends a Response r to the responses queue.
// Must hold response queue lock to invoke.
func (r *responseQueue) append(res Response) {
	r.responses = append(r.responses, res)
	r.numResponses++
}

// pop removes and returns the first Response from the responses queue, along with a nil
// error code.  If no responses are present, a zero-value Response and a non-nil error are
// returned.
// Must hold response queue to invoke.
func (r *responseQueue) pop() (Response, error) {
	var res Response
	if r.numResponses == 0 {
		return res, errors.New("no responses in response queue")
	}

	res = r.responses[0]
	r.responses = r.responses[1:]
	r.numResponses--

	return res, nil
}

// scans a connection for incoming responses, and pushes them onto the response queue.  A new scanner
// is invoked as a go routine on each Dial/Redial.  A scanner terminates when it tries to read from
// a nil g.conn.netConn, or if an obtained response is empty.  It's possible that on a Redial, we
// will have two active scanners (one scanning the old g.conn.netConn, and one scanning the new).
// The old scanner will exit when it reads the first nil response from the old g.conn.netConn.
func (g *graphd) scan() {
	g.LogDebugf("starting new scanner")

	// Check if we have a connection, or if someone disconnected.
	g.conn.Lock()
	if !g.conn.existsConnection() {
		g.LogDebugf("no connection present, terminating this scanner")
		g.conn.Unlock()
		return
	}
	scanner := bufio.NewScanner(g.conn.netConn)
	g.conn.Unlock()
	for {
		scanner.Scan()

		g.resQ.Lock()

		res := Response{scanner.Text()}
		if len(res.body) == 0 {
			g.LogDebugf("read empty response, terminating this scanner")
			g.resQ.Unlock()
			return
		}

		g.LogDebugf("scanned '%v'", res)

		g.resQ.append(res)

		g.resQ.Unlock()
	}
}

// write writes the contents of the requests queue when woken up.  On write failures, or if no connection
// is present, write will trigger a Redial.
func (g *graphd) write() {
	g.LogDebugf("starting new writer")

	for {
		// Block until there's something to write.
		<-g.conn.startWrite

		g.conn.Lock()
		g.reqQ.Lock()

		g.LogDebugf("attempting to write contents of request queue: %v", g.reqQ.requests)

		// TODO: max resend attempts?
		for req, err := g.reqQ.pop(); err == nil; req, err = g.reqQ.pop() {
			if !g.conn.existsConnection() {
				g.LogErr("failed to write: no connection present, redialing")
				g.reqQ.append(req)

				g.conn.Unlock()
				g.Redial(0)
				g.conn.Lock()
				continue
			}
			g.LogDebugf("attempting to write '%v'", req)
			n, err := fmt.Fprintf(g.conn.netConn, "%s\n", req.body)
			if err != nil {
				g.LogErrf("failed to write '%v': %v, re-pushing to request queue and redialing", req, err)
				g.reqQ.append(req)

				g.conn.Unlock()
				g.Redial(0)
				g.conn.Lock()
				continue
			}
			g.LogDebugf("wrote '%v' (%v bytes)", req, n)
		}

		g.reqQ.Unlock()
		g.conn.Unlock()
	}
}

// Flush triggers a write of any pending requests in the requests queue.  No redials.
// TODO: maybe a few redials isn't a bad idea?
func (g *graphd) Flush() {
	g.LogDebugf("flushing pending requests")

	g.conn.Lock()
	defer g.conn.Unlock()
	g.reqQ.Lock()
	defer g.reqQ.Unlock()

	if !g.conn.existsConnection() {
		g.LogErr("failed to flush: no connection present")
		return
	}

	for req, err := g.reqQ.pop(); err == nil; req, err = g.reqQ.pop() {
		g.LogDebugf("attempting to flush '%v'", req)
		n, err := fmt.Fprintf(g.conn.netConn, "%s\n", req.body)
		if err != nil {
			g.LogErrf("failed to flush '%v': %v, re-pushing to request queue", req, err)
			g.reqQ.append(req)
			continue
		}
		g.LogDebugf("flushed '%v' (%v bytes)", req, n)
	}

	//TODO now what, we should scan the results, but we can't loop over a bufio scanner as we'll loop forever.
}

// Write creates a Request from the string s and puts it on the requests queue.  It then wakes up the
// write routine which performs the actual write.
func (g *graphd) Write(s string) {
	req := Request{s}

	g.reqQ.Lock()
	g.LogDebugf("adding '%v' to request queue", req)
	g.reqQ.append(req)
	g.reqQ.Unlock()

	g.conn.startWrite <- true
}

// Read removes and returns Responses from the responses queue along with a nil error.  If no
// Responses are present, a zero-value Response a non-nil error are returned.
func (g *graphd) Read() (Response, error) {
	g.LogDebugf("attempting to read a response")

	g.resQ.Lock()
	defer g.resQ.Unlock()

	res, err := g.resQ.pop()
	if err != nil {
		errStr := fmt.Sprintf("failed to read a response: %v", err)
		g.LogErr(errStr)
		return res, errors.New(errStr)
	}

	g.LogDebugf("read '%v'", res)

	return res, nil
}
