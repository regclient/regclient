package auth

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

type charLU byte

var charLUs [256]charLU

var defaultClientID = "regclient"

// minTokenLife tokens are required to last at least 60 seconds to support older docker clients
var minTokenLife = 60

const (
	isSpace charLU = 1 << iota
	isAlphaNum
)

func init() {
	for c := 0; c < 256; c++ {
		charLUs[c] = 0
		if strings.ContainsRune(" \t\r\n", rune(c)) {
			charLUs[c] |= isSpace
		}
		if (rune('a') <= rune(c) && rune(c) <= rune('z')) || (rune('A') <= rune(c) && rune(c) <= rune('Z') || (rune('0') <= rune(c) && rune(c) <= rune('9'))) {
			charLUs[c] |= isAlphaNum
		}
	}
}

// CredsFn is passed to lookup credentials for a given hostname, response is a username and password or empty strings
type CredsFn func(string) Cred

// Cred is returned by the CredsFn
type Cred struct {
	User, Password, Token string
}

// Auth manages authorization requests/responses for http requests
type Auth interface {
	HandleResponse(*http.Response) error
	UpdateRequest(*http.Request) error
}

// Challenge is the extracted contents of the WWW-Authenticate header
type Challenge struct {
	authType string
	params   map[string]string
}

// Handler handles a challenge for a host to return an auth header
type Handler interface {
	ProcessChallenge(Challenge) error
	GenerateAuth() (string, error)
}

// HandlerBuild is used to make a new handler for a specific authType and URL
type HandlerBuild func(client *http.Client, clientID, host string, cred Cred) Handler

// Opts configures options for NewAuth
type Opts func(*auth)

type auth struct {
	httpClient *http.Client
	clientID   string
	credsFn    CredsFn
	hbs        map[string]HandlerBuild       // handler builders based on authType
	hs         map[string]map[string]Handler // handlers based on url and authType
	authTypes  []string
	log        *logrus.Logger
	mu         sync.Mutex
}

// NewAuth creates a new Auth
func NewAuth(opts ...Opts) Auth {
	a := &auth{
		httpClient: &http.Client{},
		clientID:   defaultClientID,
		credsFn:    DefaultCredsFn,
		hbs:        map[string]HandlerBuild{},
		hs:         map[string]map[string]Handler{},
		authTypes:  []string{},
	}
	a.log = &logrus.Logger{
		Out:       os.Stderr,
		Formatter: new(logrus.TextFormatter),
		Hooks:     make(logrus.LevelHooks),
		Level:     logrus.WarnLevel,
	}

	for _, opt := range opts {
		opt(a)
	}

	if len(a.authTypes) == 0 {
		a.addDefaultHandlers()
	}

	return a
}

// WithCreds provides a user/pass lookup for a url
func WithCreds(f CredsFn) Opts {
	return func(a *auth) {
		if f != nil {
			a.credsFn = f
		}
	}
}

// WithHTTPClient uses a specific http client with requests
func WithHTTPClient(h *http.Client) Opts {
	return func(a *auth) {
		if h != nil {
			a.httpClient = h
		}
	}
}

// WithClientID uses a client ID with request headers
func WithClientID(clientID string) Opts {
	return func(a *auth) {
		a.clientID = clientID
	}
}

// WithHandler includes a handler for a specific auth type
func WithHandler(authType string, hb HandlerBuild) Opts {
	return func(a *auth) {
		lcat := strings.ToLower(authType)
		a.hbs[lcat] = hb
		a.authTypes = append(a.authTypes, lcat)
	}
}

// WithDefaultHandlers includes a Basic and Bearer handler, this is automatically added with "WithHandler" is not called
func WithDefaultHandlers() Opts {
	return func(a *auth) {
		a.addDefaultHandlers()
	}
}

// WithLog injects a logrus Logger
func WithLog(log *logrus.Logger) Opts {
	return func(a *auth) {
		a.log = log
	}
}

