package cli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/config"
)

var fixedNow = time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

func authStatusOf(t *testing.T, edit func(*config.Resolved)) (map[string]any, string) {
	t.Helper()
	srv := newFakeTwenty(t)
	a, stdout, stderr := newTestApp(t, srv, edit)
	a.now = func() time.Time { return fixedNow }
	if code := a.run([]string{"auth", "status"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if strings.Count(stdout.String(), "\n") != 1 || len(srv.reqs) != 0 {
		t.Fatalf("auth status must be one offline line: %q, %d requests", stdout, len(srv.reqs))
	}
	var m map[string]any
	json.Unmarshal(stdout.Bytes(), &m)
	return m, stdout.String()
}

func TestAuthStatus(t *testing.T) {
	key := testKey(t, "ws-test", fixedNow.Add(30*24*time.Hour))
	m, raw := authStatusOf(t, func(r *config.Resolved) { r.APIKey = key })
	if m["mode"] != "api_key" || m["source"] != "env" || m["workspace_id"] != "ws-test" || m["key_id"] != "key-1" ||
		m["expires_in_days"] != float64(30) || m["expires_soon"] != false || m["expired"] != false || m["read_only"] != false {
		t.Fatalf("status = %v", m)
	}
	if strings.Contains(raw, key) {
		t.Fatal("auth status printed the key")
	}
	m, _ = authStatusOf(t, func(r *config.Resolved) { r.APIKey = testKey(t, "ws-test", fixedNow.Add(10*24*time.Hour)) })
	if m["expires_soon"] != true {
		t.Fatalf("10 days left: %v", m)
	}
	m, _ = authStatusOf(t, func(r *config.Resolved) { r.APIKey = testKey(t, "ws-test", fixedNow.Add(-time.Hour)) })
	if m["expired"] != true || !strings.Contains(m["hint"].(string), "expired") {
		t.Fatalf("expired: %v", m)
	}
	m, _ = authStatusOf(t, func(r *config.Resolved) { r.APIKey = testKey(t, "ws-test", time.Time{}) })
	if _, ok := m["expires_at"]; ok || m["expires_soon"] != false {
		t.Fatalf("never expires: %v", m)
	}
}

func TestAuthStatusProblems(t *testing.T) {
	m, _ := authStatusOf(t, func(r *config.Resolved) { *r = config.Resolved{} })
	if m["mode"] != "none" || !strings.Contains(m["hint"].(string), "twentycrm init") {
		t.Fatalf("nothing configured: %v", m)
	}
	if missing, _ := m["missing"].([]any); len(missing) != 2 {
		t.Fatalf("missing = %v", m["missing"])
	}
	m, _ = authStatusOf(t, func(r *config.Resolved) { r.APIKey = "abc" })
	if !strings.Contains(m["key_error"].(string), "three base64url parts") {
		t.Fatalf("unreadable key: %v", m)
	}
	m, _ = authStatusOf(t, func(r *config.Resolved) { r.KeySource, r.StoredBaseURL = "config", "https://other.example.com" })
	if !strings.Contains(m["hint"].(string), "belongs to https://other.example.com") {
		t.Fatalf("binding: %v", m)
	}
}
