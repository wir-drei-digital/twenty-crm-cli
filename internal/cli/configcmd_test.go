package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/config"
)

// isolate points every per-user directory at a temp dir, so the tests never
// read or write the developer's real config file or cache.
func isolate(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for k, v := range map[string]string{
		"HOME": home, "USERPROFILE": home,
		"XDG_CONFIG_HOME": filepath.Join(home, "config"), "XDG_CACHE_HOME": filepath.Join(home, "cache"),
		"AppData": filepath.Join(home, "appdata"), "LocalAppData": filepath.Join(home, "localappdata"),
	} {
		t.Setenv(k, v)
	}
	return home
}

// configApp resolves the real (isolated) config file and the given
// environment, the way Execute does.
func configApp(t *testing.T, env map[string]string, stdin string) (*app, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var out, errb bytes.Buffer
	a := &app{stdout: &out, stderr: &errb, stdin: strings.NewReader(stdin)}
	a.configure(func(k string) string { return env[k] }, func(time.Duration) {})
	return a, &out, &errb
}

func mustLoad(t *testing.T) config.Config {
	t.Helper()
	c, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestConfigSetBaseURL(t *testing.T) {
	isolate(t)
	a, out, errb := configApp(t, nil, "")
	if code := a.run([]string{"config", "set", "base-url", "https://CRM.Example.com/"}); code != 0 {
		t.Fatalf("exit %d: %s", code, errb)
	}
	if out.String() != "base_url = https://crm.example.com\n" || mustLoad(t).BaseURL != "https://crm.example.com" {
		t.Fatalf("stdout %q, file %+v", out, mustLoad(t))
	}
	a, _, errb = configApp(t, nil, "")
	if code := a.run([]string{"config", "set", "base-url", "https://crm.example.com/objects/companies?viewId=x"}); code != 2 ||
		!strings.Contains(errLine(t, errb.String()).Message, "use https://crm.example.com") {
		t.Fatalf("browser URL: exit %d %s", code, errb)
	}
}

func TestConfigSetAPIKey(t *testing.T) {
	isolate(t)
	key := testKey(t, "ws-test", time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC))
	a, _, _ := configApp(t, nil, "")
	a.run([]string{"config", "set", "base-url", "https://crm.example.com"})
	a, out, errb := configApp(t, nil, key+"\n")
	if code := a.run([]string{"config", "set", "api-key"}); code != 0 {
		t.Fatalf("exit %d: %s", code, errb)
	}
	if mustLoad(t).APIKey != key || !strings.Contains(out.String(), "workspace ws-test") ||
		!strings.Contains(out.String(), "key id key-1") || !strings.Contains(out.String(), "2027-01-02") || strings.Contains(out.String(), key) {
		t.Fatalf("stdout %q, file %+v", out, mustLoad(t))
	}
}

func TestConfigSetAPIKeyStripsBearer(t *testing.T) {
	isolate(t)
	key := testKey(t, "ws-test", time.Time{})
	a, _, _ := configApp(t, nil, "")
	a.run([]string{"config", "set", "base-url", "https://crm.example.com"})
	a, out, errb := configApp(t, nil, "Bearer "+key+"\r\n")
	if code := a.run([]string{"config", "set", "api-key"}); code != 0 || mustLoad(t).APIKey != key || !strings.Contains(out.String(), "never expires") {
		t.Fatalf("exit %d: %s %s, stored %q", code, out, errb, mustLoad(t).APIKey)
	}
}

func TestConfigSetAPIKeyRefusals(t *testing.T) {
	isolate(t)
	key := testKey(t, "ws-test", time.Time{})
	a, _, errb := configApp(t, nil, key)
	if code := a.run([]string{"config", "set", "api-key"}); code != 2 || !strings.Contains(errLine(t, errb.String()).Message, "base-url") {
		t.Fatalf("no base URL: exit %d %s", code, errb)
	}
	a, _, _ = configApp(t, nil, "")
	a.run([]string{"config", "set", "base-url", "https://crm.example.com"})
	for stdin, want := range map[string]string{
		"":          "no key on stdin",
		"not-a-jwt": "three base64url parts",
		testToken(t, map[string]any{"type": "ACCESS", "workspaceId": "ws-test"}): "ACCESS token, not an API key",
	} {
		a, _, errb := configApp(t, nil, stdin)
		code := a.run([]string{"config", "set", "api-key"})
		if code != 2 || !strings.Contains(errLine(t, errb.String()).Message, want) {
			t.Errorf("stdin %.20q: exit %d %s", stdin, code, errb)
		}
	}
	if mustLoad(t).APIKey != "" {
		t.Fatal("a refused key was stored")
	}
}

