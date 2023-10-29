//go:build !nolegacy
// +build !nolegacy

// Package retryable is a legacy package, functionality has been moved to reghttp
package retryable

import (
	"bytes"
	"context"
	"net"
	"os"
	"sync"

	// crypto libraries included for go-digest
	_ "crypto/sha256"
	_ "crypto/sha512"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	digest "github.com/opencontainers/go-digest"
	"github.com/sirupsen/logrus"
)

// Retryable is used to create requests with built in retry capabilities
type Retryable interface {
	DoRequest(ctx context.Context, method string, u []url.URL, opts ...OptsReq) (Response, error)
	BackoffClear()
	BackoffUntil() time.Time
}

// Response is used to handle the result of a request
type Response interface {
	io.ReadCloser
	HTTPResponse() *http.Response
	HTTPResponses() ([]*http.Response, error)
}

// Auth is used to process Www-Authenticate header and update request with Authorization header
type Auth interface {
	AddScope(host, scope string) error
	HandleResponse(*http.Response) error
	UpdateRequest(*http.Request) error
}

// Opts injects options into NewRetryable
type Opts func(*retryable)

// OptsReq injects options into NewRequest
type OptsReq func(*request)

type retryable struct {
	httpClient    *http.Client
	auth          Auth
	rootCAPool    [][]byte
	limit         int
	delayInit     time.Duration
	delayMax      time.Duration
	backoffNeeded bool
	backoffCur    int
	backoffUntil  time.Time
	log           *logrus.Logger
	useragent     string
	mu            sync.Mutex
}

var defaultDelayInit, _ = time.ParseDuration("1s")
var defaultDelayMax, _ = time.ParseDuration("30s")
var defaultLimit = 3

// NewRetryable returns a retryable interface
func NewRetryable(opts ...Opts) Retryable {
	r := &retryable{
		httpClient: &http.Client{
			Transport: &http.Transport{
				Dial: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).Dial,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
		},
		limit:      defaultLimit,
		delayInit:  defaultDelayInit,
		delayMax:   defaultDelayMax,
		log:        &logrus.Logger{Out: io.Discard},
		rootCAPool: [][]byte{},
	}

	for _, opt := range opts {
		opt(r)
	}

	// inject certificates from user
	if len(r.rootCAPool) > 0 {
		var tlsc *tls.Config
		if r.httpClient.Transport == nil {
			r.httpClient.Transport = &http.Transport{}
		}
		t, ok := r.httpClient.Transport.(*http.Transport)
		if ok {
			if t.TLSClientConfig != nil {
				tlsc = t.TLSClientConfig.Clone()
			} else {
				//#nosec G402 the default TLS 1.2 minimum version is allowed to support older registries
				tlsc = &tls.Config{}
			}
			if tlsc.RootCAs == nil {
				rootPool, err := x509.SystemCertPool()
				if err != nil {
					r.log.WithFields(logrus.Fields{
						"err": err,
					}).Warn("Failed to load system cert pool")
				}
				tlsc.RootCAs = rootPool
			}
			for _, ca := range r.rootCAPool {
				if ok := tlsc.RootCAs.AppendCertsFromPEM(ca); !ok {
					r.log.WithFields(logrus.Fields{
						"cert": string(ca),
					}).Warn("Failed to load root certificate")
				}
			}
			t.TLSClientConfig = tlsc
			r.httpClient.Transport = t
		}
	}
	return r
}

// WithAuth adds authentication to retryable methods
func WithAuth(auth Auth) Opts {
	return func(r *retryable) {
		r.auth = auth
	}
}

// WithCerts adds certificates
func WithCerts(certs [][]byte) Opts {
	return func(r *retryable) {
		r.rootCAPool = append(r.rootCAPool, certs...)
	}
}

// WithCertFiles adds certificates by filename
func WithCertFiles(files []string) Opts {
	return func(r *retryable) {
		for _, f := range files {
			//#nosec G304 command is run by a user accessing their own files
			c, err := os.ReadFile(f)
			if err != nil {
				r.log.WithFields(logrus.Fields{
					"err":  err,
					"file": f,
				}).Warn("Failed to read certificate")
			} else {
				r.rootCAPool = append(r.rootCAPool, c)
			}
		}
	}
}

