package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func useTempConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := userConfigDir
	userConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDir = old })
	return dir
}

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestNormalizeBaseURL(t *testing.T) {
	good := map[string]string{
		"https://crm.example.com":          "https://crm.example.com",
		"https://CRM.Example.com/":         "https://crm.example.com",
		" https://crm.example.com:8443 \n": "https://crm.example.com:8443",
		"HTTPS://api.twenty.com":           "https://api.twenty.com",
		"http://localhost:3000":            "http://localhost:3000",
		"http://127.0.0.1:3000/":           "http://127.0.0.1:3000",
		"http://[::1]:3000":                "http://[::1]:3000",
		"https://crm.example.com:":         "https://crm.example.com",
		"http://localhost:/":               "http://localhost",
		"http://[::1]:":                    "http://[::1]",
	}
	for in, want := range good {
		got, err := NormalizeBaseURL(in)
		if err != nil || got != want {
			t.Errorf("NormalizeBaseURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	bad := []string{"", "crm.example.com", "ftp://crm.example.com", "http://crm.example.com",
		"https://crm.example.com/rest", "https://crm.example.com?x=1", "https://crm.example.com?",
		"https://crm.example.com#top", "https://user:pw@crm.example.com", "https://crm.example.com:notaport", "https://"}
	for _, in := range bad {
		if got, err := NormalizeBaseURL(in); err == nil {
			t.Errorf("NormalizeBaseURL(%q) = %q, want an error", in, got)
		}
	}
}

func TestNormalizeBaseURLSuggestsOrigin(t *testing.T) {
	_, err := NormalizeBaseURL("https://crm.example.com/objects/companies?viewId=a4da5915-d16e-4ffd-a6aa-c6aed499ffeb")
	if err == nil || !strings.Contains(err.Error(), "use https://crm.example.com") {
		t.Fatalf("err = %v, want it to name the origin", err)
	}
	_, err = NormalizeBaseURL("http://crm.example.com")
	if err == nil || !strings.Contains(err.Error(), "use https://crm.example.com") {
		t.Fatalf("err = %v, want it to suggest https", err)
	}
	_, err = NormalizeBaseURL("https://me:secret@crm.example.com")
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("err = %v: must refuse and must not echo the password", err)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := useTempConfigDir(t)
	want := Config{BaseURL: "https://crm.example.com", APIKey: "a.b.c", ReadOnly: true}
	if err := Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil || got != want {
		t.Fatalf("Load = %+v, %v; want %+v", got, err, want)
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(filepath.Join(dir, "twentycrm", "config.json"))
		if err != nil || fi.Mode().Perm() != 0o600 {
			t.Fatalf("config file mode = %v, %v; want 0600", fi.Mode().Perm(), err)
		}
		di, _ := os.Stat(filepath.Join(dir, "twentycrm"))
		if di.Mode().Perm() != 0o700 {
			t.Fatalf("config dir mode = %v, want 0700", di.Mode().Perm())
		}
	}
}

func TestLoadMissingAndCorrupt(t *testing.T) {
	dir := useTempConfigDir(t)
	if c, err := Load(); err != nil || c != (Config{}) {
		t.Fatalf("missing file: %+v, %v", c, err)
	}
	os.MkdirAll(filepath.Join(dir, "twentycrm"), 0o700)
	os.WriteFile(filepath.Join(dir, "twentycrm", "config.json"), []byte("{nope"), 0o600)
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "corrupt") {
		t.Fatalf("corrupt file: err = %v", err)
	}
}

func TestResolvePrecedence(t *testing.T) {
	useTempConfigDir(t)
	Save(Config{BaseURL: "https://crm.example.com", APIKey: "file.key.x"})
	r, err := Resolve(env(nil))
	if err != nil || r.BaseURL != "https://crm.example.com" || r.BaseURLSource != "config" ||
		r.APIKey != "file.key.x" || r.KeySource != "config" || r.StoredBaseURL != "https://crm.example.com" {
		t.Fatalf("file only: %+v, %v", r, err)
	}
	r, err = Resolve(env(map[string]string{"TWENTY_BASE_URL": "https://Other.example.com/", "TWENTY_API_KEY": "env.key.x"}))
	if err != nil || r.BaseURL != "https://other.example.com" || r.BaseURLSource != "env" ||
		r.APIKey != "env.key.x" || r.KeySource != "env" || !r.FromEnv["TWENTY_API_KEY"] {
		t.Fatalf("env wins: %+v, %v", r, err)
	}
}

func TestResolveCleansEnvKey(t *testing.T) {
	useTempConfigDir(t)
	r, _ := Resolve(env(map[string]string{"TWENTY_API_KEY": "Bearer a.b.c\n"}))
	if r.APIKey != "a.b.c" {
		t.Fatalf("APIKey = %q, want the cleaned key", r.APIKey)
	}
}

func TestResolveReadOnly(t *testing.T) {
	useTempConfigDir(t)
	for v, want := range map[string]bool{"1": true, "true": true, "TRUE": true, "yes": true, "Yes": true, "on": true, "ON": true,
		" true\n": true, "0": false, "false": false, "no": false, "NO": false, "off": false, "Off": false, "": false} {
		r, err := Resolve(env(map[string]string{"TWENTY_READ_ONLY": v}))
		if err != nil || r.ReadOnly != want {
			t.Errorf("TWENTY_READ_ONLY=%q: ReadOnly = %v, %v", v, r.ReadOnly, err)
		}
	}
	Save(Config{ReadOnly: true})
	for _, v := range []string{"0", "false", "no", "off"} {
		if r, err := Resolve(env(map[string]string{"TWENTY_READ_ONLY": v})); err != nil || !r.ReadOnly {
			t.Fatalf("TWENTY_READ_ONLY=%s must not switch off read-only mode set in the file: %+v, %v", v, r, err)
		}
	}
}

func TestResolveRejectsUnknownReadOnlyValue(t *testing.T) {
	useTempConfigDir(t)
	for _, v := range []string{"y", "enabled", "2", "tru"} {
		_, err := Resolve(env(map[string]string{"TWENTY_READ_ONLY": v}))
		if err == nil || !strings.Contains(err.Error(), "TWENTY_READ_ONLY") || !strings.Contains(err.Error(), "yes") ||
			!strings.Contains(err.Error(), "off") {
			t.Errorf("TWENTY_READ_ONLY=%q: err = %v, want one naming the accepted values", v, err)
		}
	}
}

func TestResolveRejectsBadBaseURLs(t *testing.T) {
	useTempConfigDir(t)
	if _, err := Resolve(env(map[string]string{"TWENTY_BASE_URL": "crm.example.com"})); err == nil || !strings.Contains(err.Error(), "TWENTY_BASE_URL") {
		t.Fatalf("err = %v", err)
	}
	Save(Config{BaseURL: "http://crm.example.com"})
	if _, err := Resolve(env(nil)); err == nil || !strings.Contains(err.Error(), "base_url in the config file") {
		t.Fatalf("err = %v", err)
	}
}

func TestBindingMatrix(t *testing.T) {
	a, b := "https://a.example.com", "https://b.example.com"
	cases := []struct {
		name string
		r    Resolved
		ok   bool
	}{
		{"key and url from file", Resolved{BaseURL: a, BaseURLSource: "config", StoredBaseURL: a, APIKey: "k", KeySource: "config"}, true},
		{"file key, env url equal", Resolved{BaseURL: a, BaseURLSource: "env", StoredBaseURL: a, APIKey: "k", KeySource: "config"}, true},
		{"file key, env url other", Resolved{BaseURL: b, BaseURLSource: "env", StoredBaseURL: a, APIKey: "k", KeySource: "config"}, false},
		{"file key without stored url", Resolved{BaseURL: b, BaseURLSource: "env", APIKey: "k", KeySource: "config"}, false},
		{"env key, env url", Resolved{BaseURL: b, BaseURLSource: "env", StoredBaseURL: a, APIKey: "k", KeySource: "env"}, true},
		{"env key, file url", Resolved{BaseURL: a, BaseURLSource: "config", StoredBaseURL: a, APIKey: "k", KeySource: "env"}, true},
		{"no key", Resolved{BaseURL: b, BaseURLSource: "env"}, true},
	}
	for _, c := range cases {
		err := c.r.BindingError()
		if (err == nil) != c.ok {
			t.Errorf("%s: BindingError = %v", c.name, err)
		}
	}
	err := (Resolved{BaseURL: b, BaseURLSource: "env", StoredBaseURL: a, APIKey: "k", KeySource: "config"}).BindingError()
	if !strings.Contains(err.Error(), b) || !strings.Contains(err.Error(), a) || !strings.Contains(err.Error(), "TWENTY_API_KEY") {
		t.Fatalf("message must name both URLs and the fix: %v", err)
	}
}

func TestMissing(t *testing.T) {
	if m := (Resolved{}).Missing(); strings.Join(m, ",") != "base_url,api_key" {
		t.Fatalf("Missing = %v", m)
	}
	if m := (Resolved{BaseURL: "https://a.example.com", APIKey: "k"}).Missing(); len(m) != 0 {
		t.Fatalf("Missing = %v", m)
	}
}
