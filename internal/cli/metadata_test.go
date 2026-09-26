package cli

import (
	"strings"
	"testing"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/config"
)

func TestMetadataRequests(t *testing.T) {
	cases := []struct {
		args         []string
		method, path string
		body         string
	}{
		{[]string{"metadata", "objects", "list", "--limit", "10"}, "GET", "/rest/metadata/objects", ""},
		{[]string{"metadata", "view-fields", "get", testID}, "GET", "/rest/metadata/viewFields/" + testID, ""},
		{[]string{"metadata", "fields", "create", "--data", `{"name":"x"}`, "--force"}, "POST", "/rest/metadata/fields", `{"name":"x"}`},
		{[]string{"metadata", "views", "update", testID, "--data", `{"name":"y"}`, "--force"}, "PATCH", "/rest/metadata/views/" + testID, `{"name":"y"}`},
		{[]string{"metadata", "webhooks", "delete", testID, "--force"}, "DELETE", "/rest/metadata/webhooks/" + testID, ""},
		{[]string{"metadata", "api-keys", "list"}, "GET", "/rest/metadata/apiKeys", ""},
	}
	for _, c := range cases {
		srv := newFakeTwenty(t)
		a, _, stderr := newTestApp(t, srv, nil)
		if code := a.run(c.args); code != 0 {
			t.Errorf("%v: exit %d %s", c.args, code, stderr)
			continue
		}
		got := srv.calls()[0]
		if got.Method != c.method || got.Path != c.path || got.Body != c.body || got.Query.Has("soft_delete") {
			t.Errorf("%v: sent %s %s?%s %s", c.args, got.Method, got.Path, got.RawQuery, got.Body)
		}
	}
}

func TestMetadataGates(t *testing.T) {
	srv := newFakeTwenty(t)
	a, _, stderr := newTestApp(t, srv, func(r *config.Resolved) { r.ReadOnly = true })
	if code := a.run([]string{"metadata", "views", "update", testID, "--data", `{}`, "--force"}); code != 2 ||
		!strings.Contains(errLine(t, stderr.String()).Message, "read-only") {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	stderr.Reset()
	if code := a.run([]string{"metadata", "api-keys", "create", "--data", `{}`, "--force"}); code != 2 ||
		!strings.Contains(errLine(t, stderr.String()).Message, "blocked") {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	root := a.newRoot()
	cmd, _, _ := root.Find([]string{"metadata", "api-keys", "delete"})
	if !strings.HasPrefix(cmd.Short, "[blocked]") {
		t.Fatalf("short = %q", cmd.Short)
	}
}
