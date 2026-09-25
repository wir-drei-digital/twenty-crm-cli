// Package api is the HTTP client for Twenty's REST API. It owns the base URL
// guard, the headers, the retry policy and the error shape.
package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/config"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/routes"
)

// Client performs API requests. BaseURL and APIKey are required; Timeout,
// RetryBudget and MaxAttempts fall back to defaults.
type Client struct {
	BaseURL     string // scheme://host[:port], as config.NormalizeBaseURL returns it
	APIKey      string
	ReadOnly    bool
	Timeout     time.Duration // per attempt; default 30s
	RetryBudget time.Duration // total wait on 429; default 60s
	MaxAttempts int           // attempts for transient failures (read-class, or dial/DNS on any call); default 3
	Verbose     io.Writer     // nil = silent; never receives the key
	HTTP        *http.Client
	Sleep       func(time.Duration) // nil = a wait that ends early when the context is done
}

// Request is one API call.
type Request struct {
	Method string
	Path   string // relative to BaseURL: "rest/companies"
	Query  url.Values
	Body   []byte
	// Risk is the routes class the caller settled on. Every non-GET request
	// needs one. Only read-class requests are retried after a transient
	// failure, and only they pass read-only mode.
	Risk string
	// Object is the object's command name, for the schema hint; "" otherwise.
	Object string
	// Headers are applied after the defaults. Authorization is the client's
	// alone and cannot be set here.
	Headers http.Header
}

// Response is a successful (2xx) response.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
}

func (r Request) readSafe() bool { return r.Risk == routes.ClassRead }

func (c *Client) logf(format string, a ...any) {
	if c.Verbose != nil {
		fmt.Fprintf(c.Verbose, format+"\n", a...)
	}
}

// methodOverrideHeaders make some servers treat a request as another method,
// so a GET could be applied as a DELETE the guard never classified.
var methodOverrideHeaders = []string{"x-http-method-override", "x-http-method", "x-method-override"}

// headerIs compares header names the way proxies may: case-insensitively,
// with "_" and "-" as the same character.
func headerIs(k, name string) bool { return strings.EqualFold(strings.ReplaceAll(k, "_", "-"), name) }

// shape rejects a request that could change something the guard never saw.
func (r Request) shape() *Error {
	method := strings.ToUpper(r.Method)
	if method == "" {
		method = http.MethodGet
	}
	if r.Body != nil && (method == http.MethodGet || method == http.MethodDelete) {
		return Usagef("refusing to send a request body with %s %s: %s takes no body, so the guard never inspects one", method, r.Path, method)
	}
	if method != http.MethodGet && r.Risk == "" {
		return Usagef("refusing to send %s %s: no risk class was assigned, so the guard has not checked it", method, r.Path)
	}
	return nil
}