func (a *auth) HandleResponse(resp *http.Response) error {
	/* 	- HandleResponse: parse 401 response, register/update auth method
	   	- Manage handlers in map based on URL's host field
	   	- Parse Www-Authenticate header
	   	- Switch based on scheme (basic/bearer)
	   	  - If handler doesn't exist, create handler
	   	  - Call handler specific HandleResponse
	*/
	a.mu.Lock()
	defer a.mu.Unlock()
	// verify response is an access denied
	if resp.StatusCode != http.StatusUnauthorized {
		return ErrUnsupported
	}

	// identify host for the request
	host := resp.Request.URL.Host
	// parse WWW-Authenticate header
	cl, err := ParseAuthHeaders(resp.Header.Values("WWW-Authenticate"))
	if err != nil {
		return err
	}
	a.log.WithFields(logrus.Fields{
		"challenge": cl,
	}).Debug("Auth request parsed")
	if len(cl) < 1 {
		return ErrEmptyChallenge
	}
	goodChallenge := false
	for _, c := range cl {
		if _, ok := a.hbs[c.authType]; !ok {
			a.log.WithFields(logrus.Fields{
				"authtype": c.authType,
			}).Warn("Unsupported auth type")
			continue
		}
		if _, ok := a.hs[host]; !ok {
			a.hs[host] = map[string]Handler{}
		}
		if _, ok := a.hs[host][c.authType]; !ok {
			h := a.hbs[c.authType](a.httpClient, a.clientID, host, a.credsFn(host))
			if h == nil {
				continue
			}
			a.hs[host][c.authType] = h
		}
		err := a.hs[host][c.authType].ProcessChallenge(c)
		if err == nil {
			goodChallenge = true
		} else if err == ErrNoNewChallenge {
			// handle race condition when another request updates the challenge
			// detect that by seeing the current auth header is different
			prevAH := resp.Request.Header.Get("Authorization")
			ah, err := a.hs[host][c.authType].GenerateAuth()
			if err == nil && prevAH != ah {
				goodChallenge = true
			}
		} else {
			return err
		}
	}
	if goodChallenge == false {
		return ErrUnauthorized
	}

	return nil
}

func (a *auth) UpdateRequest(req *http.Request) error {
	/* 	- UpdateRequest:
	   	- Lookup handler, noop if no handler for URL's host
	   	- Call handler updateRequest func, add returned header
	*/
	a.mu.Lock()
	defer a.mu.Unlock()
	host := req.URL.Host
	if a.hs[host] == nil {
		return nil
	}
	var err error
	var ah string
	for _, at := range a.authTypes {
		if a.hs[host][at] != nil {
			ah, err = a.hs[host][at].GenerateAuth()
			if err != nil {
				a.log.WithFields(logrus.Fields{
					"err":      err,
					"host":     host,
					"authtype": at,
				}).Debug("Failed to generate auth")
				continue
			}
			req.Header.Set("Authorization", ah)
			break
		}
	}
	return err
}

func (a *auth) addDefaultHandlers() {
	if _, ok := a.hbs["basic"]; !ok {
		a.hbs["basic"] = NewBasicHandler
		a.authTypes = append(a.authTypes, "basic")
	}
	if _, ok := a.hbs["bearer"]; !ok {
		a.hbs["bearer"] = NewBearerHandler
		a.authTypes = append(a.authTypes, "bearer")
	}
	if _, ok := a.hbs["jwt"]; !ok {
		a.hbs["jwt"] = NewJWTHandler
		a.authTypes = append(a.authTypes, "jwt")
	}
}

// DefaultCredsFn is used to return no credentials when auth is not configured with a CredsFn
// This avoids the need to check for nil pointers
func DefaultCredsFn(h string) Cred {
	return Cred{}
}

// ParseAuthHeaders extracts the scheme and realm from WWW-Authenticate headers
func ParseAuthHeaders(ahl []string) ([]Challenge, error) {
	var cl []Challenge
	for _, ah := range ahl {
		c, err := ParseAuthHeader(ah)
		if err != nil {
			return nil, fmt.Errorf("Failed to parse challenge header: %s", ah)
		}
		cl = append(cl, c...)
	}
	return cl, nil
}