func TestConfigBindingGuards(t *testing.T) {
	isolate(t)
	config.Save(config.Config{BaseURL: "https://a.example.com", APIKey: testKey(t, "ws-test", time.Time{})})
	a, _, errb := configApp(t, nil, "")
	if code := a.run([]string{"config", "set", "base-url", "https://b.example.com"}); code != 2 ||
		!strings.Contains(errLine(t, errb.String()).Message, "config unset api-key") {
		t.Fatalf("other host: exit %d %s", code, errb)
	}
	a, _, errb = configApp(t, nil, "")
	if code := a.run([]string{"config", "unset", "base-url"}); code != 2 {
		t.Fatalf("unset base-url with a key: exit %d %s", code, errb)
	}
	a, _, errb = configApp(t, nil, "")
	if code := a.run([]string{"config", "set", "base-url", "https://A.example.com"}); code != 0 {
		t.Fatalf("same host: exit %d %s", code, errb)
	}
	a, out, _ := configApp(t, nil, "")
	if code := a.run([]string{"config", "unset", "api-key"}); code != 0 || !strings.Contains(out.String(), "stays valid until") || mustLoad(t).APIKey != "" {
		t.Fatalf("unset api-key: exit %d %s", code, out)
	}
}

func TestConfigReadOnlyPathAndSuggestions(t *testing.T) {
	home := isolate(t)
	a, out, _ := configApp(t, nil, "")
	if code := a.run([]string{"config", "set", "read-only", "true"}); code != 0 || !mustLoad(t).ReadOnly || out.String() != "read_only = true\n" {
		t.Fatalf("read-only: exit %d %s", code, out)
	}
	a, _, _ = configApp(t, nil, "")
	if code := a.run([]string{"config", "set", "read-only", "maybe"}); code != 2 {
		t.Fatalf("read-only maybe: exit %d", code)
	}
	a, out, _ = configApp(t, nil, "")
	if code := a.run([]string{"config", "path"}); code != 0 || !strings.HasPrefix(strings.TrimSpace(out.String()), home) {
		t.Fatalf("path: exit %d %s", code, out)
	}
	a, _, errb := configApp(t, nil, "")
	if code := a.run([]string{"config", "set", "TWENTY_API_KEY"}); code != 2 ||
		!strings.Contains(errLine(t, errb.String()).Message, "twentycrm config set api-key") {
		t.Fatalf("env name: exit %d %s", code, errb)
	}
}

// An unknown key names the valid ones, not just "unknown command" (spec:
// Authentication and configuration).
func TestConfigUnknownKeyNamesTheKeys(t *testing.T) {
	isolate(t)
	for _, verb := range []string{"set", "unset"} {
		a, _, errb := configApp(t, nil, "")
		if code := a.run([]string{"config", verb, "colour"}); code != 2 ||
			!strings.Contains(errLine(t, errb.String()).Message, "base-url, api-key and read-only") {
			t.Errorf("config %s colour: exit %d %s", verb, code, errb)
		}
	}
}

func TestConfigNotesEnvOverride(t *testing.T) {
	isolate(t)
	a, _, errb := configApp(t, map[string]string{"TWENTY_BASE_URL": "https://env.example.com"}, "")
	if code := a.run([]string{"config", "set", "base-url", "https://crm.example.com"}); code != 0 ||
		!strings.Contains(errb.String(), "TWENTY_BASE_URL is set in this environment") {
		t.Fatalf("exit %d: %s", code, errb)
	}
}

func TestCorruptConfigStillAllowsRepair(t *testing.T) {
	isolate(t)
	p, _ := config.Path()
	os.MkdirAll(filepath.Dir(p), 0o700)
	os.WriteFile(p, []byte("{nope"), 0o600)
	a, out, _ := configApp(t, nil, "")
	if code := a.run([]string{"config", "path"}); code != 0 || strings.TrimSpace(out.String()) != p {
		t.Fatalf("config path: exit %d %s", code, out)
	}
	a, _, errb := configApp(t, nil, "")
	if code := a.run([]string{"companies", "list"}); code != 2 || !strings.Contains(errLine(t, errb.String()).Message, "cannot load the configuration") {
		t.Fatalf("companies list: exit %d %s", code, errb)
	}
}
