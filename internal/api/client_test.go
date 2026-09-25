package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type hits struct{ n atomic.Int32 }

func serve(t *testing.T, h func(w http.ResponseWriter, r *http.Request, n int32)) (*httptest.Server, *hits) {
	t.Helper()
	c := &hits{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h(w, r, c.n.Add(1))
	}))
	t.Cleanup(srv.Close)
	return srv, c
}

func newClient(srv *httptest.Server) (*Client, *[]time.Duration) {
	var waits []time.Duration
	return &Client{BaseURL: srv.URL, APIKey: "k.e.y", Sleep: func(d time.Duration) { waits = append(waits, d) }}, &waits
}

func asError(t *testing.T, err error) *Error {
	t.Helper()
	e, ok := err.(*Error)
	if !ok {
		t.Fatalf("error %T %v, want *Error", err, err)
	}
	return e
}

func TestDoSendsBearerAndJSON(t *testing.T) {
	srv, _ := serve(t, func(w http.ResponseWriter, r *http.Request, _ int32) {
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get("Authorization") != "Bearer k.e.y" || r.Header.Get("Accept") != "application/json" ||
			r.Header.Get("Content-Type") != "application/json" || r.URL.Path != "/rest/companies" ||
			r.URL.Query().Get("upsert") != "true" || string(body) != `{"name":"Acme"}` {
			t.Errorf("request = %s %s %v %s", r.Method, r.URL, r.Header, body)
		}
		w.WriteHeader(201)
		io.WriteString(w, `{"data":{"createCompany":{"id":"x"}}}`)
	})
	c, _ := newClient(srv)
	resp, err := c.Do(context.Background(), Request{Method: "POST", Path: "rest/companies",
		Query: url.Values{"upsert": {"true"}}, Body: []byte(`{"name":"Acme"}`), Risk: "write"})
	if err != nil || resp.Status != 201 || !strings.Contains(string(resp.Body), "createCompany") {
		t.Fatalf("Do = %+v, %v", resp, err)
	}
}

func TestGuardRefusesBeforeNetwork(t *testing.T) {
	srv, h := serve(t, func(w http.ResponseWriter, r *http.Request, _ int32) {})
	base := Request{Method: "GET", Path: "rest/companies", Risk: "read"}
	cases := []struct {
		name   string
		mutate func(*Client, *Request)
		want   string
	}{
		{"no base URL", func(c *Client, r *Request) { c.BaseURL = "" }, "no Twenty base URL"},
		{"no key", func(c *Client, r *Request) { c.APIKey = "" }, "no API key"},
		{"plain http", func(c *Client, r *Request) { c.BaseURL = "http://crm.example.com" }, "plain HTTP"},
		{"read-only write", func(c *Client, r *Request) { c.ReadOnly = true; r.Method, r.Risk = "POST", "write" }, "read-only mode"},
		{"read-only unclassified GET", func(c *Client, r *Request) { c.ReadOnly = true; r.Risk = "" }, "read-only mode"},
		{"non-GET without class", func(c *Client, r *Request) { r.Method, r.Risk = "PATCH", "" }, "no risk class"},
		{"body on GET", func(c *Client, r *Request) { r.Body = []byte(`{}`) }, "request body"},
		{"body on DELETE", func(c *Client, r *Request) { r.Method, r.Risk, r.Body = "DELETE", "write", []byte(`{}`) }, "request body"},
		{"authorization header", func(c *Client, r *Request) { r.Headers = http.Header{"authorization": {"x"}} }, "Authorization"},
		{"method override", func(c *Client, r *Request) { r.Headers = http.Header{"X-HTTP-Method-Override": {"DELETE"}} }, "method-override"},
		{"method override underscore", func(c *Client, r *Request) { r.Headers = http.Header{"x_http_method": {"DELETE"}} }, "method-override"},
	}
	for _, tc := range cases {
		c, _ := newClient(srv)
		r := base
		tc.mutate(c, &r)
		_, err := c.Do(context.Background(), r)
		e := asError(t, err)
		if e.Kind != KindUsage || !strings.Contains(e.Message, tc.want) {
			t.Errorf("%s: %+v, want usage containing %q", tc.name, e, tc.want)
		}
	}
	if h.n.Load() != 0 {
		t.Fatalf("%d requests reached the server", h.n.Load())
	}
}

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		status int
		kind   string
	}{{400, KindValidation}, {422, KindValidation}, {401, KindAuth}, {403, KindForbidden}, {404, KindNotFound},
		{409, KindConflict}, {410, KindNotFound}}
	for _, tc := range cases {
		srv, _ := serve(t, func(w http.ResponseWriter, r *http.Request, _ int32) {
			w.WriteHeader(tc.status)
			io.WriteString(w, `{"statusCode":400,"messages":["first problem","second problem"],"error":"BAD_REQUEST"}`)
		})
		c, _ := newClient(srv)
		_, err := c.Do(context.Background(), Request{Method: "GET", Path: "rest/companies", Risk: "read"})
		e := asError(t, err)
		if e.Kind != tc.kind || e.Status != tc.status ||
			!strings.Contains(e.Message, "GET rest/companies: HTTP ") || !strings.Contains(e.Message, "first problem (and 1 more)") {
			t.Errorf("status %d: %+v", tc.status, e)
		}
		if d, ok := e.Details.(map[string]any); !ok || d["error"] != "BAD_REQUEST" {
			t.Errorf("status %d: details = %#v", tc.status, e.Details)
		}
	}
}