// ParseAuthHeader parses a single header line for WWW-Authenticate
// Example values:
// Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:samalba/my-app:pull,push"
// Basic realm="GitHub Package Registry"
func ParseAuthHeader(ah string) ([]Challenge, error) {
	var cl []Challenge
	var c *Challenge
	var eb, atb, kb, vb []byte // eb is element bytes, atb auth type, kb key, vb value
	state := "string"

	for _, b := range []byte(ah) {
		switch state {
		case "string":
			if len(eb) == 0 {
				// beginning of string
				if b == '"' { // TODO: Invalid?
					state = "quoted"
				} else if charLUs[b]&isAlphaNum != 0 {
					// read any alphanum
					eb = append(eb, b)
				} else if charLUs[b]&isSpace != 0 {
					// ignore leading whitespace
				} else {
					// unknown leading char
					return nil, ErrParseFailure
				}
			} else {
				if charLUs[b]&isAlphaNum != 0 {
					// read any alphanum
					eb = append(eb, b)
				} else if b == '=' && len(atb) > 0 {
					// equals when authtype is defined makes this a key
					kb = eb
					eb = []byte{}
					state = "value"
				} else if charLUs[b]&isSpace != 0 {
					// space ends the element
					atb = eb
					eb = []byte{}
					c = &Challenge{authType: strings.ToLower(string(atb)), params: map[string]string{}}
					cl = append(cl, *c)
				} else {
					// unknown char
					return nil, ErrParseFailure
				}
			}

		case "value":
			if charLUs[b]&isAlphaNum != 0 {
				// read any alphanum
				vb = append(vb, b)
			} else if b == '"' && len(vb) == 0 {
				// quoted value
				state = "quoted"
			} else if charLUs[b]&isSpace != 0 || b == ',' {
				// space or comma ends the value
				c.params[strings.ToLower(string(kb))] = string(vb)
				kb = []byte{}
				vb = []byte{}
				if b == ',' {
					state = "string"
				} else {
					state = "endvalue"
				}
			} else {
				// unknown char
				return nil, ErrParseFailure
			}

		case "quoted":
			if b == '"' {
				// end quoted string
				c.params[strings.ToLower(string(kb))] = string(vb)
				kb = []byte{}
				vb = []byte{}
				state = "endvalue"
			} else if b == '\\' {
				state = "escape"
			} else {
				// all other bytes in a quoted string are taken as-is
				vb = append(vb, b)
			}

		case "endvalue":
			if charLUs[b]&isSpace != 0 {
				// ignore leading whitespace
			} else if b == ',' {
				// expect a comma separator, return to start of a string
				state = "string"
			} else {
				// unknown char
				return nil, ErrParseFailure
			}

		case "escape":
			vb = append(vb, b)
			state = "quoted"

		default:
			return nil, ErrParseFailure
		}
	}

	// process any content left at end of string, and handle any unfinished sections
	switch state {
	case "string":
		if len(eb) != 0 {
			atb = eb
			c = &Challenge{authType: strings.ToLower(string(atb)), params: map[string]string{}}
			cl = append(cl, *c)
		}
	case "value":
		if len(vb) != 0 {
			c.params[strings.ToLower(string(kb))] = string(vb)
		}
	case "quoted", "escape":
		return nil, ErrParseFailure
	}

	return cl, nil
}

// BasicHandler supports Basic auth type requests
type BasicHandler struct {
	realm string
	cred  Cred
}

// NewBasicHandler creates a new BasicHandler
func NewBasicHandler(client *http.Client, clientID, host string, cred Cred) Handler {
	return &BasicHandler{
		realm: "",
		cred:  cred,
	}
}

