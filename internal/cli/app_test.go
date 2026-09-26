package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/api"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/config"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/model"
)

const testID = "3f2b8c1e-5d4a-4c1b-9e8f-1a2b3c4d5e6f"

// fakeTwenty is a Twenty stand-in: it serves the fixture OpenAPI document
// (unless openAPI is nil) and records every request. handle, when set,
// answers everything else; without it every other request gets {"data":{}}.
type fakeTwenty struct {
	*httptest.Server
	mu      sync.Mutex
	reqs    []recorded
	openAPI []byte
	handle  func(w http.ResponseWriter, r *http.Request, body []byte)
}

type recorded struct {
	Method, Path, RawQuery, Body string
	Query                        url.Values
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "model", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func newFakeTwenty(t *testing.T) *fakeTwenty {
	t.Helper()
	f := &fakeTwenty{openAPI: readFixture(t, "openapi-core.json")}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.reqs = append(f.reqs, recorded{Method: r.Method, Path: r.URL.Path, RawQuery: r.URL.RawQuery, Body: string(body), Query: r.URL.Query()})
		h, doc := f.handle, f.openAPI
		f.mu.Unlock()
		if r.URL.Path == "/rest/open-api/core" && doc != nil {
			w.Write(doc)
			return
		}
		if h != nil {
			h(w, r, body)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"data":{}}`)
	}))
	t.Cleanup(f.Close)
	return f
}

// calls returns every request except the OpenAPI fetches.
func (f *fakeTwenty) calls() []recorded {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []recorded
	for _, r := range f.reqs {
		if r.Path != "/rest/open-api/core" {
			out = append(out, r)
		}
	}
	return out
}

func (f *fakeTwenty) fetches() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.reqs {
		if r.Path == "/rest/open-api/core" {
			n++
		}
	}
	return n
}

// testToken builds an unsigned JWT with the given payload; the CLI never
// verifies signatures.
func testToken(t *testing.T, payload map[string]any) string {
	t.Helper()
	enc := func(v any) string {
		raw, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	return enc(map[string]string{"alg": "HS256", "typ": "JWT"}) + "." + enc(payload) + ".c2ln"
}

// testKey is an API key for workspace with key ID key-1, expiring at exp
// (never when exp is zero).
func testKey(t *testing.T, workspace string, exp time.Time) string {
	t.Helper()
	payload := map[string]any{"sub": workspace, "type": "API_KEY", "workspaceId": workspace, "jti": "key-1"}
	if !exp.IsZero() {
		payload["exp"] = exp.Unix()
	}
	return testToken(t, payload)
}

// newTestApp wires an app to srv with its own cache directory. edit may
// change the resolved configuration before the client is built.
func newTestApp(t *testing.T, srv *fakeTwenty, edit func(*config.Resolved)) (*app, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	res := config.Resolved{BaseURL: srv.URL, BaseURLSource: "env", KeySource: "env",
		APIKey: testKey(t, "ws-test", time.Now().Add(30*24*time.Hour))}
	if edit != nil {
		edit(&res)
	}
	var out, errb bytes.Buffer
	cache := t.TempDir()
	a := &app{res: res, stdout: &out, stderr: &errb, stdin: strings.NewReader(""),
		cacheDir: func() (string, error) { return cache, nil }}
	a.client = &api.Client{BaseURL: res.BaseURL, APIKey: res.APIKey, ReadOnly: res.ReadOnly, Sleep: func(time.Duration) {}}
	return a, &out, &errb
}

// errLine parses the single JSON error line on stderr.
func errLine(t *testing.T, stderr string) api.Error {
	t.Helper()
	lines := strings.Split(strings.TrimRight(stderr, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("stderr has %d lines, want 1: %q", len(lines), stderr)
	}
	var e api.Error
	if err := json.Unmarshal([]byte(lines[0]), &e); err != nil {
		t.Fatalf("stderr is not JSON: %q", stderr)
	}
	return e
}

// seedCache writes a cached model for a's base URL and key holding the
// fixture objects whose command is listed (all of them when none is),
// fetched age ago.
func seedCache(t *testing.T, a *app, age time.Duration, commands ...string) {
	t.Helper()
	objs, err := model.Extract(readFixture(t, "openapi-core.json"))
	if err != nil {
		t.Fatal(err)
	}
	var keep []model.Object
	for _, o := range objs {
		if len(commands) == 0 || contains(commands, o.Command) {
			keep = append(keep, o)
		}
	}
	m := &model.Model{FetchedAt: time.Now().Add(-age), BaseURL: a.res.BaseURL, WorkspaceID: a.workspaceID(), Objects: keep}
	if err := model.SaveCache(a.cacheRoot(), m); err != nil {
		t.Fatal(err)
	}
}