// guard rejects a request locally, before anything is sent.
func (c *Client) guard(r Request) *Error {
	if err := r.shape(); err != nil {
		return err
	}
	if c.BaseURL == "" {
		return Usagef("no Twenty base URL: set TWENTY_BASE_URL, run `twentycrm config set base-url <url>`, or run `twentycrm init`")
	}
	if c.APIKey == "" {
		return Usagef("no API key: set TWENTY_API_KEY, store one with `twentycrm config set api-key < key.txt`, or run `twentycrm init`")
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Host == "" {
		return Usagef("invalid base URL %q", c.BaseURL)
	}
	if u.Scheme != "https" && !config.IsLoopback(strings.ToLower(u.Hostname())) {
		return Usagef("refusing to send the API key over plain HTTP to %s; use https", u.Host)
	}
	if c.ReadOnly && !r.readSafe() {
		return Usagef("read-only mode (TWENTY_READ_ONLY or `config set read-only true`) blocks %s %s", r.Method, r.Path)
	}
	for k := range r.Headers {
		if headerIs(k, "authorization") {
			return Usagef("the Authorization header belongs to the client and cannot be set")
		}
		for _, name := range methodOverrideHeaders {
			if headerIs(k, name) {
				return Usagef("refusing to send the %s header: a method-override header could apply %s %s as another method the guard never checked", k, r.Method, r.Path)
			}
		}
	}
	return nil
}

// refuseRedirects never follows a redirect: Go keeps Authorization across a
// same-host https to http hop, and a 307/308 would replay a change.
func refuseRedirects(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

// attempt performs one round trip; Do wraps it with the retry loop.
func (c *Client) attempt(ctx context.Context, r Request) (*Response, error) {
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	u := strings.TrimRight(c.BaseURL, "/") + "/" + strings.TrimLeft(r.Path, "/")
	if len(r.Query) > 0 {
		u += "?" + r.Query.Encode()
	}
	var body io.Reader
	if r.Body != nil {
		body = bytes.NewReader(r.Body)
	}
	req, err := http.NewRequestWithContext(ctx, r.Method, u, body)
	if err != nil {
		return nil, &Error{Kind: KindUsage, Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Accept", "application/json")
	if r.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, vs := range r.Headers {
		req.Header[http.CanonicalHeaderKey(k)] = vs
	}
	c.logf("> %s %s", r.Method, r.Path)
	httpClient := &http.Client{}
	if c.HTTP != nil {
		cp := *c.HTTP
		httpClient = &cp
	}
	httpClient.CheckRedirect = refuseRedirects
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	c.logf("< %d (%d bytes)", resp.StatusCode, len(raw))
	return &Response{Status: resp.StatusCode, Header: resp.Header, Body: raw}, nil
}

// sleepCtx waits d, or less when ctx is done first.
func sleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
	}
}

// Do sends a request with the retry policy: 429 is retried for every call
// within the retry budget; network failures and 5xx are retried for
// read-class calls, and a dial or DNS failure, which never reached Twenty,
// for every call. A change is never replayed. Cancelling ctx ends a retry
// wait. The error is always *Error.
func (c *Client) Do(ctx context.Context, r Request) (*Response, error) {
	if err := c.guard(r); err != nil {
		return nil, err
	}
	sleep := c.Sleep
	if sleep == nil {
		sleep = func(d time.Duration) { sleepCtx(ctx, d) }
	}
	budget := c.RetryBudget
	if budget == 0 {
		budget = 60 * time.Second
	}
	maxAttempts := c.MaxAttempts
	if maxAttempts == 0 {
		maxAttempts = 3
	}
	var waited time.Duration
	transientTries := 0
	backoff := time.Second
	// pause waits before the next attempt. A context cancelled meanwhile
	// ends the call: another attempt would only fail, and for a change it
	// would turn a clean refusal into an unknown outcome.
	pause := func(d time.Duration) *Error {
		sleep(d)
		if err := ctx.Err(); err != nil {
			return &Error{Kind: KindTransport, Message: fmt.Sprintf("%s %s: cancelled while waiting to retry: %v", r.Method, r.Path, err)}
		}
		return nil
	}
	for {
		resp, err := c.attempt(ctx, r)
		if err != nil {
			var apiErr *Error
			if errors.As(err, &apiErr) {
				return nil, apiErr
			}
			e := classifyTransport(err, r.readSafe())
			// classifyTransport says transport for a read-class call or a
			// dial or DNS failure: both are safe to send again.
			if e.Kind == KindTransport {
				transientTries++
				if transientTries < maxAttempts {
					if e := pause(backoff); e != nil {
						return nil, e
					}
					backoff *= 2
					continue
				}
			}
			e.Message = fmt.Sprintf("%s %s: %s", r.Method, r.Path, e.Message)
			return nil, e
		}
		switch {
		case resp.Status >= 200 && resp.Status < 300:
			return resp, nil
		case resp.Status >= 300 && resp.Status < 400:
			return nil, &Error{
				Kind: KindValidation, Status: resp.Status, Details: errorDetails(resp),
				Message: fmt.Sprintf("%s %s: HTTP %d redirect to %q refused: twentycrm never follows redirects "+
					"(they can strip HTTPS off the key and replay a change); check the base URL",
					r.Method, r.Path, resp.Status, resp.Header.Get("Location")),
			}
		case resp.Status == http.StatusTooManyRequests:
			wait := max(retryAfter(resp.Header), backoff) + jitter()
			if waited+wait > budget {
				return nil, errorFor(r, resp, fmt.Sprintf("; Twenty asks to wait %s, beyond the %s retry budget", wait.Round(time.Second), budget))
			}
			waited += wait
			c.logf("* 429, waiting %s", wait)
			if e := pause(wait); e != nil {
				return nil, e
			}
			backoff *= 2
			continue
		case resp.Status >= 500 && r.readSafe():
			transientTries++
			if transientTries < maxAttempts {
				if e := pause(backoff); e != nil {
					return nil, e
				}
				backoff *= 2
				continue
			}
		}
		return nil, errorFor(r, resp, "")
	}
}