// ProcessChallenge for BasicHandler is a noop
func (b *BasicHandler) ProcessChallenge(c Challenge) error {
	if _, ok := c.params["realm"]; !ok {
		return ErrInvalidChallenge
	}
	if b.realm != c.params["realm"] {
		b.realm = c.params["realm"]
		return nil
	}
	return ErrNoNewChallenge
}

// GenerateAuth for BasicHandler generates base64 encoded user/pass for a host
func (b *BasicHandler) GenerateAuth() (string, error) {
	if b.cred.User == "" || b.cred.Password == "" {
		return "", ErrNotFound
	}
	auth := base64.StdEncoding.EncodeToString([]byte(b.cred.User + ":" + b.cred.Password))
	return fmt.Sprintf("Basic %s", auth), nil
}

// BearerHandler supports Bearer auth type requests
type BearerHandler struct {
	client         *http.Client
	clientID       string
	realm, service string
	cred           Cred
	scopes         []string
	token          BearerToken
}

// BearerToken is the json response to the Bearer request
type BearerToken struct {
	Token        string    `json:"token"`
	AccessToken  string    `json:"access_token"`
	ExpiresIn    int       `json:"expires_in"`
	IssuedAt     time.Time `json:"issued_at"`
	RefreshToken string    `json:"refresh_token"`
	Scope        string    `json:"scope"`
}

// NewBearerHandler creates a new BearerHandler
func NewBearerHandler(client *http.Client, clientID, host string, cred Cred) Handler {
	return &BearerHandler{
		client:   client,
		clientID: clientID,
		cred:     cred,
		realm:    "",
		service:  "",
		scopes:   []string{},
	}
}

// ProcessChallenge handles WWW-Authenticate header for bearer tokens
// Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:samalba/my-app:pull,push"
func (b *BearerHandler) ProcessChallenge(c Challenge) error {
	if _, ok := c.params["realm"]; !ok {
		return ErrInvalidChallenge
	}
	if _, ok := c.params["service"]; !ok {
		c.params["service"] = ""
	}
	if _, ok := c.params["scope"]; !ok {
		c.params["scope"] = ""
	}

	existingScope := b.scopeExists(c.params["scope"])

	if b.realm == c.params["realm"] && b.service == c.params["service"] && existingScope && (b.token.Token == "" || !b.isExpired()) {
		return ErrNoNewChallenge
	}

	if b.realm == "" {
		b.realm = c.params["realm"]
	} else if b.realm != c.params["realm"] {
		return ErrInvalidChallenge
	}
	if b.service == "" {
		b.service = c.params["service"]
	} else if b.service != c.params["service"] {
		return ErrInvalidChallenge
	}
	if !existingScope {
		b.scopes = append(b.scopes, c.params["scope"])
	}

	// delete any scope specific token
	b.token.Token = ""

	return nil
}

// GenerateAuth for BasicHandler generates base64 encoded user/pass for a host
func (b *BearerHandler) GenerateAuth() (string, error) {
	// if unexpired token already exists, return it
	if b.token.Token != "" && !b.isExpired() {
		return fmt.Sprintf("Bearer %s", b.token.Token), nil
	}

	// attempt to post with oauth form, this also uses refresh tokens
	if err := b.tryPost(); err == nil {
		return fmt.Sprintf("Bearer %s", b.token.Token), nil
	} else if err != ErrUnauthorized {
		return "", err
	}

	// attempt a get (with basic auth if user/pass available)
	if err := b.tryGet(); err == nil {
		return fmt.Sprintf("Bearer %s", b.token.Token), nil
	} else if err != ErrUnauthorized {
		return "", err
	}

	return "", ErrUnauthorized
}

// returns true when token issue date is either 0 or token is expired
func (b *BearerHandler) isExpired() bool {
	if b.token.IssuedAt.IsZero() {
		return true
	}
	return !time.Now().Before(b.token.IssuedAt.Add(time.Duration(b.token.ExpiresIn) * time.Second))
}

