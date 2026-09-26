package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/config"
)

func TestObjectVerbRequests(t *testing.T) {
	cases := []struct {
		args         []string
		method, path string
		query        map[string]string
		body         string
	}{
		{[]string{"companies", "list", "--filter", `name[ilike]:"%acme%"`, "--order-by", "name", "--limit", "5", "--depth", "1"},
			"GET", "/rest/companies", map[string]string{"filter": `name[ilike]:"%acme%"`, "order_by": "name", "limit": "5", "depth": "1"}, ""},
		{[]string{"companies", "list", "--starting-after", "c1"}, "GET", "/rest/companies", map[string]string{"starting_after": "c1"}, ""},
		{[]string{"companies", "get", testID}, "GET", "/rest/companies/" + testID, nil, ""},
		{[]string{"companies", "group-by", "--group-by", `[{"stage":true}]`, "--aggregate", `["countNotEmptyId"]`, "--include-records-sample"},
			"GET", "/rest/companies/groupBy", map[string]string{"group_by": `[{"stage":true}]`, "aggregate": `["countNotEmptyId"]`, "include_records_sample": "true"}, ""},
		{[]string{"companies", "find-duplicates", "--data", `{"ids":["` + testID + `"]}`}, "POST", "/rest/companies/duplicates", nil, `{"ids":["` + testID + `"]}`},
		{[]string{"companies", "create", "--data", `{"name":"Acme"}`, "--upsert"}, "POST", "/rest/companies", map[string]string{"upsert": "true"}, `{"name":"Acme"}`},
		{[]string{"companies", "batch-create", "--data", `[{"name":"A"},{"name":"B"}]`}, "POST", "/rest/batch/companies", nil, `[{"name":"A"},{"name":"B"}]`},
		{[]string{"companies", "update", testID, "--data", `{"name":"B"}`}, "PATCH", "/rest/companies/" + testID, nil, `{"name":"B"}`},
		{[]string{"companies", "delete", testID}, "DELETE", "/rest/companies/" + testID, map[string]string{"soft_delete": "true"}, ""},
		{[]string{"companies", "restore", testID}, "PATCH", "/rest/restore/companies/" + testID, nil, ""},
		{[]string{"companies", "update-many", "--filter", "city[eq]:Bern", "--data", `{"tagline":"x"}`, "--force"},
			"PATCH", "/rest/companies", map[string]string{"filter": "city[eq]:Bern"}, `{"tagline":"x"}`},
		{[]string{"companies", "delete-many", "--filter", "city[eq]:Bern", "--force"},
			"DELETE", "/rest/companies", map[string]string{"filter": "city[eq]:Bern", "soft_delete": "true"}, ""},
		{[]string{"companies", "restore-many", "--filter", "city[eq]:Bern", "--force"}, "PATCH", "/rest/restore/companies", map[string]string{"filter": "city[eq]:Bern"}, ""},
		{[]string{"companies", "merge", "--data", `{"ids":["a","b"],"conflictPriorityIndex":0}`, "--force"},
			"PATCH", "/rest/companies/merge", nil, `{"ids":["a","b"],"conflictPriorityIndex":0}`},
		{[]string{"companies", "destroy", testID, "--force"}, "DELETE", "/rest/companies/" + testID, map[string]string{"soft_delete": "false"}, ""},
		{[]string{"companies", "destroy-many", "--filter", "city[eq]:Bern", "--force"},
			"DELETE", "/rest/companies", map[string]string{"filter": "city[eq]:Bern", "soft_delete": "false"}, ""},
		{[]string{"note-targets", "list"}, "GET", "/rest/noteTargets", nil, ""},
		{[]string{"noteTargets", "list"}, "GET", "/rest/noteTargets", nil, ""},
		{[]string{"company", "get", testID}, "GET", "/rest/companies/" + testID, nil, ""},
		{[]string{"invoices", "create", "--data", "-"}, "POST", "/rest/invoices", nil, `{"name":"INV-1","status":"DRAFT"}`},
	}
	for _, c := range cases {
		srv := newFakeTwenty(t)
		a, _, stderr := newTestApp(t, srv, nil)
		a.stdin = strings.NewReader(`{"name":"INV-1","status":"DRAFT"}`)
		if code := a.run(c.args); code != 0 {
			t.Errorf("%v: exit %d, stderr %s", c.args, code, stderr)
			continue
		}
		calls := srv.calls()
		if len(calls) != 1 {
			t.Errorf("%v: %d calls", c.args, len(calls))
			continue
		}
		got := calls[0]
		if got.Method != c.method || got.Path != c.path || got.Body != c.body {
			t.Errorf("%v: sent %s %s %s", c.args, got.Method, got.Path, got.Body)
		}
		if len(got.Query) != len(c.query) {
			t.Errorf("%v: query %v, want %v", c.args, got.Query, c.query)
		}
		for k, v := range c.query {
			if got.Query.Get(k) != v || len(got.Query[k]) != 1 {
				t.Errorf("%v: query %s = %v, want %q", c.args, k, got.Query[k], v)
			}
		}
	}
}

