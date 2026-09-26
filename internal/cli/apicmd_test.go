package cli

import (
	"strings"
	"testing"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/config"
)

func TestAPIRequests(t *testing.T) {
	cases := []struct {
		args         []string
		method, path string
		query        string
	}{
		{[]string{"api", "GET", "rest/companies", "--query", "limit=1"}, "GET", "/rest/companies", "limit=1"},
		{[]string{"api", "delete", "/rest/companies/" + testID, "--query", "soft_delete=true"}, "DELETE", "/rest/companies/" + testID, "soft_delete=true"},
		{[]string{"api", "POST", "rest/metadata/fields", "--data", `{"name":"x"}`, "--force"}, "POST", "/rest/metadata/fields", ""},
		{[]string{"api", "PATCH", "rest/companies", "--query", "filter=a[eq]:1", "--data", `{}`, "--force"}, "PATCH", "/rest/companies", "filter=a%5Beq%5D%3A1"},
	}
	for _, c := range cases {
		srv := newFakeTwenty(t)
		a, _, stderr := newTestApp(t, srv, nil)
		if code := a.run(c.args); code != 0 {
			t.Errorf("%v: exit %d %s", c.args, code, stderr)
			continue
		}
		got := srv.calls()
		if len(got) != 1 || got[0].Method != c.method || got[0].Path != c.path || got[0].RawQuery != c.query {
			t.Errorf("%v: sent %+v", c.args, got)
		}
	}
}

func TestAPIDoesNotFetchTheModel(t *testing.T) {
	srv := newFakeTwenty(t)
	a, _, _ := newTestApp(t, srv, nil)
	a.run([]string{"api", "GET", "rest/companies"})
	if srv.fetches() != 0 {
		t.Fatal("api must not fetch the model")
	}
}

func TestAPIGates(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"api", "DELETE", "rest/companies/" + testID}, "needs --force"},
		{[]string{"api", "DELETE", "rest/companies/" + testID, "--query", "soft_delete=TRUE"}, "needs --force"},
		{[]string{"api", "PATCH", "rest/companies", "--data", `{}`, "--force"}, "needs --filter"},
		{[]string{"api", "POST", "graphql", "--data", `{}`}, "GraphQL"},
		{[]string{"api", "GET", "rest/companies/%2e%2e"}, "not allowed"},
		{[]string{"api", "GET", "rest/companies?limit=1"}, "--query"},
		{[]string{"api", "GET", "rest/companies", "--header", "Authorization: Bearer x"}, "Authorization"},
		{[]string{"api", "GET", "rest/companies", "--header", "X-HTTP-Method-Override: DELETE"}, "method-override"},
		{[]string{"api", "DELETE", "rest/companies/" + testID, "--query", "soft_delete=true", "--query", "soft_delete=true"}, "only once"},
		{[]string{"api", "POST", "rest/metadata/apiKeys", "--data", `{}`, "--force"}, "blocked"},
		{[]string{"api", "GET", "rest/companies", "--data", `{}`}, "body"},
		{[]string{"api", "GET", "rest/companies", "--query", "novalue"}, "k=v"},
		{[]string{"api", "TRACE", "rest/companies"}, "method"},
	}
	for _, c := range cases {
		srv := newFakeTwenty(t)
		a, _, stderr := newTestApp(t, srv, nil)
		code := a.run(c.args)
		if code != 2 || !strings.Contains(errLine(t, stderr.String()).Message, c.want) {
			t.Errorf("%v: exit %d %s, want %q", c.args, code, stderr, c.want)
		}
		if len(srv.reqs) != 0 {
			t.Errorf("%v: %d requests reached the server", c.args, len(srv.reqs))
		}
	}
}

func TestAPIReadOnly(t *testing.T) {
	srv := newFakeTwenty(t)
	a, _, stderr := newTestApp(t, srv, func(r *config.Resolved) { r.ReadOnly = true })
	a.client.ReadOnly = true
	if code := a.run([]string{"api", "POST", "rest/companies", "--data", `{}`}); code != 2 {
		t.Fatalf("POST in read-only mode: exit %d", code)
	}
	stderr.Reset()
	if code := a.run([]string{"api", "GET", "rest/companies"}); code != 0 {
		t.Fatalf("GET in read-only mode: exit %d %s", code, stderr)
	}
}