func (b *BearerHandler) tryGet() error {
	req, err := http.NewRequest("GET", b.realm, nil)
	if err != nil {
		return err
	}

	reqParams := req.URL.Query()
	reqParams.Add("client_id", b.clientID)
	reqParams.Add("offline_token", "true")
	if b.service != "" {
		reqParams.Add("service", b.service)
	}

	for _, s := range b.scopes {
		reqParams.Add("scope", s)
	}

	if b.cred.User != "" && b.cred.Password != "" {
		reqParams.Add("account", b.cred.User)
		req.SetBasicAuth(b.cred.User, b.cred.Password)
	}

	req.URL.RawQuery = reqParams.Encode()

	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return b.validateResponse(resp)
}

func (b *BearerHandler) tryPost() error {
	form := url.Values{}
	if len(b.scopes) > 0 {
		form.Set("scope", strings.Join(b.scopes, " "))
	}
	if b.service != "" {
		form.Set("service", b.service)
	}
	form.Set("client_id", b.clientID)
	if b.token.RefreshToken != "" {
		form.Set("grant_type", "refresh_token")
		form.Set("refresh_token", b.token.RefreshToken)
	} else if b.cred.User != "" && b.cred.Password != "" {
		form.Set("grant_type", "password")
		form.Set("username", b.cred.User)
		form.Set("password", b.cred.Password)
	}

	req, err := http.NewRequest("POST", b.realm, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")

	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return b.validateResponse(resp)
}

// check if the scope already exists within the list of scopes
func (b *BearerHandler) scopeExists(search string) bool {
	if search == "" {
		return true
	}
	for _, scope := range b.scopes {
		if scope == search {
			return true
		}
	}
	return false
}

func (b *BearerHandler) validateResponse(resp *http.Response) error {
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return ErrUnauthorized
	}

	decoder := json.NewDecoder(resp.Body)

	if err := decoder.Decode(&b.token); err != nil {
		return err
	}

	if b.token.ExpiresIn < minTokenLife {
		b.token.ExpiresIn = minTokenLife
	}

	if b.token.IssuedAt.IsZero() {
		b.token.IssuedAt = time.Now().UTC()
	}

	// AccessToken and Token should be the same and we use Token elsewhere
	if b.token.AccessToken != "" {
		b.token.Token = b.token.AccessToken
	}

	return nil
}

// JWTHandler supports JWT auth type requests
type JWTHubHandler struct {
	client   *http.Client
	clientID string
	realm    string
	cred     Cred
	jwt      string
}

type jwtHubPost struct {
	User string `json:"username"`
	Pass string `json:"password"`
}
type jwtHubResp struct {
	Detail       string `json:"detail"`
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token"`
}

// NewJWTHandler creates a new JWTHandler
func NewJWTHandler(client *http.Client, clientID, host string, cred Cred) Handler {
	// JWT handler is only tested against Hub, and the API is Hub specific
	if host == "hub.docker.com" {
		return &JWTHubHandler{
			client:   client,
			clientID: clientID,
			cred:     cred,
			realm:    "https://hub.docker.com/v2/users/login",
		}
	}
	return nil
}

// ProcessChallenge handles WWW-Authenticate header for JWT auth on Docker Hub
func (j *JWTHubHandler) ProcessChallenge(c Challenge) error {
	// use token if provided
	if j.cred.Token != "" {
		j.jwt = j.cred.Token
		return nil
	}

	// send a login request to hub
	bodyBytes, err := json.Marshal(jwtHubPost{
		User: j.cred.User,
		Pass: j.cred.Password,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", j.realm, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	req.Header.Add("User-Agent", j.clientID)

	resp, err := j.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ErrUnauthorized
	}

	var bodyParsed jwtHubResp
	err = json.Unmarshal(body, &bodyParsed)
	if err != nil {
		return err
	}
	j.jwt = bodyParsed.Token

	return nil
}

// GenerateAuth for JWTHubHandler adds JWT header
func (j *JWTHubHandler) GenerateAuth() (string, error) {
	if len(j.jwt) > 0 {
		return fmt.Sprintf("JWT %s", j.jwt), nil
	}
	return "", ErrUnauthorized
}