// WithDelay initial time to wait between retries (increased with exponential backoff)
func WithDelay(delayInit time.Duration, delayMax time.Duration) Opts {
	return func(r *retryable) {
		if delayInit > 0 {
			r.delayInit = delayInit
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
		if l > 0 {
			r.limit = l
		}
	}
}

// WithLog injects a logrus Logger configuration
func WithLog(log *logrus.Logger) Opts {
	return func(r *retryable) {
		r.log = log
	}
}

// WithTransport uses a specific http transport with retryable requests
func WithTransport(t *http.Transport) Opts {
	return func(r *retryable) {
		r.httpClient = &http.Client{Transport: t}
	}
}

// WithUserAgent sets a user agent header
func WithUserAgent(ua string) Opts {
	return func(r *retryable) {
		r.useragent = ua
	}
}

func (r *retryable) BackoffClear() {
	if r.backoffCur > r.limit {
		r.backoffCur = r.limit
	}
	if r.backoffCur > 0 {
		r.backoffCur--
		if r.backoffCur == 0 {
			r.backoffUntil = time.Time{}
		}
	}
	r.backoffNeeded = false
}

func (r *retryable) backoffSet(lastResp *http.Response) error {
	r.backoffCur++
	// sleep for backoff time
	sleepTime := r.delayInit << r.backoffCur
	// limit to max delay
	if sleepTime > r.delayMax {
		sleepTime = r.delayMax
	}
	// check rate limit header
	if lastResp != nil && lastResp.Header.Get("Retry-After") != "" {
		ras := lastResp.Header.Get("Retry-After")
		ra, _ := time.ParseDuration(ras + "s")
		if ra > r.delayMax {
			sleepTime = r.delayMax
		} else if ra > sleepTime {
			sleepTime = ra
		}
	}

	r.backoffUntil = time.Now().Add(sleepTime)
	r.backoffNeeded = true

	if r.backoffCur == r.limit {
		return fmt.Errorf("%w: backoffs %d", ErrBackoffLimit, r.backoffCur)
	}

	return nil
}

// BackoffUntil returns the time until the next backoff would complete
func (r *retryable) BackoffUntil() time.Time {
	return r.backoffUntil
}

type request struct {
	r          *retryable
	context    context.Context
	method     string
	urls       []url.URL
	curURL     int
	header     http.Header
	getBody    func() (io.ReadCloser, error)
	contentLen int64
	chunking   bool
	offset     int64
	curRead    int64
	done       bool
	digest     digest.Digest
	digester   digest.Digester
	progressCB func(int64, error)
	responses  []*http.Response
	reader     io.Reader
	log        *logrus.Logger
}

func (r *retryable) DoRequest(ctx context.Context, method string, u []url.URL, opts ...OptsReq) (Response, error) {
	req := &request{
		r:          r,
		context:    ctx,
		method:     method,
		urls:       u,
		curURL:     0,
		header:     http.Header{},
		getBody:    nil,
		contentLen: -1,
		chunking:   false,
		offset:     0,
		curRead:    0,
		done:       false,
		digest:     "",
		digester:   nil,
		progressCB: nil,
		responses:  []*http.Response{},
		reader:     nil,
		log:        r.log,
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
			return io.NopCloser(bytes.NewReader(body)), nil
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

func WithScope(repo string, push bool) OptsReq {
	scope := "repository:" + repo + ":pull"
	if push {
		scope = scope + ",push"
	}
	return func(req *request) {
		for _, url := range req.urls {
			_ = req.r.auth.AddScope(url.Host, scope)
		}
	}
}

func (req *request) retryLoop() error {
	req.r.mu.Lock()
	defer req.r.mu.Unlock()
	curRetry := 0

	var httpErr error
	for {
		// handle backoffs and errors
		if len(req.urls) == 0 {
			if httpErr != nil {
				return httpErr
			}
			return ErrAllRequestsFailed
		}
		curRetry++
		if curRetry > req.r.limit {
			return ErrAllRequestsFailed
		}

		if !req.r.backoffUntil.IsZero() && req.r.backoffUntil.After(time.Now()) {
			sleepTime := time.Until(req.r.backoffUntil)
			req.log.WithFields(logrus.Fields{
				"Host":    req.urls[req.curURL].Host,
				"Seconds": sleepTime.Seconds(),
			}).Warn("Sleeping for backoff")
			select {
			case <-req.context.Done():
				return ErrCanceled
			case <-time.After(sleepTime):
			}
		}

		// close any previous responses before making a new request
		if len(req.responses) > 0 {
			errC := req.responses[len(req.responses)-1].Body.Close()
			if errC != nil {
				return fmt.Errorf("failed to close connection: %w", errC)
			}
		}
		// send the new request
		httpErr = req.httpDo()
		if httpErr != nil {
			_ = req.r.backoffSet(nil)
			req.nextURL(true)
			continue
		}

		// check the response
		lastURL := req.urls[req.curURL]
		lastResp := req.responses[len(req.responses)-1]
		statusCode := lastResp.StatusCode
		removeURL := false
		runBackoff := false

		switch {
		case 200 <= statusCode && statusCode < 300:
			// all 200 status codes are successful
			req.r.BackoffClear()
			return nil
		case statusCode == http.StatusUnauthorized:
			err := req.handleAuth()
			if err != nil {
				req.log.WithFields(logrus.Fields{
					"URL": lastURL.String(),
					"Err": err,
				}).Warn("Failed to handle auth request")
				runBackoff = true
				removeURL = true
			}
		case statusCode == http.StatusForbidden:
			req.log.WithFields(logrus.Fields{
				"URL":    lastURL.String(),
				"Status": lastResp.Status,
			}).Debug("Forbidden")
			runBackoff = true
			removeURL = true
		case statusCode == http.StatusNotFound:
			req.log.WithFields(logrus.Fields{
				"URL":    lastURL.String(),
				"Status": lastResp.Status,
			}).Debug("Not found")
			removeURL = true
		case statusCode == http.StatusTooManyRequests:
			req.log.WithFields(logrus.Fields{
				"URL":    lastURL.String(),
				"Status": lastResp.Status,
			}).Debug("Rate limit exceeded")
			runBackoff = true
		case statusCode == http.StatusRequestTimeout:
			req.log.WithFields(logrus.Fields{
				"URL":    lastURL.String(),
				"Status": lastResp.Status,
			}).Debug("Timeout")
			runBackoff = true
		case statusCode == http.StatusGatewayTimeout:
			req.log.WithFields(logrus.Fields{
				"URL":    lastURL.String(),
				"Status": lastResp.Status,
			}).Debug("Gateway timeout")
			runBackoff = true
		default:
			body, _ := io.ReadAll(lastResp.Body)
			req.log.WithFields(logrus.Fields{
				"URL":    lastURL.String(),
				"Status": lastResp.Status,
				"Body":   string(body),
			}).Debug("Unexpected status")
			runBackoff = true
			removeURL = true
		}

		// remove url and trigger backoff if needed
		if removeURL {
			req.nextURL(removeURL)
		}
		if runBackoff {
			_ = req.r.backoffSet(lastResp) // ignore error indicating backoff limit reached
		}
	}
}

func (req *request) handleAuth() error {
	curURL := req.urls[req.curURL]
	lastResp := req.responses[len(req.responses)-1]
	// for unauthorized requests, try to setup auth and retry without backoff
	if req.r.auth == nil {
		return ErrUnauthorized
	}
	err := req.r.auth.HandleResponse(lastResp)
	if err != nil {
		req.log.WithFields(logrus.Fields{
			"URL": curURL.String(),
			"Err": err,
		}).Warn("Failed to handle auth request")
		return err
	}
	return nil
}

func (req *request) httpDo() error {
	// build the http reqest for the current mirror url
	httpReq, err := http.NewRequestWithContext(req.context, req.method, req.urls[req.curURL].String(), nil)
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
	if req.r.useragent != "" && httpReq.Header.Get("User-Agent") == "" {
		httpReq.Header.Add("User-Agent", req.r.useragent)
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

func (req *request) nextURL(removeLast bool) {
	// next mirror based on whether remove flag is set
	if removeLast {
		req.urls = append(req.urls[:req.curURL], req.urls[req.curURL+1:]...)
		if req.curURL >= len(req.urls) {
			req.curURL = 0
		}
	} else {
		if len(req.urls) > 0 {
			req.curURL = (req.curURL + 1) % len(req.urls)
		} else {
			req.curURL = 0
		}
	}
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
		if lastResp.Request.Method == "HEAD" {
			// no body on a head request
			req.done = true
		} else if req.curRead < lastResp.ContentLength {
			// TODO: handle early EOF or other failed connection with a retry
			// req.offset += req.curRead
			// err = req.retryLoop()
			// if err != nil {
			// 	return i, err
			// }
			req.log.WithFields(logrus.Fields{
				"curRead":    req.curRead,
				"contentLen": lastResp.ContentLength,
			}).Debug("EOF before reading all content, retrying")
			return i, err
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
	if req.reader == nil || len(req.responses) == 0 {
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