func TestNestJSMessageShapeAndNonJSONBody(t *testing.T) {
	srv, _ := serve(t, func(w http.ResponseWriter, r *http.Request, _ int32) {
		if r.URL.Path == "/rest/x" {
			w.WriteHeader(404)
			io.WriteString(w, `{"statusCode":404,"message":"Cannot GET /rest/x","error":"Not Found"}`)
			return
		}
		w.WriteHeader(418)
		io.WriteString(w, "<html>teapot</html>")
	})
	c, _ := newClient(srv)
	_, err := c.Do(context.Background(), Request{Method: "GET", Path: "rest/x", Risk: "read"})
	if e := asError(t, err); !strings.Contains(e.Message, "Cannot GET /rest/x") {
		t.Errorf("NestJS message lost: %+v", e)
	}
	_, err = c.Do(context.Background(), Request{Method: "GET", Path: "rest/y", Risk: "read"})
	if e := asError(t, err); e.Details != "<html>teapot</html>" {
		t.Errorf("non-JSON details = %#v", e.Details)
	}
}

func TestHints(t *testing.T) {
	cases := []struct {
		status        int
		path, object  string
		message, want string
	}{
		{401, "rest/companies", "", "This API Key has expired", "config set api-key"},
		{401, "rest/companies", "", "Token invalid", "auth status"},
		{403, "rest/metadata/objects", "", "Forbidden", "Data Model permission"},
		{403, "rest/metadata/fields/x", "", "Forbidden", "Data Model permission"},
		{403, "rest/companies", "companies", "Forbidden", "Settings, Roles"},
		{400, "rest/companies", "companies", `Object company doesn't have any "foo" field.`, "twentycrm schema companies"},
		{400, "rest/companies", "", `unknown field`, ""},
	}
	for _, tc := range cases {
		srv, _ := serve(t, func(w http.ResponseWriter, r *http.Request, _ int32) {
			w.WriteHeader(tc.status)
			io.WriteString(w, `{"statusCode":1,"messages":[`+jsonString(tc.message)+`],"error":"X"}`)
		})
		c, _ := newClient(srv)
		_, err := c.Do(context.Background(), Request{Method: "GET", Path: tc.path, Risk: "read", Object: tc.object})
		e := asError(t, err)
		if tc.want == "" && strings.Contains(e.Message, "hint") || tc.want != "" && !strings.Contains(e.Message, tc.want) {
			t.Errorf("%d %s: %s, want %q", tc.status, tc.path, e.Message, tc.want)
		}
	}
}

