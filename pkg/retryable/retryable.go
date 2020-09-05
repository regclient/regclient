package retryable

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"time"

	digest "github.com/opencontainers/go-digest"
	"github.com/sirupsen/logrus"
)

// Retryable is used to create requests with built in retry capabilities
type Retryable interface {
	DoRequest(ctx context.Context, method string, u url.URL, opts ...OptsReq) (Response, error)
}

// Response is used to handle the result of a request
type Response interface {
	io.ReadCloser
	HTTPResponse() *http.Response
	HTTPResponses() ([]*http.Response, error)
}

// Auth is used to process Www-Authenticate header and update request with Authorization header
type Auth interface {
	HandleResponse(*http.Response) error
	UpdateRequest(*http.Request) error
}

// Opts injects options into NewRetryable
type Opts func(*retryable)

// OptsReq injects options into NewRequest
type OptsReq func(*request)

type retryable struct {
	httpClient *http.Client
	mirrorFunc func(url.URL) ([]url.URL, error)
	auth       Auth
	limit      int
	delayInit  time.Duration
	delayMax   time.Duration
	log        *logrus.Logger
}

// NewRetryable returns a retryable interface
func NewRetryable(opts ...Opts) Retryable {
	r := &retryable{
		httpClient: &http.Client{},
		limit:      5,
	}
	r.delayInit, _ = time.ParseDuration("1s")
	r.delayMax, _ = time.ParseDuration("30s")
	r.log = &logrus.Logger{
		Out:       os.Stderr,
		Formatter: new(logrus.TextFormatter),
		Hooks:     make(logrus.LevelHooks),
		Level:     logrus.WarnLevel,
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

// WithAuth adds authentication to retryable methods
func WithAuth(auth Auth) Opts {
	return func(r *retryable) {
		r.auth = auth
	}
}

// WithDelay initial time to wait between retries (increased with exponential backoff)
func WithDelay(delayInit time.Duration, delayMax time.Duration) Opts {
	return func(r *retryable) {
		if delayInit > 0 {
			r.delayInit = delayInit
		} else {
			r.delayInit = 0
		}
		// delayMax must be at least delayInit, if 0 initialize to 30x delayInit
		if delayMax > r.delayInit {
			r.delayMax = delayMax
		} else if delayMax > 0 {
			r.delayMax = r.delayInit
		} else {
			r.delayMax = r.delayInit * 30
		}
	}
}

// WithHTTPClient uses a specific http client with retryable requests
func WithHTTPClient(h *http.Client) Opts {
	return func(r *retryable) {
		r.httpClient = h
	}
}

// WithLimit restricts the number of retries (defaults to 5)
func WithLimit(l int) Opts {
	return func(r *retryable) {
		r.limit = l
	}
}

// WithLog injects a logrus Logger configuration
func WithLog(log *logrus.Logger) Opts {
	return func(r *retryable) {
		r.log = log
	}
}

// WithMirrors adds ability to contact mirrors in retryable methods
func WithMirrors(mirrorFunc func(url.URL) ([]url.URL, error)) Opts {
	return func(r *retryable) {
		r.mirrorFunc = mirrorFunc
	}
}

// WithTransport uses a specific http transport with retryable requests
func WithTransport(t *http.Transport) Opts {
	return func(r *retryable) {
		r.httpClient = &http.Client{Transport: t}
	}
}

type request struct {
	r          *retryable
	method     string
	urls       []url.URL
	curURL     int
	header     http.Header
	getBody    func() (io.ReadCloser, error)
	contentLen int64
	chunking   bool
	offset     int64
	curRead    int64
	backoffs   int
	done       bool
	digest     digest.Digest
	digester   digest.Digester
	progressCB func(int64, error)
	responses  []*http.Response
	reader     io.Reader
	log        *logrus.Logger
}

func (r *retryable) DoRequest(ctx context.Context, method string, u url.URL, opts ...OptsReq) (Response, error) {
	req := &request{
		r:          r,
		method:     method,
		urls:       []url.URL{u},
		curURL:     0,
		header:     http.Header{},
		getBody:    nil,
		contentLen: -1,
		chunking:   false,
		offset:     0,
		curRead:    0,
		backoffs:   0,
		done:       false,
		digest:     "",
		digester:   nil,
		progressCB: nil,
		responses:  []*http.Response{},
		reader:     nil,
		log:        r.log,
	}
	// replace url list with mirrors
	if r.mirrorFunc != nil {
		if urls, err := r.mirrorFunc(u); err == nil {
			req.urls = urls
		}
	}
	// apply opts
	for _, opt := range opts {
		opt(req)
	}

	// run the request until successful or non-recoverable error
	err := req.retryLoop()
	return req, err
}

// WithBodyBytes converts a bytes slice into a body func and content length
func WithBodyBytes(body []byte) OptsReq {
	return func(req *request) {
		req.contentLen = int64(len(body))
		req.getBody = func() (io.ReadCloser, error) {
			return ioutil.NopCloser(bytes.NewReader(body)), nil
		}
	}
}

// WithBodyFunc includes body content in a request
func WithBodyFunc(getbody func() (io.ReadCloser, error)) OptsReq {
	return func(req *request) {
		req.getBody = getbody
	}
}

// WithChunking allows content to be divided into multiple smaller chunks
func WithChunking() OptsReq {
	return func(req *request) {
		req.chunking = true
	}
}

// WithContentLen sets the content length
func WithContentLen(l int64) OptsReq {
	return func(req *request) {
		req.contentLen = l
	}
}

// WithDigest verifies the returned content digest matches.
// Note that the digest is only calculated upon EOF from the downloaded
// content, so the reader may receive an error rather than EOF from a
// digest mismatch. The content itself must still be read.
func WithDigest(d digest.Digest) OptsReq {
	return func(req *request) {
		req.digest = d
		req.digester = digest.Canonical.Digester()
	}
}

// WithHeader sets a header
func WithHeader(key string, values []string) OptsReq {
	return func(req *request) {
		for _, v := range values {
			req.header.Add(key, v)
		}
	}
}

// WithHeaders includes a header object
func WithHeaders(headers http.Header) OptsReq {
	return func(req *request) {
		for key := range headers {
			for _, val := range headers.Values(key) {
				req.header.Add(key, val)
			}
		}
	}
}

// WithProgressCB calls the CB function as data is received
func WithProgressCB(cb func(int64, error)) OptsReq {
	return func(req *request) {
		req.progressCB = cb
	}
}

func (req *request) httpDo() error {
	// build the http reqest for the current mirror url
	httpReq, err := http.NewRequest(req.method, req.urls[req.curURL].String(), nil)
	if err != nil {
		return err
	}
	if req.getBody != nil {
		httpReq.Body, err = req.getBody()
		if err != nil {
			return err
		}
		httpReq.GetBody = req.getBody
		httpReq.ContentLength = req.contentLen
	}
	if len(req.header) > 0 {
		httpReq.Header = req.header
	}
	if req.offset > 0 {
		// TODO: implement range requests
		return ErrNotImplemented
	}

	// include auth header
	if req.r.auth != nil {
		err = req.r.auth.UpdateRequest(httpReq)
		if err != nil {
			return err
		}
	}

	req.log.WithFields(logrus.Fields{
		"method":   req.method,
		"url":      req.urls[req.curURL].String(),
		"withAuth": (len(httpReq.Header.Values("Authorization")) > 0),
	}).Debug("Sending request")
	resp, err := req.r.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	req.responses = append(req.responses, resp)

	// update reader
	if req.digester == nil {
		req.reader = resp.Body
	} else {
		req.reader = io.TeeReader(resp.Body, req.digester.Hash())
	}

	return nil
}

func (req *request) retryLoop() error {
	for {
		err := req.httpDo()
		if err != nil {
			if boerr := req.backoff(true); boerr == nil {
				continue
			}
		}
		if err != nil {
			return err
		}
		err = req.checkResp()
		if err == nil {
			break
		} else if err != ErrRetryNeeded {
			return err
		}
	}
	return nil
}

func (req *request) checkResp() error {
	if len(req.responses) == 0 {
		return ErrNotFound
	}
	curURL := req.urls[req.curURL]
	lastResp := req.responses[len(req.responses)-1]
	if lastResp.StatusCode == http.StatusUnauthorized {
		if req.r.auth != nil {
			if err := req.r.auth.HandleResponse(lastResp); err != nil {
				req.log.WithFields(logrus.Fields{
					"URL": curURL.String(),
					"Err": err,
				}).Warn("Failed to handle auth request")
				if boerr := req.backoff(true); boerr == nil {
					return nil
				}
				return err
			}
			req.log.WithFields(logrus.Fields{
				"URL": curURL.String(),
			}).Debug("Retry needed with auth header")
			return ErrRetryNeeded
		}
		return ErrUnauthorized
	} else if lastResp.StatusCode == http.StatusRequestTimeout || lastResp.StatusCode == http.StatusTooManyRequests {
		req.log.WithFields(logrus.Fields{
			"URL":    curURL.String(),
			"Status": lastResp.Status,
		}).Debug("Backoff and retry needed")
		// backoff, next mirror
		if boerr := req.backoff(false); boerr == nil {
			return ErrRetryNeeded
		}
		return fmt.Errorf("Unexpected http status code %d", lastResp.StatusCode)
	} else if lastResp.StatusCode >= 200 && lastResp.StatusCode < 300 {
		return nil
	}

	// any other return codes are unexpected, remove mirror from list
	req.log.WithFields(logrus.Fields{
		"URL":    curURL.String(),
		"Status": lastResp.Status,
	}).Debug("Backoff and retry needed, removing failing mirror")
	if boerr := req.backoff(true); boerr == nil {
		return ErrRetryNeeded
	}
	return fmt.Errorf("Unexpected http status code %d", lastResp.StatusCode)
}

func (req *request) backoff(removeMirror bool) error {
	req.backoffs++
	if req.backoffs >= req.r.limit {
		return ErrBackoffLimit
	}
	failedURL := req.urls[req.curURL]
	// next mirror based on whether remove flag is set
	if removeMirror {
		req.urls = append(req.urls[:req.curURL], req.urls[req.curURL+1:]...)
	} else {
		req.curURL = (req.curURL + 1) % len(req.urls)
	}
	if len(req.urls) == 0 {
		return ErrAllMirrorsFailed
	}
	// sleep for backoff time
	sleepTime := req.r.delayInit << req.backoffs
	if sleepTime > req.r.delayMax {
		sleepTime = req.r.delayMax
	}
	req.log.WithFields(logrus.Fields{
		"Host":    failedURL.Host,
		"Removed": removeMirror,
		"Seconds": sleepTime.Seconds(),
	}).Warn("Sleeping for backoff")
	time.Sleep(sleepTime)
	return nil
}

func (req *request) Read(b []byte) (int, error) {
	// if done, return eof
	if req.done {
		return 0, io.EOF
	}
	// if no responses, error
	if len(req.responses) == 0 {
		return 0, ErrNotFound
	}
	// fetch block
	lastResp := req.responses[len(req.responses)-1]
	i, err := req.reader.Read(b)
	req.curRead += int64(i)
	if err == io.EOF && lastResp.ContentLength > 0 {
		// handle early EOF or other failed connection with a retry from an offset
		if req.curRead < lastResp.ContentLength {
			req.offset += req.curRead
			req.log.WithFields(logrus.Fields{
				"url":    req.urls[req.curURL].String(),
				"offset": req.offset,
			}).Warn("EOF before reading all content, retrying")
			err = req.retryLoop()
			if err != nil {
				return i, err
			}
		} else if req.curRead >= lastResp.ContentLength {
			req.done = true
		}
	}
	// if eof, verify digest, set error on mismatch
	if req.digester != nil && err == io.EOF && req.digest != req.digester.Digest() {
		req.log.WithFields(logrus.Fields{
			"expected": req.digest,
			"computed": req.digester.Digest(),
		}).Warn("Digest mismatch")
		req.done = true
		return i, ErrDigestMismatch
	}

	// pass through read on the last response
	return i, err
}

func (req *request) Close() error {
	// if no responses, error
	if req.reader == nil {
		return ErrNotFound
	}
	// pass through close to last request, mark as done
	lastResp := req.responses[len(req.responses)-1]
	req.done = true
	return lastResp.Body.Close()
}

func (req *request) HTTPResponse() *http.Response {
	if len(req.responses) > 0 {
		return req.responses[len(req.responses)-1]
	}
	return nil
}

func (req *request) HTTPResponses() ([]*http.Response, error) {
	if len(req.responses) > 0 {
		return req.responses, nil
	}
	return nil, ErrNotFound
}