func TestMergeDryRunIsReadClass(t *testing.T) {
	srv := newFakeTwenty(t)
	a, _, stderr := newTestApp(t, srv, func(r *config.Resolved) { r.ReadOnly = true })
	a.client.ReadOnly = true
	if code := a.run([]string{"companies", "merge", "--data", `{"ids":["a","b"],"dryRun":false}`, "--dry-run"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var body map[string]any
	json.Unmarshal([]byte(srv.calls()[0].Body), &body)
	if body["dryRun"] != true || len(body["ids"].([]any)) != 2 {
		t.Fatalf("body = %v", body)
	}
}

func TestGatesRefuseBeforeNetwork(t *testing.T) {
	parts := make([]string, 61)
	for i := range parts {
		parts[i] = fmt.Sprintf(`{"name":"C%d"}`, i)
	}
	sixtyOne := "[" + strings.Join(parts, ",") + "]"
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"update-many without filter", []string{"companies", "update-many", "--data", `{}`, "--force"}, "needs --filter"},
		{"blank filter", []string{"companies", "delete-many", "--filter", " ", "--force"}, "needs --filter"},
		{"bulk without force", []string{"companies", "update-many", "--filter", "a[eq]:1", "--data", `{}`}, "needs --force"},
		{"destroy without force", []string{"companies", "destroy", testID}, "needs --force"},
		{"destroy-many without force", []string{"companies", "destroy-many", "--filter", "a[eq]:1"}, "needs --force"},
		{"merge without force", []string{"companies", "merge", "--data", `{"ids":["a","b"]}`}, "needs --force"},
		{"batch too large", []string{"companies", "batch-create", "--data", sixtyOne}, "at most 60"},
		{"bad id", []string{"companies", "get", "42"}, "not a record ID"},
		{"path in id", []string{"companies", "delete", "../people/" + testID}, "not a record ID"},
		{"depth 2", []string{"companies", "list", "--depth", "2"}, "--depth takes 0 or 1"},
		{"limit 0", []string{"companies", "list", "--limit", "0"}, "--limit takes 1 to 200"},
		{"limit 201", []string{"companies", "list", "--limit", "201"}, "--limit takes 1 to 200"},
		{"create without data", []string{"companies", "create"}, "--data is required"},
		{"create with array", []string{"companies", "create", "--data", `[]`}, "JSON object"},
		{"invalid json", []string{"companies", "create", "--data", `{nope`}, "not valid JSON"},
		{"group-by without group-by", []string{"companies", "group-by"}, "--group-by is required"},
		{"metadata create without force", []string{"metadata", "fields", "create", "--data", `{}`}, "needs --force"},
		{"api key blocked", []string{"metadata", "api-keys", "delete", testID, "--force"}, "blocked"},
	}
	for _, c := range cases {
		srv := newFakeTwenty(t)
		a, _, stderr := newTestApp(t, srv, nil)
		code := a.run(c.args)
		if code != 2 {
			t.Errorf("%s: exit %d, want 2", c.name, code)
			continue
		}
		if e := errLine(t, stderr.String()); e.Kind != "usage" || !strings.Contains(e.Message, c.want) {
			t.Errorf("%s: %+v, want usage containing %q", c.name, e, c.want)
		}
		if n := len(srv.calls()); n != 0 {
			t.Errorf("%s: %d calls reached the server", c.name, n)
		}
	}
}