func jsonString(s string) string { return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"` }

func TestRetry429ThenSuccess(t *testing.T) {
	srv, h := serve(t, func(w http.ResponseWriter, r *http.Request, n int32) {
		if n == 1 {
			w.WriteHeader(429)
			io.WriteString(w, `{"statusCode":429,"messages":["Too many requests"],"error":"TOO_MANY_REQUESTS"}`)
			return
		}
		io.WriteString(w, `{}`)
	})
	c, waits := newClient(srv)
	if _, err := c.Do(context.Background(), Request{Method: "POST", Path: "rest/companies", Body: []byte(`{}`), Risk: "write"}); err != nil {
		t.Fatal(err)
	}
	if h.n.Load() != 2 || len(*waits) != 1 || (*waits)[0] < time.Second {
		t.Fatalf("hits %d waits %v", h.n.Load(), *waits)
	}
}

func TestRetryAfterBeyondBudgetFailsAtOnce(t *testing.T) {
	srv, h := serve(t, func(w http.ResponseWriter, r *http.Request, _ int32) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(429)
	})
	c, waits := newClient(srv)
	_, err := c.Do(context.Background(), Request{Method: "GET", Path: "rest/companies", Risk: "read"})
	e := asError(t, err)
	if e.Kind != KindRateLimited || !strings.Contains(e.Message, "retry budget") || !strings.Contains(e.Message, "100 requests per minute") ||
		h.n.Load() != 1 || len(*waits) != 0 {
		t.Fatalf("%+v hits %d waits %v", e, h.n.Load(), *waits)
	}
}

func TestServerErrorsRetriedForReadsOnly(t *testing.T) {
	srv, h := serve(t, func(w http.ResponseWriter, r *http.Request, _ int32) { w.WriteHeader(502) })
	c, _ := newClient(srv)
	_, err := c.Do(context.Background(), Request{Method: "GET", Path: "rest/companies", Risk: "read"})
	if e := asError(t, err); e.Kind != KindServer || h.n.Load() != 3 {
		t.Fatalf("read: %+v after %d hits", e, h.n.Load())
	}
	h.n.Store(0)
	_, err = c.Do(context.Background(), Request{Method: "PATCH", Path: "rest/companies/x", Body: []byte(`{}`), Risk: "write"})
	if e := asError(t, err); e.Kind != KindServer || h.n.Load() != 1 {
		t.Fatalf("write: %+v after %d hits", e, h.n.Load())
	}
}

func TestBrokenConnection(t *testing.T) {
	srv, h := serve(t, func(w http.ResponseWriter, r *http.Request, _ int32) {
		conn, _, _ := w.(http.Hijacker).Hijack()
		conn.Close()
	})
	c, _ := newClient(srv)
	_, err := c.Do(context.Background(), Request{Method: "POST", Path: "rest/companies", Body: []byte(`{}`), Risk: "write"})
	if e := asError(t, err); e.Kind != KindOutcomeUnknown || h.n.Load() != 1 {
		t.Fatalf("write: %+v after %d hits", e, h.n.Load())
	}
	h.n.Store(0)
	_, err = c.Do(context.Background(), Request{Method: "GET", Path: "rest/companies", Risk: "read"})
	if e := asError(t, err); e.Kind != KindTransport || h.n.Load() != 3 {
		t.Fatalf("read: %+v after %d hits", e, h.n.Load())
	}
}

func TestRedirectRefused(t *testing.T) {
	srv, h := serve(t, func(w http.ResponseWriter, r *http.Request, _ int32) {
		http.Redirect(w, r, "https://elsewhere.example.com/rest/companies", http.StatusFound)
	})
	c, _ := newClient(srv)
	_, err := c.Do(context.Background(), Request{Method: "GET", Path: "rest/companies", Risk: "read"})
	e := asError(t, err)
	if e.Kind != KindValidation || !strings.Contains(e.Message, "elsewhere.example.com") || h.n.Load() != 1 {
		t.Fatalf("%+v after %d hits", e, h.n.Load())
	}
}
