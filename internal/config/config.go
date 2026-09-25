// Package config resolves the CLI's settings from the config file and the
// environment. Environment variables win, with one rule on top: a key read
// from the config file is only ever sent to the base URL stored with it, so a
// tampered environment cannot redirect a stored key to another host.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/auth"
)

// userConfigDir is swapped in tests.
var userConfigDir = os.UserConfigDir

// Config is the on-disk configuration.
type Config struct {
	BaseURL  string `json:"base_url,omitempty"`
	APIKey   string `json:"api_key,omitempty"`
	ReadOnly bool   `json:"read_only,omitempty"`
}

// Path returns <os.UserConfigDir()>/twentycrm/config.json.
func Path() (string, error) {
	dir, err := userConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "twentycrm", "config.json"), nil
}

// Load reads the config file. A missing file is the zero Config; a corrupt
// file is an error.
func Load() (Config, error) {
	p, err := Path()
	if err != nil {
		return Config{}, err
	}
	raw, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(raw, &c); err != nil {
		return Config{}, fmt.Errorf("%s is corrupt, repair or delete it: %w", p, err)
	}
	return c, nil
}

// Save writes the config file with 0600 perms inside a 0700 directory,
// through a temp file and a rename, so a reader never sees a partial file.
func Save(c Config) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	// Same directory as the target: rename is atomic only within a
	// filesystem, and CreateTemp already makes the file 0600.
	tmp, err := os.CreateTemp(filepath.Dir(p), ".config-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		tmp.Close()
		return err
	}
	// fsync before the rename: rename is atomic, not durable.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), p); err != nil {
		return err
	}
	// Best effort: make the rename itself durable. Windows cannot open a
	// directory for this, which is fine.
	if d, err := os.Open(filepath.Dir(p)); err == nil {
		d.Sync()
		d.Close()
	}
	return nil
}

// IsLoopback reports whether host is one of the names over which plain HTTP
// is allowed.
func IsLoopback(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// NormalizeBaseURL reduces a base URL to scheme://host[:port] in lower case.
// Anything more (a path, a query, a fragment, user info) is refused rather
// than dropped, because it usually means a browser URL was pasted; the
// message names the origin to use instead.
func NormalizeBaseURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	u, err := url.Parse(s)
	if err != nil || u.Scheme == "" || u.Host == "" || u.Opaque != "" {
		return "", fmt.Errorf("want a base URL such as https://crm.example.com, got %q", raw)
	}
	scheme, host := strings.ToLower(u.Scheme), strings.ToLower(u.Host)
	if scheme != "https" && scheme != "http" {
		return "", fmt.Errorf("a base URL starts with https://, got %q", raw)
	}
	origin := scheme + "://" + host
	if u.User != nil {
		return "", fmt.Errorf("a base URL must not contain a user name or password: use %s", origin)
	}
	if (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", fmt.Errorf("a base URL is only the scheme and the host: use %s, not %q", origin, raw)
	}
	if scheme == "http" && !IsLoopback(strings.ToLower(u.Hostname())) {
		return "", fmt.Errorf("a base URL must use https (plain http only for localhost, 127.0.0.1 and ::1): use https://%s", host)
	}
	return origin, nil
}

// Resolved is the effective configuration after the environment overlay.
type Resolved struct {
	BaseURL       string // normalised; "" when none is configured
	BaseURLSource string // "env", "config" or ""
	APIKey        string
	KeySource     string // "env", "config" or ""
	// StoredBaseURL is base_url from the config file: the URL a stored key
	// is bound to.
	StoredBaseURL string
	ReadOnly      bool
	// FromEnv names the TWENTY_* variables that were set non-empty. Names
	// only, never values.
	FromEnv map[string]bool
}

// Resolve loads the config file and overlays the environment.
func Resolve(getenv func(string) string) (Resolved, error) {
	c, err := Load()
	if err != nil {
		return Resolved{}, err
	}
	r := Resolved{ReadOnly: c.ReadOnly}
	if c.BaseURL != "" {
		n, err := NormalizeBaseURL(c.BaseURL)
		if err != nil {
			return Resolved{}, fmt.Errorf("base_url in the config file: %w", err)
		}
		r.StoredBaseURL, r.BaseURL, r.BaseURLSource = n, n, "config"
	}
	if c.APIKey != "" {
		r.APIKey, r.KeySource = auth.CleanKey(c.APIKey), "config"
	}
	env := func(name string) string {
		v := strings.TrimSpace(getenv(name))
		if v != "" {
			if r.FromEnv == nil {
				r.FromEnv = map[string]bool{}
			}
			r.FromEnv[name] = true
		}
		return v
	}
	if v := env("TWENTY_BASE_URL"); v != "" {
		n, err := NormalizeBaseURL(v)
		if err != nil {
			return Resolved{}, fmt.Errorf("TWENTY_BASE_URL: %w", err)
		}
		r.BaseURL, r.BaseURLSource = n, "env"
	}
	if v := env("TWENTY_API_KEY"); v != "" {
		r.APIKey, r.KeySource = auth.CleanKey(v), "env"
	}
	if v := env("TWENTY_READ_ONLY"); v == "1" || strings.EqualFold(v, "true") {
		r.ReadOnly = true
	}
	return r, nil
}

// BindingError explains why the resolved key must not be sent to the
// resolved base URL, or returns nil. A key from the config file goes only to
// the base URL it was stored with.
func (r Resolved) BindingError() error {
	if r.KeySource != "config" || r.BaseURL == r.StoredBaseURL {
		return nil
	}
	if r.StoredBaseURL == "" {
		return errors.New("the API key in the config file has no base URL stored with it; run `twentycrm config set base-url <url>`, or set TWENTY_API_KEY together with TWENTY_BASE_URL")
	}
	return fmt.Errorf("TWENTY_BASE_URL points to %s, but the stored API key belongs to %s; set TWENTY_API_KEY as well, or unset TWENTY_BASE_URL",
		r.BaseURL, r.StoredBaseURL)
}

// Missing lists what the configuration lacks for an API call, as the keys
// `auth status` reports: "base_url", "api_key".
func (r Resolved) Missing() []string {
	var m []string
	if r.BaseURL == "" {
		m = append(m, "base_url")
	}
	if r.APIKey == "" {
		m = append(m, "api_key")
	}
	return m
}