func TestReadOnlyMode(t *testing.T) {
	srv := newFakeTwenty(t)
	a, _, stderr := newTestApp(t, srv, func(r *config.Resolved) { r.ReadOnly = true })
	if code := a.run([]string{"companies", "create", "--data", `{"name":"A"}`}); code != 2 ||
		!strings.Contains(errLine(t, stderr.String()).Message, "read-only mode") {
		t.Fatalf("create in read-only mode: exit %d %s", code, stderr)
	}
	stderr.Reset()
	if code := a.run([]string{"companies", "list"}); code != 0 {
		t.Fatalf("list in read-only mode: exit %d %s", code, stderr)
	}
}

func TestFilterReachesTwentyUnchanged(t *testing.T) {
	srv := newFakeTwenty(t)
	a, _, stderr := newTestApp(t, srv, nil)
	filter := `name[ilike]:"%Zürich, AG%"`
	if code := a.run([]string{"companies", "list", "--filter", filter}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	c := srv.calls()[0]
	if c.Query.Get("filter") != filter {
		t.Fatalf("filter arrived as %q", c.Query.Get("filter"))
	}
	if !strings.Contains(c.RawQuery, "%25Z") || strings.Contains(c.RawQuery, "%2525") {
		t.Fatalf("%% must be encoded exactly once: %s", c.RawQuery)
	}
}

func TestNoKeyAndBinding(t *testing.T) {
	srv := newFakeTwenty(t)
	a, _, stderr := newTestApp(t, srv, func(r *config.Resolved) { r.APIKey = "" })
	a.client.APIKey = ""
	if code := a.run([]string{"companies", "list"}); code != 2 || !strings.Contains(errLine(t, stderr.String()).Message, "no API key") {
		t.Fatalf("no key: exit %d %s", code, stderr)
	}
	srv2 := newFakeTwenty(t)
	b, _, stderr2 := newTestApp(t, srv2, func(r *config.Resolved) {
		r.KeySource, r.StoredBaseURL = "config", "https://other.example.com"
	})
	if code := b.run([]string{"companies", "list"}); code != 2 || !strings.Contains(errLine(t, stderr2.String()).Message, "belongs to https://other.example.com") {
		t.Fatalf("binding: exit %d %s", code, stderr2)
	}
	if len(srv2.reqs) != 0 {
		t.Fatalf("a bound key must not reach another host: %d requests", len(srv2.reqs))
	}
}

func TestUnknownCommandAndRootWithoutArgs(t *testing.T) {
	srv := newFakeTwenty(t)
	a, _, stderr := newTestApp(t, srv, nil)
	if code := a.run([]string{"compnies", "list"}); code != 2 || errLine(t, stderr.String()).Kind != "usage" {
		t.Fatalf("typo: exit %d %s", code, stderr)
	}
	stderr.Reset()
	// An empty, non-nil slice: with nil, cobra would read the test binary's own os.Args.
	if code := a.run([]string{}); code != 2 || !strings.Contains(errLine(t, stderr.String()).Message, "needs a subcommand") {
		t.Fatalf("no args: exit %d %s", code, stderr)
	}
}

func TestOutputFile(t *testing.T) {
	srv := newFakeTwenty(t)
	a, stdout, stderr := newTestApp(t, srv, nil)
	p := filepath.Join(t.TempDir(), "out.json")
	if code := a.run([]string{"companies", "get", testID, "--output", p}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	raw, _ := os.ReadFile(p)
	if string(raw) != `{"data":{}}` || stdout.Len() != 0 {
		t.Fatalf("file %q, stdout %q", raw, stdout)
	}
}

func TestAliasCollisionKeepsTheObjectsOwnCommand(t *testing.T) {
	srv := newFakeTwenty(t)
	a, _, stderr := newTestApp(t, srv, nil)
	// A custom object whose plural is "company" takes the name away from the
	// singular alias of companies.
	srv.openAPI = []byte(strings.Replace(string(srv.openAPI), `"/invoices": {`,
		`"/company": {"get": {"operationId": "findManyCompany"}, "post": {"operationId": "createOneCompanyItem"}}, "/invoices": {`, 1))
	srv.openAPI = []byte(strings.Replace(string(srv.openAPI), `"Invoice": {`,
		`"CompanyItem": {"type": "object", "properties": {"name": {"type": "string"}}}, "CompanyItemForResponse": {"type": "object", "properties": {"id": {"type": "string"}, "name": {"type": "string"}}}, "Invoice": {`, 1))
	if code := a.run([]string{"company", "list"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if got := srv.calls()[0].Path; got != "/rest/company" {
		t.Fatalf("company list went to %s, want the object named company", got)
	}
}
