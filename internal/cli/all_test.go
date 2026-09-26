package cli

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

// pages serves three pages of companies keyed by starting_after.
func pages(srv *fakeTwenty, lastCursor string) {
	srv.handle = func(w http.ResponseWriter, r *http.Request, _ []byte) {
		switch r.URL.Query().Get("starting_after") {
		case "":
			io.WriteString(w, `{"data":{"companies":[{"id":"1"},{"id":"2"}]},"totalCount":5,"pageInfo":{"hasNextPage":true,"endCursor":"c1"}}`)
		case "c1":
			io.WriteString(w, `{"data":{"companies":[{"id":"3"},{"id":"4"}]},"totalCount":5,"pageInfo":{"hasNextPage":true,"endCursor":"`+lastCursor+`"}}`)
		default:
			io.WriteString(w, `{"data":{"companies":[{"id":"5"}]},"totalCount":5,"pageInfo":{"hasNextPage":false,"endCursor":"c3"}}`)
		}
	}
}

func TestAllMergesPagesAndCarriesTheQuery(t *testing.T) {
	srv := newFakeTwenty(t)
	pages(srv, "c2")
	a, stdout, stderr := newTestApp(t, srv, nil)
	if code := a.run([]string{"companies", "list", "--all", "--filter", "stage[eq]:LEAD", "--order-by", "name"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if got := stdout.String(); got != `[{"id":"1"},{"id":"2"},{"id":"3"},{"id":"4"},{"id":"5"}]`+"\n" {
		t.Fatalf("stdout = %s", got)
	}
	calls := srv.calls()
	if len(calls) != 3 {
		t.Fatalf("%d calls", len(calls))
	}
	for i, want := range []string{"", "c1", "c2"} {
		q := calls[i].Query
		if q.Get("starting_after") != want || q.Get("limit") != "200" || q.Get("filter") != "stage[eq]:LEAD" || q.Get("order_by") != "name" {
			t.Errorf("page %d query = %v", i+1, q)
		}
	}
}

func TestAllHonoursLimit(t *testing.T) {
	srv := newFakeTwenty(t)
	pages(srv, "c2")
	a, _, _ := newTestApp(t, srv, nil)
	a.run([]string{"companies", "list", "--all", "--limit", "50"})
	for _, c := range srv.calls() {
		if c.Query.Get("limit") != "50" {
			t.Fatalf("limit = %q", c.Query.Get("limit"))
		}
	}
}

func TestAllMaxPages(t *testing.T) {
	srv := newFakeTwenty(t)
	pages(srv, "c2")
	a, stdout, stderr := newTestApp(t, srv, nil)
	code := a.run([]string{"companies", "list", "--all", "--max-pages", "2"})
	if code != 1 || errLine(t, stderr.String()).Kind != "incomplete" {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if stdout.String() != `[{"id":"1"},{"id":"2"},{"id":"3"},{"id":"4"}]`+"\n" {
		t.Fatalf("partial output = %s", stdout)
	}
}

func TestAllStopsWithoutCursor(t *testing.T) {
	srv := newFakeTwenty(t)
	pages(srv, "")
	a, stdout, stderr := newTestApp(t, srv, nil)
	code := a.run([]string{"companies", "list", "--all"})
	e := errLine(t, stderr.String())
	if code != 1 || e.Kind != "server" || !strings.Contains(e.Message, "endCursor") {
		t.Fatalf("exit %d: %+v", code, e)
	}
	if stdout.String() != `[{"id":"1"},{"id":"2"},{"id":"3"},{"id":"4"}]`+"\n" || len(srv.calls()) != 2 {
		t.Fatalf("stdout %s after %d calls", stdout, len(srv.calls()))
	}
}

func TestAllRejectsEndingBefore(t *testing.T) {
	srv := newFakeTwenty(t)
	a, _, stderr := newTestApp(t, srv, nil)
	if code := a.run([]string{"companies", "list", "--all", "--ending-before", "c9"}); code != 2 || len(srv.calls()) != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
}

func TestAllMetadataShapes(t *testing.T) {
	for body, want := range map[string]string{
		`{"data":[{"id":"a"}],"pageInfo":{"hasNextPage":false},"totalCount":1}`: `[{"id":"a"}]`,
		`{"data":{"objects":[{"id":"b"}]},"pageInfo":{"hasNextPage":false}}`:    `[{"id":"b"}]`,
	} {
		srv := newFakeTwenty(t)
		srv.handle = func(w http.ResponseWriter, r *http.Request, _ []byte) { io.WriteString(w, body) }
		a, stdout, stderr := newTestApp(t, srv, nil)
		if code := a.run([]string{"metadata", "objects", "list", "--all"}); code != 0 {
			t.Fatalf("exit %d: %s", code, stderr)
		}
		if stdout.String() != want+"\n" || srv.calls()[0].Query.Get("limit") != "1000" {
			t.Errorf("%s: stdout %s, query %v", body, stdout, srv.calls()[0].Query)
		}
	}
}

func TestAllUnexpectedShape(t *testing.T) {
	srv := newFakeTwenty(t)
	srv.handle = func(w http.ResponseWriter, r *http.Request, _ []byte) {
		fmt.Fprint(w, `{"data":{"somethingElse":[]}}`)
	}
	a, _, stderr := newTestApp(t, srv, nil)
	if code := a.run([]string{"companies", "list", "--all"}); code != 1 || errLine(t, stderr.String()).Kind != "server" {
		t.Fatalf("exit %d: %s", code, stderr)
	}
}
