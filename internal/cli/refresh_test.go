package cli

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/model"
)

func TestFirstWord(t *testing.T) {
	cases := map[string]string{
		"companies list":              "companies",
		"--verbose companies list":    "companies",
		"--timeout 5s companies list": "companies",
		"--timeout=5s people list":    "people",
		"--output x.json people list": "people",
		"-- companies":                "companies",
		"":                            "",
		"--force":                     "",
	}
	for in, want := range cases {
		if got := firstWord(strings.Fields(in)); got != want {
			t.Errorf("firstWord(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestModelFetchedOnceThenCached(t *testing.T) {
	srv := newFakeTwenty(t)
	a, _, stderr := newTestApp(t, srv, nil)
	if code := a.run([]string{"companies", "list"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	b, _, stderr2 := newTestApp(t, srv, nil)
	b.cacheDir = a.cacheDir
	if code := b.run([]string{"people", "list"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr2)
	}
	if srv.fetches() != 1 {
		t.Fatalf("%d OpenAPI fetches, want 1", srv.fetches())
	}
	if m := model.LoadCache(a.cacheRoot(), a.res.BaseURL, "ws-test"); m == nil || m.Find("invoices") == nil {
		t.Fatal("the model was not cached")
	}
}

func TestStaleCacheIsRefreshed(t *testing.T) {
	srv := newFakeTwenty(t)
	a, _, _ := newTestApp(t, srv, nil)
	seedCache(t, a, 25*time.Hour)
	a.run([]string{"companies", "list"})
	if srv.fetches() != 1 {
		t.Fatalf("%d fetches, want 1", srv.fetches())
	}
}

func TestFreshCacheIsUsed(t *testing.T) {
	srv := newFakeTwenty(t)
	a, _, _ := newTestApp(t, srv, nil)
	seedCache(t, a, time.Hour)
	a.run([]string{"companies", "list"})
	if srv.fetches() != 0 || len(srv.calls()) != 1 {
		t.Fatalf("%d fetches, %d calls", srv.fetches(), len(srv.calls()))
	}
}

func TestUnknownObjectTriggersOneRefresh(t *testing.T) {
	srv := newFakeTwenty(t)
	a, _, stderr := newTestApp(t, srv, nil)
	seedCache(t, a, time.Hour, "companies")
	if code := a.run([]string{"invoices", "list"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if srv.fetches() != 1 || srv.calls()[0].Path != "/rest/invoices" {
		t.Fatalf("%d fetches, calls %v", srv.fetches(), srv.calls())
	}
}

func openAPIFails(srv *fakeTwenty) {
	srv.openAPI = nil
	srv.handle = func(w http.ResponseWriter, r *http.Request, _ []byte) {
		if r.URL.Path == "/rest/open-api/core" {
			w.WriteHeader(500)
			return
		}
		io.WriteString(w, `{"data":{}}`)
	}
}

func TestRefreshFailureFallsBackToStaleCache(t *testing.T) {
	srv := newFakeTwenty(t)
	a, _, stderr := newTestApp(t, srv, nil)
	seedCache(t, a, 48*time.Hour)
	openAPIFails(srv)
	if code := a.run([]string{"companies", "list"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if len(srv.calls()) != 1 {
		t.Fatalf("calls = %v", srv.calls())
	}
}

func TestRefreshFailureWithoutCache(t *testing.T) {
	srv := newFakeTwenty(t)
	a, _, stderr := newTestApp(t, srv, nil)
	openAPIFails(srv)
	if code := a.run([]string{"companies", "list"}); code != 1 || errLine(t, stderr.String()).Kind != "server" {
		t.Fatalf("exit %d: %s", code, stderr)
	}
}

func TestEmptyDocumentMeansKeyProblem(t *testing.T) {
	srv := newFakeTwenty(t)
	srv.openAPI = readFixture(t, "openapi-skeleton.json")
	a, _, stderr := newTestApp(t, srv, nil)
	code := a.run([]string{"companies", "list"})
	e := errLine(t, stderr.String())
	if code != 1 || e.Kind != "auth" || !strings.Contains(e.Message, "auth status") {
		t.Fatalf("exit %d: %+v", code, e)
	}
}

func TestBuiltinsNeverFetch(t *testing.T) {
	srv := newFakeTwenty(t)
	a, _, _ := newTestApp(t, srv, nil)
	a.run([]string{"version"})
	a.run([]string{"metadata", "objects", "list"})
	a.run([]string{"--help"})
	if srv.fetches() != 0 {
		t.Fatalf("%d fetches", srv.fetches())
	}
}

func TestShadowedObjectIsNotACommand(t *testing.T) {
	srv := newFakeTwenty(t)
	a, _, _ := newTestApp(t, srv, nil)
	a.model = &model.Model{Objects: []model.Object{
		{Command: "companies", NamePlural: "companies", NameSingular: "company"},
		{Command: "config", NamePlural: "config", NameSingular: "configEntry"},
	}}
	root := a.newRoot()
	for _, c := range root.Commands() {
		if c.Name() == "config" {
			for _, sub := range c.Commands() {
				if sub.Name() == "list" {
					t.Fatal("an object named config shadowed the built-in")
				}
			}
		}
	}
}
