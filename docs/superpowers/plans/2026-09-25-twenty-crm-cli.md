# twentycrm (Twenty CRM CLI) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `twentycrm`, a single static Go binary that turns every object of a Twenty CRM workspace into commands (`twentycrm companies list`), authenticates with a Twenty API key bound to its base URL, and refuses permanent deletes, bulk changes and data-model changes without an explicit `--force`.

**Architecture:** There is no vendored spec and no generator. A fixed verb table (`internal/routes`) describes Twenty's REST grammar, which is identical for every object. At runtime the CLI reads the workspace's objects and fields from `GET /rest/open-api/core` (`internal/model`), caches them, and builds the cobra tree from the cache. `internal/api` sends the requests with the key from `internal/config`; the routes package decides each call's risk class and local checks before anything is sent.

**Tech Stack:** Go 1.26, `github.com/spf13/cobra` v1.10.2, `golang.org/x/term` v0.45.0, goreleaser, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-09-25-twenty-crm-cli-design.md`. Read it before your task; this plan argues from it.

**Reference implementation:** `/Users/daniel/Development/google-ads-cli`, a sibling CLI by the same author with the same contracts. Where a step says "copy X from google-ads-cli", copy the file and make exactly the listed changes. Never import anything from google-ads-cli: the repositories share no module.

**Twenty source for reference:** a sparse checkout of Twenty at tag `twenty/v2.27.0` is at `/private/tmp/claude-501/-Users-daniel-Development-workspace/546ce888-1f6b-5556-a15f-79cd036ea3f1/scratchpad/twenty-src`; the same files are on GitHub under `https://github.com/twentyhq/twenty/tree/twenty/v2.27.0/packages/twenty-server/src/engine`. Only read it; nothing in this repository depends on it.

## Global Constraints

- Repo root: `/Users/daniel/Development/twenty-crm-cli`, branch `main`. Run every command from there.
- Module `github.com/wir-drei-digital/twenty-crm-cli`; binary `twentycrm`; `go 1.26` in `go.mod`; must build with `CGO_ENABLED=0`.
- Dependencies: only `github.com/spf13/cobra v1.10.2` and `golang.org/x/term v0.45.0` and their indirect requirements. Add them with `go get <module>@<version>` when a task first needs them, then `go mod tidy`.
- Environment variables: exactly `TWENTY_BASE_URL`, `TWENTY_API_KEY`, `TWENTY_READ_ONLY`. No others.
- Base URLs are `scheme://host[:port]`, lower case, no path; HTTPS required except on the loopback hosts `localhost`, `127.0.0.1`, `::1`.
- A key from the config file is only ever sent to the base URL stored with it (the binding rule, spec *Authentication and configuration*).
- stdout carries only the API response (or a config/auth/schema/commands command's own result); every failure is exactly one line of JSON on stderr with keys `kind`, `error`, `status` (omitted when there is no response) and `details`. Exit codes: `0` success, `1` API or network error, `2` usage error, which includes every guardrail refusal.
- Secrets never come from argv. The API key comes from stdin (`config set api-key`), `TWENTY_API_KEY`, or the hidden prompt of `init`.
- Risk classes are exactly `read`, `write`, `bulk`, `destroy`, `admin`; `bulk`, `destroy` and `admin` need `--force`; `metadata api-keys create/update/delete` are blocked.
- `delete` always sends `soft_delete=true`; only `destroy` and `destroy-many` send `soft_delete=false`.
- `go vet ./...` clean; `go test ./...` passes on Linux, macOS and Windows without network access. The only exception is `e2e/live_test.go`, which skips unless `TWENTY_LIVE=1`.
- All code, identifiers, comments and docs in English. Prose in `README.md`, `SECURITY.md`, `docs/agents.md` and in error messages contains no em dash character; use a colon, a semicolon, a comma or a new sentence.
- Commit at the end of every task. Every commit message ends with a blank line followed by `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Review Focus

These inputs are implied by the spec but not spelled out there. Each has a test in the task that owns the code.

1. **A browser URL pasted as the base URL** (`https://crm.example.com/objects/companies?viewId=...`, exactly what a person copies from the address bar): refused, and the message names the origin to use instead (`use https://crm.example.com`). Test: Task 1 (`TestNormalizeBaseURLSuggestsOrigin`).
2. **An API key pasted with a `Bearer ` prefix or a trailing newline** (copied from a curl example or `pbpaste`): cleaned before it is parsed, stored or sent, never sent as `Bearer Bearer ...`. Tests: Task 1 (`TestCleanKey`, `TestResolveCleansEnvKey`), Task 4 (`TestConfigSetAPIKeyStripsBearer`).
3. **A filter with spaces, quotes, commas, `%` wildcards and umlauts** (`name[ilike]:"%Zürich, AG%"`): reaches Twenty byte for byte, encoded once. Test: Task 3 (`TestFilterReachesTwentyUnchanged`).
4. **An object typed the way the API spells it** (`noteTargets`) or in the singular (`company`): works as an alias of the command (`note-targets`, `companies`) unless the alias would collide with a built-in or another object. Tests: Task 2 (`TestModelFindMatchesAliases`), Task 3 (rows `noteTargets list` and `company get` in `TestObjectVerbRequests`).
5. **`--all` on a response with `hasNextPage: true` but no `endCursor`**: stops with kind `server` and prints the rows collected so far, instead of fetching the first page again until `--max-pages` and printing duplicates. Test: Task 3 (`TestAllStopsWithoutCursor`).

## File Structure

```
twenty-crm-cli/
├── go.mod, go.sum, LICENSE, Makefile, .gitignore, .gitattributes, .goreleaser.yaml
├── .github/workflows/ci.yml, .github/workflows/release.yml
├── README.md, SECURITY.md, docs/agents.md
├── cmd/twentycrm/main.go                 # hands argv to internal/cli
├── internal/auth/auth.go                 # CleanKey, Decode, ParseAPIKey (JWT payload, no verification)
├── internal/config/config.go             # config file, NormalizeBaseURL, Resolve, BindingError, Missing
├── internal/routes/routes.go             # classes, verb tables, ValidateID, CheckBody, Decision
├── internal/routes/raw.go                # CleanRawPath, ClassifyRaw for the api escape hatch
├── internal/api/errors.go                # Error, kinds, Usagef, kindForStatus
├── internal/api/client.go                # Client.Do: base URL guard, headers, retries
├── internal/api/twenty.go                # Twenty error bodies, messages, hints
├── internal/api/retry.go                 # classifyTransport, retryAfter, jitter
├── internal/model/model.go               # Object, Field, Model, CommandName, Find, Stale
├── internal/model/extract.go             # Extract: OpenAPI document -> objects
├── internal/model/cache.go               # CachePath, LoadCache, SaveCache
├── internal/model/testdata/openapi-core.json, openapi-skeleton.json
├── internal/cli/app.go                   # Execute, app, configure, renderError, requireConfig
├── internal/cli/refresh.go               # firstWord, builtins, prepareModel, fetchModel
├── internal/cli/root.go, version.go, util.go
├── internal/cli/objects.go               # object commands from the model, runObjectVerb, send
├── internal/cli/metadata.go              # metadata commands
├── internal/cli/all.go                   # --all cursor walk
├── internal/cli/help.go                  # per-object help: field table, filter syntax
├── internal/cli/body.go, output.go, prompt.go   # copied from google-ads-cli
├── internal/cli/catalog.go, schemacmd.go, apicmd.go
├── internal/cli/configcmd.go, authcmd.go, initcmd.go
└── e2e/helpers_test.go, e2e/smoke_test.go, e2e/live_test.go
```

---

### Task 1: Scaffold, config, API key parsing, routes

The pure foundations: no network, no cobra. Everything here is table-tested.

**Files:**
- Create: `go.mod`, `LICENSE`, `Makefile`, `.gitignore`, `.gitattributes`
- Create: `internal/auth/auth.go`, `internal/auth/auth_test.go`
- Create: `internal/config/config.go`, `internal/config/config_test.go`
- Create: `internal/routes/routes.go`, `internal/routes/raw.go`, `internal/routes/routes_test.go`, `internal/routes/raw_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `auth.CleanKey(raw string) string`
  - `auth.Key{Type, WorkspaceID, KeyID string; ExpiresAt time.Time}`, `(*Key).Expired(now time.Time) bool`
  - `auth.Decode(raw string) (*Key, error)` (lenient: any JWT), `auth.ParseAPIKey(raw string) (*Key, error)` (strict: `type` `API_KEY` and a workspace)
  - `config.Config{BaseURL, APIKey string; ReadOnly bool}` with JSON keys `base_url`, `api_key`, `read_only`
  - `config.Path() (string, error)`, `config.Load() (Config, error)`, `config.Save(Config) error`
  - `config.IsLoopback(host string) bool`, `config.NormalizeBaseURL(raw string) (string, error)`
  - `config.Resolved{BaseURL, BaseURLSource, APIKey, KeySource, StoredBaseURL string; ReadOnly bool; FromEnv map[string]bool}`, `config.Resolve(getenv func(string) string) (Resolved, error)`, `(Resolved).BindingError() error`, `(Resolved).Missing() []string`
  - `routes.ClassRead|ClassWrite|ClassBulk|ClassDestroy|ClassAdmin` (strings), `routes.NeedsForce(class string) bool`
  - `routes.BodyKind` with `BodyNone`, `BodyObject`, `BodyArray`, `BodyMerge`, `BodyDuplicates`; `routes.MaxBatch = 60`
  - `routes.Verb{Name, Method, PathTemplate, Class string; TakesID, FilterRequired bool; Body BodyKind; SoftDelete string; Flags []string; RequiredFlag string; MaxLimit int; Summary string}`, `(Verb).Path(plural, id string) string`
  - `routes.ObjectVerbs []Verb`, `routes.MetadataVerbs []Verb`, `routes.FindVerb(verbs []Verb, name string) (Verb, bool)`
  - `routes.MetaKind{Command, Segment string}`, `routes.MetadataKinds []MetaKind`, `routes.MetadataBlocked(kindCommand, verb string) string`, `routes.APIKeysBlocked` (string)
  - `routes.ValidateID(id string) error`, `routes.CheckBody(kind BodyKind, body []byte) error`
  - `routes.Decision{Command, Class, Blocked string; FilterRequired bool; Filter string; ReadOnly, Force bool}`, `(Decision).Check() error`
  - `routes.CleanRawPath(p string) (string, error)`, `routes.Raw{Class, Blocked string; FilterRequired bool}`, `routes.ClassifyRaw(method, path string, q url.Values) (Raw, error)`

- [ ] **Step 1: Scaffold the repository**

The repository already exists on `main` with the spec committed. Create:

`go.mod`:

```
module github.com/wir-drei-digital/twenty-crm-cli

go 1.26
```

`LICENSE`: copy `/Users/daniel/Development/google-ads-cli/LICENSE` unchanged (MIT, "Copyright (c) 2026 wir drei digital").

`Makefile`:

```make
.PHONY: build test check

build:
	go build -o bin/twentycrm ./cmd/twentycrm

test:
	go test ./...

check:
	go vet ./...
	go test ./...
```

`.gitignore`:

```
bin/
dist/
*.test
coverage.out

# `go build ./cmd/twentycrm` drops the binary here; never commit it.
/twentycrm
/twentycrm.exe

# Superpowers working artifacts: process scratch, not repo content.
.superpowers/
```

`.gitattributes`:

```
# Text files are LF in the repository and on checkout, on every platform.
* text=auto eol=lf
```

- [ ] **Step 2: Write the failing auth tests**

`internal/auth/auth_test.go`:

```go
package auth

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// jwt builds an unsigned token with the given payload; the CLI never
// verifies signatures, so the third part only has to be present.
func jwt(t *testing.T, payload map[string]any) string {
	t.Helper()
	enc := func(v any) string {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	return enc(map[string]string{"alg": "HS256", "typ": "JWT"}) + "." + enc(payload) + ".c2ln"
}

func TestCleanKey(t *testing.T) {
	for in, want := range map[string]string{
		"abc":              "abc",
		"  abc \n":         "abc",
		"Bearer abc":       "abc",
		"bearer   abc\r\n": "abc",
		"BEARER abc":       "abc",
		"Bearerabc":        "Bearerabc",
	} {
		if got := CleanKey(in); got != want {
			t.Errorf("CleanKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseAPIKey(t *testing.T) {
	exp := time.Date(2027, 3, 1, 12, 0, 0, 0, time.UTC)
	raw := jwt(t, map[string]any{"sub": "ws-1", "type": "API_KEY", "workspaceId": "ws-1", "jti": "key-9", "exp": exp.Unix(), "iat": exp.Add(-time.Hour).Unix()})
	k, err := ParseAPIKey("Bearer " + raw + "\n")
	if err != nil {
		t.Fatal(err)
	}
	if k.Type != "API_KEY" || k.WorkspaceID != "ws-1" || k.KeyID != "key-9" || !k.ExpiresAt.Equal(exp) {
		t.Fatalf("got %+v", k)
	}
	if k.Expired(exp.Add(-time.Second)) || !k.Expired(exp) {
		t.Fatal("Expired is wrong around the expiry instant")
	}
}

func TestParseAPIKeyWithoutExp(t *testing.T) {
	k, err := ParseAPIKey(jwt(t, map[string]any{"type": "API_KEY", "workspaceId": "ws-1", "jti": "k"}))
	if err != nil {
		t.Fatal(err)
	}
	if !k.ExpiresAt.IsZero() || k.Expired(time.Now()) {
		t.Fatalf("a key without exp never expires, got %+v", k)
	}
}

func TestParseAPIKeyRefusals(t *testing.T) {
	cases := map[string]struct{ raw, want string }{
		"not a jwt":        {"abc", "three base64url parts"},
		"empty part":       {"a..c", "three base64url parts"},
		"bad base64":       {"a.!!!.c", "not base64url"},
		"not json":         {"a." + base64.RawURLEncoding.EncodeToString([]byte("[1]")) + ".c", "not a JSON object"},
		"user token":       {jwt(t, map[string]any{"type": "ACCESS", "workspaceId": "ws-1"}), "ACCESS token, not an API key"},
		"untyped":          {jwt(t, map[string]any{"workspaceId": "ws-1"}), "untyped token"},
		"no workspace":     {jwt(t, map[string]any{"type": "API_KEY"}), "names no workspace"},
	}
	for name, c := range cases {
		_, err := ParseAPIKey(c.raw)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want it to contain %q", name, err, c.want)
		}
	}
}

func TestDecodeIsLenient(t *testing.T) {
	k, err := Decode(jwt(t, map[string]any{"type": "ACCESS", "workspaceId": "ws-2"}))
	if err != nil || k.Type != "ACCESS" || k.WorkspaceID != "ws-2" {
		t.Fatalf("Decode = %+v, %v", k, err)
	}
}
```

- [ ] **Step 3: Run the auth tests to verify they fail**

Run: `go test ./internal/auth/`
Expected: FAIL (build error: undefined `CleanKey`, `ParseAPIKey`, `Decode`).

- [ ] **Step 4: Implement `internal/auth/auth.go`**

```go
// Package auth reads a Twenty API key. A key is a JWT signed by the Twenty
// server; the CLI cannot verify the signature and does not try. It reads the
// payload only to learn the workspace, the key's ID and its expiry.
package auth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// CleanKey trims whitespace and a leading "Bearer " that comes along when a
// key is copied from an Authorization header or a curl example.
func CleanKey(raw string) string {
	s := strings.TrimSpace(raw)
	if len(s) > len("bearer ") && strings.EqualFold(s[:len("bearer ")], "bearer ") {
		s = strings.TrimSpace(s[len("bearer "):])
	}
	return s
}

// Key is what the CLI can learn from an API key without the server's secret.
type Key struct {
	Type        string    // "API_KEY" for an API key, "ACCESS" for a user session token
	WorkspaceID string    // the workspace the key belongs to
	KeyID       string    // the jti claim: the key's ID in Twenty
	ExpiresAt   time.Time // zero when the key carries no exp claim
}

// Expired reports whether the key has an expiry at or before now.
func (k *Key) Expired(now time.Time) bool {
	return !k.ExpiresAt.IsZero() && !now.Before(k.ExpiresAt)
}

// Decode reads the payload of any JWT. It does not check what kind of token
// it is; ParseAPIKey does.
func Decode(raw string) (*Key, error) {
	parts := strings.Split(CleanKey(raw), ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return nil, errors.New("not a Twenty API key: an API key is a JWT, three base64url parts joined by dots")
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return nil, errors.New("not a Twenty API key: its middle part is not base64url")
	}
	var c struct {
		Type        string   `json:"type"`
		WorkspaceID string   `json:"workspaceId"`
		JTI         string   `json:"jti"`
		Exp         *float64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &c); err != nil {
		return nil, errors.New("not a Twenty API key: its middle part is not a JSON object")
	}
	k := &Key{Type: c.Type, WorkspaceID: c.WorkspaceID, KeyID: c.JTI}
	if c.Exp != nil {
		k.ExpiresAt = time.Unix(int64(*c.Exp), 0).UTC()
	}
	return k, nil
}

// ParseAPIKey decodes raw and insists that it is an API key for a workspace.
func ParseAPIKey(raw string) (*Key, error) {
	k, err := Decode(raw)
	if err != nil {
		return nil, err
	}
	if k.Type != "API_KEY" {
		kind := k.Type
		if kind == "" {
			kind = "untyped"
		}
		return nil, fmt.Errorf("this is a Twenty %s token, not an API key; create an API key in Twenty under Settings, APIs & Webhooks", kind)
	}
	if k.WorkspaceID == "" {
		return nil, errors.New("the API key names no workspace (its payload has no workspaceId)")
	}
	return k, nil
}
```

- [ ] **Step 5: Run the auth tests to verify they pass**

Run: `go test ./internal/auth/`
Expected: PASS.

- [ ] **Step 6: Write the failing config tests**

`internal/config/config_test.go`:

```go
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
	for v, want := range map[string]bool{"1": true, "true": true, "TRUE": true, "0": false, "false": false, "": false} {
		r, _ := Resolve(env(map[string]string{"TWENTY_READ_ONLY": v}))
		if r.ReadOnly != want {
			t.Errorf("TWENTY_READ_ONLY=%q: ReadOnly = %v", v, r.ReadOnly)
		}
	}
	Save(Config{ReadOnly: true})
	if r, _ := Resolve(env(map[string]string{"TWENTY_READ_ONLY": "0"})); !r.ReadOnly {
		t.Fatal("the environment must not switch off read-only mode set in the file")
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
```

- [ ] **Step 7: Run the config tests to verify they fail**

Run: `go test ./internal/config/`
Expected: FAIL (build error: undefined `Save`, `Load`, `NormalizeBaseURL`, ...).

- [ ] **Step 8: Implement `internal/config/config.go`**

```go
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
```

- [ ] **Step 9: Run the config tests to verify they pass**

Run: `go test ./internal/config/`
Expected: PASS.

- [ ] **Step 10: Write the failing routes tests**

`internal/routes/routes_test.go`:

```go
package routes

import (
	"fmt"
	"strings"
	"testing"
)

const id = "3f2b8c1e-5d4a-4c1b-9e8f-1a2b3c4d5e6f"

func TestObjectVerbTable(t *testing.T) {
	want := []struct {
		name, method, path, class, soft string
		filter                          bool
		body                            BodyKind
	}{
		{"list", "GET", "rest/companies", ClassRead, "", false, BodyNone},
		{"get", "GET", "rest/companies/" + id, ClassRead, "", false, BodyNone},
		{"group-by", "GET", "rest/companies/groupBy", ClassRead, "", false, BodyNone},
		{"find-duplicates", "POST", "rest/companies/duplicates", ClassRead, "", false, BodyDuplicates},
		{"create", "POST", "rest/companies", ClassWrite, "", false, BodyObject},
		{"batch-create", "POST", "rest/batch/companies", ClassWrite, "", false, BodyArray},
		{"update", "PATCH", "rest/companies/" + id, ClassWrite, "", false, BodyObject},
		{"delete", "DELETE", "rest/companies/" + id, ClassWrite, "true", false, BodyNone},
		{"restore", "PATCH", "rest/restore/companies/" + id, ClassWrite, "", false, BodyNone},
		{"update-many", "PATCH", "rest/companies", ClassBulk, "", true, BodyObject},
		{"delete-many", "DELETE", "rest/companies", ClassBulk, "true", true, BodyNone},
		{"restore-many", "PATCH", "rest/restore/companies", ClassBulk, "", true, BodyNone},
		{"merge", "PATCH", "rest/companies/merge", ClassBulk, "", false, BodyMerge},
		{"destroy", "DELETE", "rest/companies/" + id, ClassDestroy, "false", false, BodyNone},
		{"destroy-many", "DELETE", "rest/companies", ClassDestroy, "false", true, BodyNone},
	}
	if len(ObjectVerbs) != len(want) {
		t.Fatalf("%d object verbs, want %d", len(ObjectVerbs), len(want))
	}
	for i, w := range want {
		v := ObjectVerbs[i]
		got := v.Path("companies", id)
		if v.Name != w.name || v.Method != w.method || got != w.path || v.Class != w.class ||
			v.SoftDelete != w.soft || v.FilterRequired != w.filter || v.Body != w.body {
			t.Errorf("verb %d = %+v (path %s), want %+v", i, v, got, w)
		}
		if v.TakesID != strings.Contains(v.PathTemplate, "{id}") {
			t.Errorf("%s: TakesID disagrees with the path template", v.Name)
		}
		if v.Summary == "" {
			t.Errorf("%s: no summary", v.Name)
		}
	}
	if v, _ := FindVerb(ObjectVerbs, "list"); v.MaxLimit != 200 {
		t.Errorf("list MaxLimit = %d, want 200", v.MaxLimit)
	}
	if v, _ := FindVerb(ObjectVerbs, "group-by"); v.RequiredFlag != "group-by" {
		t.Errorf("group-by must require --group-by")
	}
	if _, ok := FindVerb(ObjectVerbs, "nope"); ok {
		t.Error("FindVerb found a verb that does not exist")
	}
}

func TestMetadataTables(t *testing.T) {
	if len(MetadataKinds) != 13 {
		t.Fatalf("%d metadata kinds, want 13", len(MetadataKinds))
	}
	seg := map[string]string{}
	for _, k := range MetadataKinds {
		seg[k.Command] = k.Segment
	}
	if seg["view-filter-groups"] != "viewFilterGroups" || seg["api-keys"] != "apiKeys" || seg["objects"] != "objects" {
		t.Fatalf("segments = %v", seg)
	}
	list, _ := FindVerb(MetadataVerbs, "list")
	if list.Path("viewFields", "") != "rest/metadata/viewFields" || list.MaxLimit != 1000 || list.Class != ClassRead {
		t.Fatalf("metadata list = %+v", list)
	}
	del, _ := FindVerb(MetadataVerbs, "delete")
	if del.Path("fields", id) != "rest/metadata/fields/"+id || del.Class != ClassAdmin || del.Method != "DELETE" {
		t.Fatalf("metadata delete = %+v", del)
	}
	for _, verb := range []string{"create", "update", "delete"} {
		if MetadataBlocked("api-keys", verb) == "" {
			t.Errorf("api-keys %s must be blocked", verb)
		}
		if MetadataBlocked("webhooks", verb) != "" {
			t.Errorf("webhooks %s must not be blocked", verb)
		}
	}
	for _, verb := range []string{"list", "get"} {
		if MetadataBlocked("api-keys", verb) != "" {
			t.Errorf("api-keys %s must stay readable", verb)
		}
	}
}

func TestNeedsForce(t *testing.T) {
	for class, want := range map[string]bool{ClassRead: false, ClassWrite: false, ClassBulk: true, ClassDestroy: true, ClassAdmin: true} {
		if NeedsForce(class) != want {
			t.Errorf("NeedsForce(%s) = %v", class, !want)
		}
	}
}

func TestValidateID(t *testing.T) {
	for _, ok := range []string{id, strings.ToUpper(id)} {
		if err := ValidateID(ok); err != nil {
			t.Errorf("ValidateID(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"", "42", id + "/x", "../" + id, id[:35], " " + id, "3f2b8c1e5d4a4c1b9e8f1a2b3c4d5e6f"} {
		if err := ValidateID(bad); err == nil {
			t.Errorf("ValidateID(%q) accepted", bad)
		}
	}
}

func TestCheckBody(t *testing.T) {
	sixty, sixtyOne := batch(60), batch(61)
	cases := []struct {
		kind BodyKind
		body string // "<nil>" means --data was not given
		want string // "" means accepted
	}{
		{BodyNone, "<nil>", ""},
		{BodyNone, `{}`, "takes no --data"},
		{BodyObject, "<nil>", "--data is required"},
		{BodyObject, `{"name":"Acme"}`, ""},
		{BodyObject, `[]`, "JSON object"},
		{BodyObject, `"x"`, "JSON object"},
		{BodyArray, `[]`, "at least one record"},
		{BodyArray, `{"name":"A"}`, "JSON array"},
		{BodyArray, `[{"name":"A"},3]`, "record 2 is not a JSON object"},
		{BodyArray, sixty, ""},
		{BodyArray, sixtyOne, "at most 60"},
		{BodyMerge, `{"ids":["a","b"],"conflictPriorityIndex":0}`, ""},
		{BodyMerge, `{"ids":["a"]}`, "at least two"},
		{BodyMerge, `{"ids":["a",2]}`, "at least two"},
		{BodyMerge, `{"conflictPriorityIndex":0}`, "at least two"},
		{BodyDuplicates, `{"ids":["a"]}`, ""},
		{BodyDuplicates, `{"data":[{"name":"A"}]}`, ""},
		{BodyDuplicates, `{"ids":[]}`, `"ids" or "data"`},
		{BodyDuplicates, `{}`, `"ids" or "data"`},
	}
	for _, c := range cases {
		var body []byte
		if c.body != "<nil>" {
			body = []byte(c.body)
		}
		err := CheckBody(c.kind, body)
		if c.want == "" && err != nil || c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
			t.Errorf("CheckBody(%s, %.40s) = %v, want %q", c.kind, c.body, err, c.want)
		}
	}
}

func batch(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = fmt.Sprintf(`{"name":"C%d"}`, i)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func TestDecisionOrderAndMessages(t *testing.T) {
	cases := []struct {
		d    Decision
		want string
	}{
		{Decision{Command: "companies list", Class: ClassRead, ReadOnly: true}, ""},
		{Decision{Command: "companies create", Class: ClassWrite}, ""},
		{Decision{Command: "companies create", Class: ClassWrite, ReadOnly: true}, "read-only mode"},
		{Decision{Command: "companies update-many", Class: ClassBulk, FilterRequired: true}, "needs --filter"},
		{Decision{Command: "companies update-many", Class: ClassBulk, FilterRequired: true, Filter: "  "}, "needs --filter"},
		{Decision{Command: "companies update-many", Class: ClassBulk, FilterRequired: true, Filter: "a[eq]:1"}, "needs --force"},
		{Decision{Command: "companies update-many", Class: ClassBulk, FilterRequired: true, Filter: "a[eq]:1", Force: true}, ""},
		{Decision{Command: "companies destroy", Class: ClassDestroy}, "deletes permanently"},
		{Decision{Command: "metadata fields create", Class: ClassAdmin}, "needs --force"},
		{Decision{Command: "metadata api-keys delete", Class: ClassAdmin, Blocked: "reason", Force: true}, "is blocked: reason"},
		// Order: the filter comes before the block, the block before read-only, read-only before --force.
		{Decision{Command: "x", Class: ClassBulk, FilterRequired: true, Blocked: "b", ReadOnly: true}, "needs --filter"},
		{Decision{Command: "x", Class: ClassAdmin, Blocked: "b", ReadOnly: true}, "is blocked"},
		{Decision{Command: "x", Class: ClassDestroy, ReadOnly: true}, "read-only mode"},
	}
	for _, c := range cases {
		err := c.d.Check()
		if c.want == "" && err != nil || c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
			t.Errorf("%+v: Check = %v, want %q", c.d, err, c.want)
		}
	}
}
```

`internal/routes/raw_test.go`:

```go
package routes

import (
	"net/url"
	"strings"
	"testing"
)

func TestClassifyRaw(t *testing.T) {
	cases := []struct {
		method, path, query string
		class               string
		blocked, filter     bool
	}{
		{"GET", "rest/apiKeys", "", ClassRead, false, false},
		{"POST", "rest/apiKeys", "", ClassAdmin, true, false},
		{"DELETE", "rest/metadata/apiKeys/" + id, "", ClassAdmin, true, false},
		{"GET", "rest/metadata/objects", "", ClassRead, false, false},
		{"POST", "rest/metadata/fields", "", ClassAdmin, false, false},
		{"PATCH", "rest/webhooks/" + id, "", ClassAdmin, false, false},
		{"GET", "rest/open-api/core", "", ClassRead, false, false},
		{"POST", "rest/batch/companies", "", ClassWrite, false, false},
		{"PATCH", "rest/restore/companies/" + id, "", ClassWrite, false, false},
		{"PATCH", "rest/restore/companies", "filter=a[eq]:1", ClassBulk, false, true},
		{"POST", "rest/companies/duplicates", "", ClassRead, false, false},
		{"PATCH", "rest/companies/merge", "", ClassBulk, false, false},
		{"GET", "rest/companies/groupBy", "group_by=x", ClassRead, false, false},
		{"GET", "rest/companies/" + id, "", ClassRead, false, false},
		{"PATCH", "rest/companies/" + id, "", ClassWrite, false, false},
		{"PUT", "rest/companies/" + id, "", ClassWrite, false, false},
		{"DELETE", "rest/companies/" + id, "soft_delete=true", ClassWrite, false, false},
		{"DELETE", "rest/companies/" + id, "", ClassDestroy, false, false},
		{"DELETE", "rest/companies/" + id, "soft_delete=TRUE", ClassDestroy, false, false},
		{"DELETE", "rest/companies/" + id, "soft_delete=false", ClassDestroy, false, false},
		{"GET", "rest/companies", "limit=5", ClassRead, false, false},
		{"POST", "rest/companies", "", ClassWrite, false, false},
		{"PATCH", "rest/companies", "filter=a[eq]:1", ClassBulk, false, true},
		{"PUT", "rest/companies", "", ClassBulk, false, true},
		{"DELETE", "rest/companies", "soft_delete=true&filter=a[eq]:1", ClassBulk, false, true},
		{"DELETE", "rest/companies", "filter=a[eq]:1", ClassDestroy, false, true},
		{"POST", "rest/companies/" + id, "", ClassAdmin, false, false},
		{"POST", "rest/dashboards/" + id + "/duplicate", "", ClassAdmin, false, false},
		{"GET", "rest/dashboards/" + id + "/duplicate", "", ClassRead, false, false},
		{"POST", "rest", "", ClassAdmin, false, false},
	}
	for _, c := range cases {
		q, _ := url.ParseQuery(c.query)
		got, err := ClassifyRaw(c.method, c.path, q)
		if err != nil {
			t.Errorf("%s %s?%s: %v", c.method, c.path, c.query, err)
			continue
		}
		if got.Class != c.class || (got.Blocked != "") != c.blocked || got.FilterRequired != c.filter {
			t.Errorf("%s %s?%s = %+v, want class %s blocked %v filter %v", c.method, c.path, c.query, got, c.class, c.blocked, c.filter)
		}
	}
}

func TestClassifyRawAcceptsLeadingSlashAndLowercaseMethod(t *testing.T) {
	got, err := ClassifyRaw("delete", "/rest/companies/"+id, url.Values{"soft_delete": {"true"}})
	if err != nil || got.Class != ClassWrite {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestClassifyRawRefusals(t *testing.T) {
	cases := []struct {
		method, path string
		q            url.Values
		want         string
	}{
		{"GET", "graphql", nil, "GraphQL is not supported"},
		{"POST", "metadata", nil, "GraphQL is not supported"},
		{"GET", "healthz", nil, "only paths under rest/"},
		{"GET", "rest/companies?limit=1", nil, "--query"},
		{"GET", "rest/companies#x", nil, "--query"},
		{"DELETE", "rest/companies/%2e%2e", nil, "not allowed"},
		{"DELETE", "rest/./companies", nil, "not allowed"},
		{"DELETE", "rest/../rest/companies", nil, "not allowed"},
		{"DELETE", "rest//companies", nil, "not allowed"},
		{"DELETE", "rest/companies/", nil, "not allowed"},
		{"GET", `rest\companies`, nil, "not allowed"},
		{"GET", "rest/com panies", nil, "not allowed"},
		{"TRACE", "rest/companies", nil, "method"},
		{"DELETE", "rest/companies/" + id, url.Values{"soft_delete": {"true", "true"}}, "soft_delete may appear only once"},
		{"PATCH", "rest/companies", url.Values{"filter": {"a[eq]:1", "b[eq]:2"}}, "filter may appear only once"},
	}
	for _, c := range cases {
		if _, err := ClassifyRaw(c.method, c.path, c.q); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s %s %v: err = %v, want %q", c.method, c.path, c.q, err, c.want)
		}
	}
}
```

- [ ] **Step 11: Run the routes tests to verify they fail**

Run: `go test ./internal/routes/`
Expected: FAIL (build error: undefined `ObjectVerbs`, `ClassifyRaw`, ...).

- [ ] **Step 12: Implement `internal/routes/routes.go`**

```go
// Package routes is the one place that knows Twenty's REST grammar: which
// request each command sends, which risk class it carries, and which local
// checks it must pass before anything goes over the network. The grammar is
// the same for every object, so it lives in code, not in a generated spec.
package routes

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Risk classes. Every request the CLI sends carries exactly one.
const (
	ClassRead    = "read"    // changes nothing
	ClassWrite   = "write"   // creates or changes records, one call at a time
	ClassBulk    = "bulk"    // changes every record a filter matches, or merges records
	ClassDestroy = "destroy" // deletes permanently
	ClassAdmin   = "admin"   // changes the data model, views, webhooks, or an unknown path
)

// NeedsForce reports whether a class is gated by --force.
func NeedsForce(class string) bool {
	return class == ClassBulk || class == ClassDestroy || class == ClassAdmin
}

// BodyKind says what --data must hold for a verb.
type BodyKind string

const (
	BodyNone       BodyKind = ""
	BodyObject     BodyKind = "object"     // one JSON object
	BodyArray      BodyKind = "array"      // 1 to MaxBatch JSON objects
	BodyMerge      BodyKind = "merge"      // {"ids":[two or more strings], ...}
	BodyDuplicates BodyKind = "duplicates" // {"ids":[...]} or {"data":[...]}
)

// MaxBatch is the number of records Twenty accepts in one batch call.
const MaxBatch = 60

// Verb is one command of the fixed grammar.
type Verb struct {
	Name           string
	Method         string
	PathTemplate   string // "rest/{plural}/{id}"; {plural} is the object's or the metadata kind's API name
	Class          string
	TakesID        bool
	FilterRequired bool
	Body           BodyKind
	SoftDelete     string   // fixed soft_delete query value; "" when not sent
	Flags          []string // verb-specific flags (the CLI owns their types and help)
	RequiredFlag   string   // a flag that must be given, "" for none
	MaxLimit       int      // upper bound for --limit; 0 when the verb has no --limit
	Summary        string
}

// Path fills the template.
func (v Verb) Path(plural, id string) string {
	return strings.ReplaceAll(strings.ReplaceAll(v.PathTemplate, "{plural}", plural), "{id}", id)
}

// FindVerb looks a verb up by name.
func FindVerb(verbs []Verb, name string) (Verb, bool) {
	for _, v := range verbs {
		if v.Name == name {
			return v, true
		}
	}
	return Verb{}, false
}

var listFlags = []string{"filter", "order-by", "limit", "depth", "starting-after", "ending-before", "all", "max-pages"}

// ObjectVerbs is the verb table every object gets (spec: Object commands).
var ObjectVerbs = []Verb{
	{Name: "list", Method: "GET", PathTemplate: "rest/{plural}", Class: ClassRead, Flags: listFlags, MaxLimit: 200,
		Summary: "List records (cursor paging; --all merges every page)"},
	{Name: "get", Method: "GET", PathTemplate: "rest/{plural}/{id}", Class: ClassRead, TakesID: true, Flags: []string{"depth"},
		Summary: "Get one record by ID"},
	{Name: "group-by", Method: "GET", PathTemplate: "rest/{plural}/groupBy", Class: ClassRead, RequiredFlag: "group-by", MaxLimit: 200,
		Flags:   []string{"group-by", "aggregate", "filter", "order-by", "limit", "view-id", "include-records-sample", "order-by-for-records"},
		Summary: "Group records and aggregate each group"},
	{Name: "find-duplicates", Method: "POST", PathTemplate: "rest/{plural}/duplicates", Class: ClassRead, Body: BodyDuplicates, Flags: []string{"depth"},
		Summary: "Find records that duplicate the given IDs or data"},
	{Name: "create", Method: "POST", PathTemplate: "rest/{plural}", Class: ClassWrite, Body: BodyObject, Flags: []string{"upsert", "depth"},
		Summary: "Create one record"},
	{Name: "batch-create", Method: "POST", PathTemplate: "rest/batch/{plural}", Class: ClassWrite, Body: BodyArray, Flags: []string{"upsert", "depth"},
		Summary: "Create up to 60 records in one call"},
	{Name: "update", Method: "PATCH", PathTemplate: "rest/{plural}/{id}", Class: ClassWrite, TakesID: true, Body: BodyObject, Flags: []string{"depth"},
		Summary: "Update one record"},
	{Name: "delete", Method: "DELETE", PathTemplate: "rest/{plural}/{id}", Class: ClassWrite, TakesID: true, SoftDelete: "true",
		Summary: "Move one record to the trash (restore brings it back)"},
	{Name: "restore", Method: "PATCH", PathTemplate: "rest/restore/{plural}/{id}", Class: ClassWrite, TakesID: true, Flags: []string{"depth"},
		Summary: "Restore one record from the trash"},
	{Name: "update-many", Method: "PATCH", PathTemplate: "rest/{plural}", Class: ClassBulk, FilterRequired: true, Body: BodyObject, Flags: []string{"filter", "depth"},
		Summary: "Update every record the filter matches"},
	{Name: "delete-many", Method: "DELETE", PathTemplate: "rest/{plural}", Class: ClassBulk, FilterRequired: true, SoftDelete: "true", Flags: []string{"filter"},
		Summary: "Move every record the filter matches to the trash"},
	{Name: "restore-many", Method: "PATCH", PathTemplate: "rest/restore/{plural}", Class: ClassBulk, FilterRequired: true, Flags: []string{"filter", "depth"},
		Summary: "Restore every record the filter matches from the trash"},
	{Name: "merge", Method: "PATCH", PathTemplate: "rest/{plural}/merge", Class: ClassBulk, Body: BodyMerge, Flags: []string{"dry-run", "depth"},
		Summary: "Merge duplicate records into one (--dry-run previews without changing anything)"},
	{Name: "destroy", Method: "DELETE", PathTemplate: "rest/{plural}/{id}", Class: ClassDestroy, TakesID: true, SoftDelete: "false",
		Summary: "Delete one record permanently"},
	{Name: "destroy-many", Method: "DELETE", PathTemplate: "rest/{plural}", Class: ClassDestroy, FilterRequired: true, SoftDelete: "false", Flags: []string{"filter"},
		Summary: "Delete every record the filter matches permanently"},
}

// MetaKind is one kind of metadata entry: its command name and its segment
// under rest/metadata/.
type MetaKind struct {
	Command string
	Segment string
}

// MetadataKinds lists the metadata endpoints of Twenty v2.27.
var MetadataKinds = []MetaKind{
	{"objects", "objects"}, {"fields", "fields"}, {"views", "views"}, {"view-fields", "viewFields"},
	{"view-filters", "viewFilters"}, {"view-sorts", "viewSorts"}, {"view-groups", "viewGroups"},
	{"view-filter-groups", "viewFilterGroups"}, {"page-layouts", "pageLayouts"}, {"page-layout-tabs", "pageLayoutTabs"},
	{"page-layout-widgets", "pageLayoutWidgets"}, {"webhooks", "webhooks"}, {"api-keys", "apiKeys"},
}

// MetadataVerbs is the verb table every metadata kind gets.
var MetadataVerbs = []Verb{
	{Name: "list", Method: "GET", PathTemplate: "rest/metadata/{plural}", Class: ClassRead, MaxLimit: 1000,
		Flags: []string{"limit", "starting-after", "ending-before", "all", "max-pages"}, Summary: "List entries"},
	{Name: "get", Method: "GET", PathTemplate: "rest/metadata/{plural}/{id}", Class: ClassRead, TakesID: true, Summary: "Get one entry by ID"},
	{Name: "create", Method: "POST", PathTemplate: "rest/metadata/{plural}", Class: ClassAdmin, Body: BodyObject, Summary: "Create an entry"},
	{Name: "update", Method: "PATCH", PathTemplate: "rest/metadata/{plural}/{id}", Class: ClassAdmin, TakesID: true, Body: BodyObject, Summary: "Update an entry"},
	{Name: "delete", Method: "DELETE", PathTemplate: "rest/metadata/{plural}/{id}", Class: ClassAdmin, TakesID: true,
		Summary: "Delete an entry (an object or a field goes with its data, permanently)"},
}

// APIKeysBlocked is why the CLI never creates, changes or revokes API keys.
const APIKeysBlocked = "Twenty lets only a signed-in user manage API keys, a new key would appear once in the response " +
	"and so in an agent's transcript, and revoking the CLI's own key would cut it off; manage keys in Twenty under Settings, APIs & Webhooks"

// MetadataBlocked returns the reason a metadata command is blocked, or "".
func MetadataBlocked(kindCommand, verb string) string {
	if kindCommand == "api-keys" && verb != "list" && verb != "get" {
		return APIKeysBlocked
	}
	return ""
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ValidateID accepts a UUID only, so no argument can add a path segment.
func ValidateID(id string) error {
	if !uuidPattern.MatchString(id) {
		return fmt.Errorf("%q is not a record ID: Twenty IDs are UUIDs such as 3f2b8c1e-5d4a-4c1b-9e8f-1a2b3c4d5e6f", id)
	}
	return nil
}

// CheckBody validates --data for a verb. body is nil when --data was not
// given. The body is only inspected, never changed.
func CheckBody(kind BodyKind, body []byte) error {
	if kind == BodyNone {
		if body != nil {
			return errors.New("this command takes no --data")
		}
		return nil
	}
	if body == nil {
		return errors.New("--data is required: a JSON literal, @file.json, or - for stdin")
	}
	switch kind {
	case BodyObject:
		if _, err := object(body); err != nil {
			return err
		}
	case BodyArray:
		var records []json.RawMessage
		if err := json.Unmarshal(body, &records); err != nil {
			return errors.New("--data must be a JSON array of records")
		}
		if len(records) == 0 {
			return errors.New("--data must hold at least one record")
		}
		if len(records) > MaxBatch {
			return fmt.Errorf("--data holds %d records; batch-create takes at most %d per call. Split it yourself: "+
				"the CLI does not, so that each call succeeds or fails as a whole", len(records), MaxBatch)
		}
		for i, r := range records {
			if !isObject(r) {
				return fmt.Errorf("record %d is not a JSON object", i+1)
			}
		}
	case BodyMerge:
		m, err := object(body)
		if err != nil {
			return err
		}
		var ids []string
		if json.Unmarshal(m["ids"], &ids) != nil || len(ids) < 2 {
			return errors.New(`--data must name at least two record IDs to merge: {"ids":["<id>","<id>"],"conflictPriorityIndex":0}`)
		}
	case BodyDuplicates:
		m, err := object(body)
		if err != nil {
			return err
		}
		var ids, data []json.RawMessage
		json.Unmarshal(m["ids"], &ids)
		json.Unmarshal(m["data"], &data)
		if len(ids) == 0 && len(data) == 0 {
			return errors.New(`--data must hold "ids" or "data": {"ids":["<id>"]} or {"data":[{"name":"Acme"}]}`)
		}
	}
	return nil
}

func object(body []byte) (map[string]json.RawMessage, error) {
	if !isObject(body) {
		return nil, errors.New("--data must be a JSON object")
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, errors.New("--data must be a JSON object")
	}
	return m, nil
}

func isObject(raw []byte) bool {
	t := bytes.TrimSpace(raw)
	return len(t) > 0 && t[0] == '{'
}

// Decision is everything the gates need to know about one call.
type Decision struct {
	Command        string // "companies update-many", for messages
	Class          string
	Blocked        string // reason; non-empty refuses the call
	FilterRequired bool
	Filter         string
	ReadOnly       bool
	Force          bool
}

var forceReasons = map[string]string{
	ClassBulk:    "it changes every record the filter matches, or merges records",
	ClassDestroy: "it deletes permanently; `delete` moves a record to the trash instead",
	ClassAdmin:   "it changes the data model, views or webhooks, or calls a path the CLI does not know",
}

// Check applies the gates in the spec's order, after the shape checks: a
// missing filter, a blocked command, read-only mode, then --force.
func (d Decision) Check() error {
	if d.FilterRequired && strings.TrimSpace(d.Filter) == "" {
		return fmt.Errorf("%s needs --filter: without one it would act on every record", d.Command)
	}
	if d.Blocked != "" {
		return fmt.Errorf("%s is blocked: %s", d.Command, d.Blocked)
	}
	if d.ReadOnly && d.Class != ClassRead {
		return fmt.Errorf("read-only mode (TWENTY_READ_ONLY or `config set read-only true`) blocks %s, a %s-class call", d.Command, d.Class)
	}
	if NeedsForce(d.Class) && !d.Force {
		return fmt.Errorf("%s is a %s-class call and needs --force: %s", d.Command, d.Class, forceReasons[d.Class])
	}
	return nil
}
```

- [ ] **Step 13: Implement `internal/routes/raw.go`**

```go
package routes

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode"
)

var rawMethods = map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}

// CleanRawPath validates a path for the api escape hatch: relative to the
// base URL, under rest/, no query string, and every segment plain, so no
// spelling of a path can dodge ClassifyRaw.
func CleanRawPath(p string) (string, error) {
	p = strings.TrimPrefix(p, "/")
	if strings.ContainsAny(p, "?#") {
		return "", errors.New("put query parameters in --query k=v, not in the path")
	}
	segs := strings.Split(p, "/")
	for _, s := range segs {
		if s == "" || s == "." || s == ".." || strings.ContainsAny(s, `%\`) || strings.IndexFunc(s, unicode.IsSpace) >= 0 {
			return "", fmt.Errorf("path segment %q is not allowed: segments must be non-empty, not . or .., and free of %%, whitespace and backslashes", s)
		}
	}
	if segs[0] != "rest" {
		if segs[0] == "graphql" || segs[0] == "metadata" {
			return "", errors.New("GraphQL is not supported in v1; use the REST paths under rest/")
		}
		return "", errors.New("only paths under rest/ are allowed, for example rest/companies")
	}
	return p, nil
}

// Raw is how the api escape hatch treats one request.
type Raw struct {
	Class          string
	Blocked        string
	FilterRequired bool
}

// ClassifyRaw gives a raw request its class from the route grammar alone,
// independent of the workspace model (spec: The api escape hatch). Rows are
// tried in the spec's order; the first match wins.
func ClassifyRaw(method, path string, q url.Values) (Raw, error) {
	method = strings.ToUpper(method)
	if !rawMethods[method] {
		return Raw{}, fmt.Errorf("method %q is not allowed (GET, POST, PUT, PATCH, DELETE)", method)
	}
	p, err := CleanRawPath(path)
	if err != nil {
		return Raw{}, err
	}
	for _, k := range []string{"soft_delete", "filter"} {
		if len(q[k]) > 1 {
			return Raw{}, fmt.Errorf("--query %s may appear only once", k)
		}
	}
	segs := strings.Split(p, "/")[1:]
	get := method == "GET"
	// Twenty compares soft_delete with the string "true"; anything else,
	// including TRUE, deletes permanently.
	soft := q.Get("soft_delete") == "true"
	readOr := func(other Raw) (Raw, error) {
		if get {
			return Raw{Class: ClassRead}, nil
		}
		return other, nil
	}
	n := len(segs)
	switch {
	case n == 0:
		return readOr(Raw{Class: ClassAdmin})
	case segs[0] == "apiKeys" || (segs[0] == "metadata" && n > 1 && segs[1] == "apiKeys"):
		return readOr(Raw{Class: ClassAdmin, Blocked: APIKeysBlocked})
	case segs[0] == "webhooks" || segs[0] == "metadata" || segs[0] == "open-api":
		return readOr(Raw{Class: ClassAdmin})
	case segs[0] == "batch" && n == 2 && method == "POST":
		return Raw{Class: ClassWrite}, nil
	case segs[0] == "restore" && n == 3 && method == "PATCH":
		return Raw{Class: ClassWrite}, nil
	case segs[0] == "restore" && n == 2 && method == "PATCH":
		return Raw{Class: ClassBulk, FilterRequired: true}, nil
	case n == 2 && segs[1] == "duplicates" && method == "POST":
		return Raw{Class: ClassRead}, nil
	case n == 2 && segs[1] == "merge" && method == "PATCH":
		return Raw{Class: ClassBulk}, nil
	case n == 2 && segs[1] == "groupBy" && get:
		return Raw{Class: ClassRead}, nil
	case n == 2:
		switch method {
		case "GET":
			return Raw{Class: ClassRead}, nil
		case "PATCH", "PUT":
			return Raw{Class: ClassWrite}, nil
		case "DELETE":
			if soft {
				return Raw{Class: ClassWrite}, nil
			}
			return Raw{Class: ClassDestroy}, nil
		}
	case n == 1:
		switch method {
		case "GET":
			return Raw{Class: ClassRead}, nil
		case "POST":
			return Raw{Class: ClassWrite}, nil
		case "PATCH", "PUT":
			return Raw{Class: ClassBulk, FilterRequired: true}, nil
		case "DELETE":
			if soft {
				return Raw{Class: ClassBulk, FilterRequired: true}, nil
			}
			return Raw{Class: ClassDestroy, FilterRequired: true}, nil
		}
	}
	return readOr(Raw{Class: ClassAdmin})
}
```

- [ ] **Step 14: Run all tests and vet**

Run: `go vet ./... && go test ./...`
Expected: PASS for `internal/auth`, `internal/config`, `internal/routes`.

- [ ] **Step 15: Commit**

```bash
git add go.mod LICENSE Makefile .gitignore .gitattributes internal/
git commit -m "Add config, API key parsing and the route grammar

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---
### Task 2: API client and workspace model

The two network-facing packages: the HTTP client that every call goes through, and the model that turns the workspace's OpenAPI document into objects and fields.

**Files:**
- Create: `internal/api/errors.go`, `internal/api/retry.go`, `internal/api/client.go`, `internal/api/twenty.go`, `internal/api/client_test.go`
- Create: `internal/model/model.go`, `internal/model/extract.go`, `internal/model/cache.go`, `internal/model/model_test.go`
- Create: `internal/model/testdata/openapi-core.json`, `internal/model/testdata/openapi-skeleton.json`

**Interfaces:**
- Consumes (Task 1): `routes.ClassRead`, `config.IsLoopback(host string) bool`.
- Produces:
  - `api.KindAuth|KindForbidden|KindNotFound|KindValidation|KindConflict|KindRateLimited|KindServer|KindTransport|KindOutcomeUnknown|KindIncomplete|KindUsage|KindOutputFailed` (strings)
  - `api.Error{Kind string \`json:"kind"\`; Message string \`json:"error"\`; Status int \`json:"status,omitempty"\`; Details any \`json:"details"\`}`, `(*Error).Error() string`, `api.Usagef(format string, a ...any) *Error`
  - `api.Client{BaseURL, APIKey string; ReadOnly bool; Timeout, RetryBudget time.Duration; MaxAttempts int; Verbose io.Writer; HTTP *http.Client; Sleep func(time.Duration)}`
  - `api.Request{Method, Path string; Query url.Values; Body []byte; Risk, Object string; Headers http.Header}`, `api.Response{Status int; Header http.Header; Body []byte}`
  - `(*Client).Do(ctx context.Context, r Request) (*Response, error)`; the error is always `*api.Error`
  - `model.Field{Name, Type, Format string; Enum, Subfields []string; Required, ReadOnly bool; Relation *Relation; Description string}` (JSON keys `name`, `type`, `format`, `enum`, `subfields`, `required`, `read_only`, `relation`, `description`, all but `name` omitempty)
  - `model.Relation{Target, Kind string}` (JSON `target`, `kind`; kind `many_to_one` or `one_to_many`)
  - `model.Object{Command, NamePlural, NameSingular, Description string; Fields []Field}` (JSON `command`, `name_plural`, `name_singular`, `description`, `fields`)
  - `model.Model{FetchedAt time.Time; BaseURL, WorkspaceID string; Objects []Object}` (JSON `fetched_at`, `base_url`, `workspace_id`, `objects`)
  - `model.MaxAge = 24 * time.Hour`, `(*Model).Stale(now time.Time) bool` (true for nil), `(*Model).Find(name string) *Object` (nil-safe; matches command, plural name, or the kebab-case singular)
  - `model.CommandName(apiName string) string`, `model.ErrNoObjects`, `model.Extract(doc []byte) ([]Object, error)`
  - `model.CachePath(root, baseURL, workspaceID string) string`, `model.LoadCache(root, baseURL, workspaceID string) *Model`, `model.SaveCache(root string, m *Model) error`

- [ ] **Step 1: Write the failing API client tests**

`internal/api/client_test.go`:

```go
package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type hits struct{ n atomic.Int32 }

func serve(t *testing.T, h func(w http.ResponseWriter, r *http.Request, n int32)) (*httptest.Server, *hits) {
	t.Helper()
	c := &hits{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h(w, r, c.n.Add(1))
	}))
	t.Cleanup(srv.Close)
	return srv, c
}

func newClient(srv *httptest.Server) (*Client, *[]time.Duration) {
	var waits []time.Duration
	return &Client{BaseURL: srv.URL, APIKey: "k.e.y", Sleep: func(d time.Duration) { waits = append(waits, d) }}, &waits
}

func asError(t *testing.T, err error) *Error {
	t.Helper()
	e, ok := err.(*Error)
	if !ok {
		t.Fatalf("error %T %v, want *Error", err, err)
	}
	return e
}

func TestDoSendsBearerAndJSON(t *testing.T) {
	srv, _ := serve(t, func(w http.ResponseWriter, r *http.Request, _ int32) {
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get("Authorization") != "Bearer k.e.y" || r.Header.Get("Accept") != "application/json" ||
			r.Header.Get("Content-Type") != "application/json" || r.URL.Path != "/rest/companies" ||
			r.URL.Query().Get("upsert") != "true" || string(body) != `{"name":"Acme"}` {
			t.Errorf("request = %s %s %v %s", r.Method, r.URL, r.Header, body)
		}
		w.WriteHeader(201)
		io.WriteString(w, `{"data":{"createCompany":{"id":"x"}}}`)
	})
	c, _ := newClient(srv)
	resp, err := c.Do(context.Background(), Request{Method: "POST", Path: "rest/companies",
		Query: url.Values{"upsert": {"true"}}, Body: []byte(`{"name":"Acme"}`), Risk: "write"})
	if err != nil || resp.Status != 201 || !strings.Contains(string(resp.Body), "createCompany") {
		t.Fatalf("Do = %+v, %v", resp, err)
	}
}

func TestGuardRefusesBeforeNetwork(t *testing.T) {
	srv, h := serve(t, func(w http.ResponseWriter, r *http.Request, _ int32) {})
	base := Request{Method: "GET", Path: "rest/companies", Risk: "read"}
	cases := []struct {
		name   string
		mutate func(*Client, *Request)
		want   string
	}{
		{"no base URL", func(c *Client, r *Request) { c.BaseURL = "" }, "no Twenty base URL"},
		{"no key", func(c *Client, r *Request) { c.APIKey = "" }, "no API key"},
		{"plain http", func(c *Client, r *Request) { c.BaseURL = "http://crm.example.com" }, "plain HTTP"},
		{"read-only write", func(c *Client, r *Request) { c.ReadOnly = true; r.Method, r.Risk = "POST", "write" }, "read-only mode"},
		{"read-only unclassified GET", func(c *Client, r *Request) { c.ReadOnly = true; r.Risk = "" }, "read-only mode"},
		{"non-GET without class", func(c *Client, r *Request) { r.Method, r.Risk = "PATCH", "" }, "no risk class"},
		{"body on GET", func(c *Client, r *Request) { r.Body = []byte(`{}`) }, "request body"},
		{"body on DELETE", func(c *Client, r *Request) { r.Method, r.Risk, r.Body = "DELETE", "write", []byte(`{}`) }, "request body"},
		{"authorization header", func(c *Client, r *Request) { r.Headers = http.Header{"authorization": {"x"}} }, "Authorization"},
		{"method override", func(c *Client, r *Request) { r.Headers = http.Header{"X-HTTP-Method-Override": {"DELETE"}} }, "method-override"},
		{"method override underscore", func(c *Client, r *Request) { r.Headers = http.Header{"x_http_method": {"DELETE"}} }, "method-override"},
	}
	for _, tc := range cases {
		c, _ := newClient(srv)
		r := base
		tc.mutate(c, &r)
		_, err := c.Do(context.Background(), r)
		e := asError(t, err)
		if e.Kind != KindUsage || !strings.Contains(e.Message, tc.want) {
			t.Errorf("%s: %+v, want usage containing %q", tc.name, e, tc.want)
		}
	}
	if h.n.Load() != 0 {
		t.Fatalf("%d requests reached the server", h.n.Load())
	}
}

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		status int
		kind   string
	}{{400, KindValidation}, {422, KindValidation}, {401, KindAuth}, {403, KindForbidden}, {404, KindNotFound},
		{409, KindConflict}, {410, KindNotFound}}
	for _, tc := range cases {
		srv, _ := serve(t, func(w http.ResponseWriter, r *http.Request, _ int32) {
			w.WriteHeader(tc.status)
			io.WriteString(w, `{"statusCode":400,"messages":["first problem","second problem"],"error":"BAD_REQUEST"}`)
		})
		c, _ := newClient(srv)
		_, err := c.Do(context.Background(), Request{Method: "GET", Path: "rest/companies", Risk: "read"})
		e := asError(t, err)
		if e.Kind != tc.kind || e.Status != tc.status ||
			!strings.Contains(e.Message, "GET rest/companies: HTTP ") || !strings.Contains(e.Message, "first problem (and 1 more)") {
			t.Errorf("status %d: %+v", tc.status, e)
		}
		if d, ok := e.Details.(map[string]any); !ok || d["error"] != "BAD_REQUEST" {
			t.Errorf("status %d: details = %#v", tc.status, e.Details)
		}
	}
}

func TestNestJSMessageShapeAndNonJSONBody(t *testing.T) {
	srv, _ := serve(t, func(w http.ResponseWriter, r *http.Request, _ int32) {
		if r.URL.Path == "/rest/x" {
			w.WriteHeader(404)
			io.WriteString(w, `{"statusCode":404,"message":"Cannot GET /rest/x","error":"Not Found"}`)
			return
		}
		w.WriteHeader(418)
		io.WriteString(w, "<html>teapot</html>")
	})
	c, _ := newClient(srv)
	_, err := c.Do(context.Background(), Request{Method: "GET", Path: "rest/x", Risk: "read"})
	if e := asError(t, err); !strings.Contains(e.Message, "Cannot GET /rest/x") {
		t.Errorf("NestJS message lost: %+v", e)
	}
	_, err = c.Do(context.Background(), Request{Method: "GET", Path: "rest/y", Risk: "read"})
	if e := asError(t, err); e.Details != "<html>teapot</html>" {
		t.Errorf("non-JSON details = %#v", e.Details)
	}
}

func TestHints(t *testing.T) {
	cases := []struct {
		status          int
		path, object    string
		message, want   string
	}{
		{401, "rest/companies", "", "This API Key has expired", "config set api-key"},
		{401, "rest/companies", "", "Token invalid", "auth status"},
		{403, "rest/metadata/objects", "", "Forbidden", "Data Model permission"},
		{403, "rest/metadata/fields/x", "", "Forbidden", "Data Model permission"},
		{403, "rest/companies", "companies", "Forbidden", "Settings, Roles"},
		{400, "rest/companies", "companies", `Object company doesn't have any "foo" field.`, "twentycrm schema companies"},
		{400, "rest/companies", "", `unknown field`, ""},
	}
	for _, tc := range cases {
		srv, _ := serve(t, func(w http.ResponseWriter, r *http.Request, _ int32) {
			w.WriteHeader(tc.status)
			io.WriteString(w, `{"statusCode":1,"messages":[`+jsonString(tc.message)+`],"error":"X"}`)
		})
		c, _ := newClient(srv)
		_, err := c.Do(context.Background(), Request{Method: "GET", Path: tc.path, Risk: "read", Object: tc.object})
		e := asError(t, err)
		if tc.want == "" && strings.Contains(e.Message, "hint") || tc.want != "" && !strings.Contains(e.Message, tc.want) {
			t.Errorf("%d %s: %s, want %q", tc.status, tc.path, e.Message, tc.want)
		}
	}
}

func jsonString(s string) string { return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"` }

func TestRetry429ThenSuccess(t *testing.T) {
	srv, h := serve(t, func(w http.ResponseWriter, r *http.Request, n int32) {
		if n == 1 {
			w.WriteHeader(429)
			io.WriteString(w, `{"statusCode":429,"messages":["Too many requests"],"error":"TOO_MANY_REQUESTS"}`)
			return
		}
		io.WriteString(w, `{}`)
	})
	c, waits := newClient(srv)
	if _, err := c.Do(context.Background(), Request{Method: "POST", Path: "rest/companies", Body: []byte(`{}`), Risk: "write"}); err != nil {
		t.Fatal(err)
	}
	if h.n.Load() != 2 || len(*waits) != 1 || (*waits)[0] < time.Second {
		t.Fatalf("hits %d waits %v", h.n.Load(), *waits)
	}
}

func TestRetryAfterBeyondBudgetFailsAtOnce(t *testing.T) {
	srv, h := serve(t, func(w http.ResponseWriter, r *http.Request, _ int32) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(429)
	})
	c, waits := newClient(srv)
	_, err := c.Do(context.Background(), Request{Method: "GET", Path: "rest/companies", Risk: "read"})
	e := asError(t, err)
	if e.Kind != KindRateLimited || !strings.Contains(e.Message, "retry budget") || !strings.Contains(e.Message, "100 requests per minute") ||
		h.n.Load() != 1 || len(*waits) != 0 {
		t.Fatalf("%+v hits %d waits %v", e, h.n.Load(), *waits)
	}
}

func TestServerErrorsRetriedForReadsOnly(t *testing.T) {
	srv, h := serve(t, func(w http.ResponseWriter, r *http.Request, _ int32) { w.WriteHeader(502) })
	c, _ := newClient(srv)
	_, err := c.Do(context.Background(), Request{Method: "GET", Path: "rest/companies", Risk: "read"})
	if e := asError(t, err); e.Kind != KindServer || h.n.Load() != 3 {
		t.Fatalf("read: %+v after %d hits", e, h.n.Load())
	}
	h.n.Store(0)
	_, err = c.Do(context.Background(), Request{Method: "PATCH", Path: "rest/companies/x", Body: []byte(`{}`), Risk: "write"})
	if e := asError(t, err); e.Kind != KindServer || h.n.Load() != 1 {
		t.Fatalf("write: %+v after %d hits", e, h.n.Load())
	}
}

func TestBrokenConnection(t *testing.T) {
	srv, h := serve(t, func(w http.ResponseWriter, r *http.Request, _ int32) {
		conn, _, _ := w.(http.Hijacker).Hijack()
		conn.Close()
	})
	c, _ := newClient(srv)
	_, err := c.Do(context.Background(), Request{Method: "POST", Path: "rest/companies", Body: []byte(`{}`), Risk: "write"})
	if e := asError(t, err); e.Kind != KindOutcomeUnknown || h.n.Load() != 1 {
		t.Fatalf("write: %+v after %d hits", e, h.n.Load())
	}
	h.n.Store(0)
	_, err = c.Do(context.Background(), Request{Method: "GET", Path: "rest/companies", Risk: "read"})
	if e := asError(t, err); e.Kind != KindTransport || h.n.Load() != 3 {
		t.Fatalf("read: %+v after %d hits", e, h.n.Load())
	}
}

func TestRedirectRefused(t *testing.T) {
	srv, h := serve(t, func(w http.ResponseWriter, r *http.Request, _ int32) {
		http.Redirect(w, r, "https://elsewhere.example.com/rest/companies", http.StatusFound)
	})
	c, _ := newClient(srv)
	_, err := c.Do(context.Background(), Request{Method: "GET", Path: "rest/companies", Risk: "read"})
	e := asError(t, err)
	if e.Kind != KindValidation || !strings.Contains(e.Message, "elsewhere.example.com") || h.n.Load() != 1 {
		t.Fatalf("%+v after %d hits", e, h.n.Load())
	}
}
```

- [ ] **Step 2: Run the API tests to verify they fail**

Run: `go test ./internal/api/`
Expected: FAIL (build error: undefined `Client`, `Request`, ...).

- [ ] **Step 3: Create `internal/api/errors.go` and `internal/api/retry.go`**

`internal/api/retry.go`: copy `/Users/daniel/Development/google-ads-cli/internal/api/retry.go` unchanged except the comment of `classifyTransport`, which says "never reached Twenty" instead of "never reached Google".

`internal/api/errors.go`:

```go
package api

import "fmt"

// Error kinds. Every error the CLI reports carries exactly one.
const (
	KindAuth           = "auth"
	KindForbidden      = "forbidden"
	KindNotFound       = "not_found"
	KindValidation     = "validation"
	KindConflict       = "conflict"
	KindRateLimited    = "rate_limited"
	KindServer         = "server"
	KindTransport      = "transport"
	KindOutcomeUnknown = "outcome_unknown"
	KindIncomplete     = "incomplete"
	KindUsage          = "usage"
	// KindOutputFailed: the call succeeded, but its response could not be
	// written where it was asked to go. The change, if any, has happened.
	KindOutputFailed = "output_failed"
)

// Error is the single error type the CLI reports, shaped for JSON output.
type Error struct {
	Kind    string `json:"kind"`
	Message string `json:"error"`
	Status  int    `json:"status,omitempty"`
	Details any    `json:"details"`
}

func (e *Error) Error() string { return e.Message }

// Usagef builds a caller error: something wrong before any request is sent.
func Usagef(format string, a ...any) *Error {
	return &Error{Kind: KindUsage, Message: fmt.Sprintf(format, a...)}
}

func kindForStatus(status int) string {
	switch {
	case status == 401:
		return KindAuth
	case status == 403:
		return KindForbidden
	case status == 404 || status == 410:
		return KindNotFound
	case status == 409:
		return KindConflict
	case status == 429:
		return KindRateLimited
	case status >= 500:
		return KindServer
	default:
		return KindValidation
	}
}
```

- [ ] **Step 4: Create `internal/api/twenty.go`**

```go
package api

import (
	"encoding/json"
	"fmt"
	"strings"
)

// failure is the error body Twenty's REST API sends,
// {"statusCode":400,"messages":["..."],"error":"BAD_REQUEST"}, or the NestJS
// default {"statusCode":404,"message":"...","error":"Not Found"}.
type failure struct {
	messages []string
	code     string
}

func parseFailure(body []byte) failure {
	var raw struct {
		Messages json.RawMessage `json:"messages"`
		Message  json.RawMessage `json:"message"`
		Error    json.RawMessage `json:"error"`
	}
	var f failure
	if json.Unmarshal(body, &raw) != nil {
		return f
	}
	f.messages = append(stringsOf(raw.Messages), stringsOf(raw.Message)...)
	if s := stringsOf(raw.Error); len(s) == 1 {
		f.code = s[0]
	}
	return f
}

// stringsOf reads a JSON string or an array of strings; anything else is nil.
func stringsOf(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var one string
	if json.Unmarshal(raw, &one) == nil {
		if one == "" {
			return nil
		}
		return []string{one}
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil {
		return many
	}
	return nil
}

// summary leads with Twenty's first message rather than the status text.
func (f failure) summary() string {
	switch {
	case len(f.messages) > 1:
		return fmt.Sprintf(": %s (and %d more)", f.messages[0], len(f.messages)-1)
	case len(f.messages) == 1:
		return ": " + f.messages[0]
	case f.code != "":
		return ": " + f.code
	}
	return ""
}

// hint names the likely fix for the errors people meet while setting up.
func hint(r Request, status int, f failure) string {
	text := strings.ToLower(strings.Join(f.messages, " "))
	switch {
	case status == 401 && strings.Contains(text, "expired"):
		return "; hint: the API key has expired: create a new one in Twenty under Settings, APIs & Webhooks, then run `twentycrm config set api-key`"
	case status == 401:
		return "; hint: the API key is invalid or revoked; `twentycrm auth status` shows which key is in use"
	case status == 403 && (strings.HasPrefix(r.Path, "rest/metadata/objects") || strings.HasPrefix(r.Path, "rest/metadata/fields")):
		return "; hint: objects and fields need the Data Model permission on the key's role; `twentycrm schema` works without it"
	case status == 403:
		return "; hint: the key's role does not allow this on this object (Twenty: Settings, Roles)"
	case status == 400 && r.Object != "" && strings.Contains(text, "field"):
		return fmt.Sprintf("; hint: run `twentycrm schema %s` for the field names", r.Object)
	case status == 429:
		return "; hint: Twenty allows 100 requests per minute"
	}
	return ""
}

func errorDetails(resp *Response) any {
	if len(resp.Body) == 0 {
		return nil
	}
	var v any
	if json.Unmarshal(resp.Body, &v) == nil {
		return v
	}
	return string(resp.Body)
}

// errorFor builds the error for a non-2xx response; extra goes between
// Twenty's message and the hint.
func errorFor(r Request, resp *Response, extra string) *Error {
	f := parseFailure(resp.Body)
	return &Error{
		Kind:    kindForStatus(resp.Status),
		Message: fmt.Sprintf("%s %s: HTTP %d%s%s%s", r.Method, r.Path, resp.Status, f.summary(), extra, hint(r, resp.Status, f)),
		Status:  resp.Status,
		Details: errorDetails(resp),
	}
}
```

- [ ] **Step 5: Create `internal/api/client.go`**

Start from `/Users/daniel/Development/google-ads-cli/internal/api/client.go` and reshape it to this (the retry loop keeps its structure; Google's `retryDelay`, the token source, `login-customer-id`, `developer-token`, credential query parameters, `AllowCustomBase` and `request-id` go away):

```go
// Package api is the HTTP client for Twenty's REST API. It owns the base URL
// guard, the headers, the retry policy and the error shape.
package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/config"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/routes"
)

// Client performs API requests. BaseURL and APIKey are required; Timeout,
// RetryBudget and MaxAttempts fall back to defaults.
type Client struct {
	BaseURL     string // scheme://host[:port], as config.NormalizeBaseURL returns it
	APIKey      string
	ReadOnly    bool
	Timeout     time.Duration // per attempt; default 30s
	RetryBudget time.Duration // total wait on 429; default 60s
	MaxAttempts int           // attempts for read-class transient failures; default 3
	Verbose     io.Writer     // nil = silent; never receives the key
	HTTP        *http.Client
	Sleep       func(time.Duration)
}

// Request is one API call.
type Request struct {
	Method string
	Path   string // relative to BaseURL: "rest/companies"
	Query  url.Values
	Body   []byte
	// Risk is the routes class the caller settled on. Every non-GET request
	// needs one. Only read-class requests are retried after a transient
	// failure, and only they pass read-only mode.
	Risk string
	// Object is the object's command name, for the schema hint; "" otherwise.
	Object string
	// Headers are applied after the defaults. Authorization is the client's
	// alone and cannot be set here.
	Headers http.Header
}

// Response is a successful (2xx) response.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
}

func (r Request) readSafe() bool { return r.Risk == routes.ClassRead }

func (c *Client) logf(format string, a ...any) {
	if c.Verbose != nil {
		fmt.Fprintf(c.Verbose, format+"\n", a...)
	}
}

// methodOverrideHeaders make some servers treat a request as another method,
// so a GET could be applied as a DELETE the guard never classified.
var methodOverrideHeaders = []string{"x-http-method-override", "x-http-method", "x-method-override"}

// headerIs compares header names the way proxies may: case-insensitively,
// with "_" and "-" as the same character.
func headerIs(k, name string) bool { return strings.EqualFold(strings.ReplaceAll(k, "_", "-"), name) }

// shape rejects a request that could change something the guard never saw.
func (r Request) shape() *Error {
	method := strings.ToUpper(r.Method)
	if method == "" {
		method = http.MethodGet
	}
	if r.Body != nil && (method == http.MethodGet || method == http.MethodDelete) {
		return Usagef("refusing to send a request body with %s %s: %s takes no body, so the guard never inspects one", method, r.Path, method)
	}
	if method != http.MethodGet && r.Risk == "" {
		return Usagef("refusing to send %s %s: no risk class was assigned, so the guard has not checked it", method, r.Path)
	}
	return nil
}

// guard rejects a request locally, before anything is sent.
func (c *Client) guard(r Request) *Error {
	if err := r.shape(); err != nil {
		return err
	}
	if c.BaseURL == "" {
		return Usagef("no Twenty base URL: set TWENTY_BASE_URL, run `twentycrm config set base-url <url>`, or run `twentycrm init`")
	}
	if c.APIKey == "" {
		return Usagef("no API key: set TWENTY_API_KEY, store one with `twentycrm config set api-key < key.txt`, or run `twentycrm init`")
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Host == "" {
		return Usagef("invalid base URL %q", c.BaseURL)
	}
	if u.Scheme != "https" && !config.IsLoopback(strings.ToLower(u.Hostname())) {
		return Usagef("refusing to send the API key over plain HTTP to %s; use https", u.Host)
	}
	if c.ReadOnly && !r.readSafe() {
		return Usagef("read-only mode (TWENTY_READ_ONLY or `config set read-only true`) blocks %s %s", r.Method, r.Path)
	}
	for k := range r.Headers {
		if headerIs(k, "authorization") {
			return Usagef("the Authorization header belongs to the client and cannot be set")
		}
		for _, name := range methodOverrideHeaders {
			if headerIs(k, name) {
				return Usagef("refusing to send the %s header: a method-override header could apply %s %s as another method the guard never checked", k, r.Method, r.Path)
			}
		}
	}
	return nil
}

// refuseRedirects never follows a redirect: Go keeps Authorization across a
// same-host https to http hop, and a 307/308 would replay a change.
func refuseRedirects(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

// attempt performs one round trip; Do wraps it with the retry loop.
func (c *Client) attempt(ctx context.Context, r Request) (*Response, error) {
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	u := strings.TrimRight(c.BaseURL, "/") + "/" + strings.TrimLeft(r.Path, "/")
	if len(r.Query) > 0 {
		u += "?" + r.Query.Encode()
	}
	var body io.Reader
	if r.Body != nil {
		body = bytes.NewReader(r.Body)
	}
	req, err := http.NewRequestWithContext(ctx, r.Method, u, body)
	if err != nil {
		return nil, &Error{Kind: KindUsage, Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Accept", "application/json")
	if r.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, vs := range r.Headers {
		req.Header[http.CanonicalHeaderKey(k)] = vs
	}
	c.logf("> %s %s", r.Method, r.Path)
	httpClient := &http.Client{}
	if c.HTTP != nil {
		cp := *c.HTTP
		httpClient = &cp
	}
	httpClient.CheckRedirect = refuseRedirects
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	c.logf("< %d (%d bytes)", resp.StatusCode, len(raw))
	return &Response{Status: resp.StatusCode, Header: resp.Header, Body: raw}, nil
}

// Do sends a request with the retry policy: 429 is retried for every call
// within the retry budget; network failures and 5xx are retried for
// read-class calls only. A change is never replayed. The error is always
// *Error.
func (c *Client) Do(ctx context.Context, r Request) (*Response, error) {
	if err := c.guard(r); err != nil {
		return nil, err
	}
	sleep := c.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	budget := c.RetryBudget
	if budget == 0 {
		budget = 60 * time.Second
	}
	maxAttempts := c.MaxAttempts
	if maxAttempts == 0 {
		maxAttempts = 3
	}
	var waited time.Duration
	transientTries := 0
	backoff := time.Second
	// pause waits before the next attempt. A context cancelled meanwhile
	// ends the call: another attempt would only fail, and for a change it
	// would turn a clean refusal into an unknown outcome.
	pause := func(d time.Duration) *Error {
		sleep(d)
		if err := ctx.Err(); err != nil {
			return &Error{Kind: KindTransport, Message: fmt.Sprintf("%s %s: cancelled while waiting to retry: %v", r.Method, r.Path, err)}
		}
		return nil
	}
	for {
		resp, err := c.attempt(ctx, r)
		if err != nil {
			var apiErr *Error
			if errors.As(err, &apiErr) {
				return nil, apiErr
			}
			e := classifyTransport(err, r.readSafe())
			if e.Kind == KindTransport && r.readSafe() {
				transientTries++
				if transientTries < maxAttempts {
					if e := pause(backoff); e != nil {
						return nil, e
					}
					backoff *= 2
					continue
				}
			}
			e.Message = fmt.Sprintf("%s %s: %s", r.Method, r.Path, e.Message)
			return nil, e
		}
		switch {
		case resp.Status >= 200 && resp.Status < 300:
			return resp, nil
		case resp.Status >= 300 && resp.Status < 400:
			return nil, &Error{
				Kind: KindValidation, Status: resp.Status, Details: errorDetails(resp),
				Message: fmt.Sprintf("%s %s: HTTP %d redirect to %q refused: twentycrm never follows redirects "+
					"(they can strip HTTPS off the key and replay a change); check the base URL",
					r.Method, r.Path, resp.Status, resp.Header.Get("Location")),
			}
		case resp.Status == http.StatusTooManyRequests:
			wait := max(retryAfter(resp.Header), backoff) + jitter()
			if waited+wait > budget {
				return nil, errorFor(r, resp, fmt.Sprintf("; Twenty asks to wait %s, beyond the %s retry budget", wait.Round(time.Second), budget))
			}
			waited += wait
			c.logf("* 429, waiting %s", wait)
			if e := pause(wait); e != nil {
				return nil, e
			}
			backoff *= 2
			continue
		case resp.Status >= 500 && r.readSafe():
			transientTries++
			if transientTries < maxAttempts {
				if e := pause(backoff); e != nil {
					return nil, e
				}
				backoff *= 2
				continue
			}
		}
		return nil, errorFor(r, resp, "")
	}
}
```

- [ ] **Step 6: Run the API tests to verify they pass**

Run: `go test ./internal/api/`
Expected: PASS.

- [ ] **Step 7: Create the fixture documents**

These are hand-built in the exact shape Twenty v2.27 generates (`open-api.service.ts`, `path.utils.ts`, `components.utils.ts`, `convert-object-metadata-to-schema-properties.util.ts`), trimmed to what extraction reads plus a few paths it must ignore. They use neutral names because the repository is public.

`internal/model/testdata/openapi-skeleton.json` (what Twenty returns to a caller whose token it does not accept):

```json
{
  "openapi": "3.1.1",
  "info": {"title": "Twenty Api", "description": "This is a **Twenty REST/API** playground.", "version": "v0.1"},
  "servers": [{"url": "https://crm.example.com/rest/", "description": "Production Development"}],
  "components": {"securitySchemes": {"bearerAuth": {"type": "http", "scheme": "bearer", "bearerFormat": "JWT"}}},
  "security": [{"bearerAuth": []}],
  "paths": {"/open-api/core": {"get": {"tags": ["General"], "summary": "Get Open Api Schema", "operationId": "GetOpenApiSchema"}}},
  "tags": [{"name": "General", "description": "General requests"}]
}
```

`internal/model/testdata/openapi-core.json`:

```json
{
  "openapi": "3.1.1",
  "info": {"title": "Twenty Api", "description": "This is a **Twenty REST/API** playground.", "version": "v0.1"},
  "servers": [{"url": "https://crm.example.com/rest/", "description": "Production Development"}],
  "security": [{"bearerAuth": []}],
  "tags": [
    {"name": "General", "description": "General requests"},
    {"name": "companies", "description": "A company"},
    {"name": "invoices", "description": "An invoice sent to a customer"},
    {"name": "noteTargets", "description": "A note target"},
    {"name": "people", "description": "A person"}
  ],
  "paths": {
    "/open-api/core": {"get": {"tags": ["General"], "summary": "Get Open Api Schema", "operationId": "GetOpenApiSchema"}},
    "/companies": {
      "get": {"tags": ["companies"], "summary": "Find Many companies", "operationId": "findManyCompanies"},
      "post": {"tags": ["companies"], "summary": "Create One company", "operationId": "createOneCompany"},
      "delete": {"tags": ["companies"], "summary": "Delete Many companies", "operationId": "deleteManyCompanies"},
      "patch": {"tags": ["companies"], "summary": "Update Many companies", "operationId": "updateManyCompanies"}
    },
    "/batch/companies": {"post": {"tags": ["companies"], "summary": "Create Many companies", "operationId": "createManyCompanies"}},
    "/companies/{id}": {
      "get": {"tags": ["companies"], "summary": "Find One company", "operationId": "findOneCompany"},
      "delete": {"tags": ["companies"], "summary": "Delete One company", "operationId": "deleteOneCompany"},
      "patch": {"tags": ["companies"], "summary": "Update One company", "operationId": "UpdateOneCompany"}
    },
    "/companies/duplicates": {"post": {"tags": ["companies"], "summary": "Find company Duplicates", "operationId": "findCompanyDuplicates"}},
    "/restore/companies/{id}": {"patch": {"tags": ["companies"], "summary": "Restore One company", "operationId": "restoreOneCompany"}},
    "/restore/companies": {"patch": {"tags": ["companies"], "summary": "Restore Many companies", "operationId": "restoreManyCompanies"}},
    "/companies/merge": {"patch": {"tags": ["companies"], "summary": "Merge Many companies", "operationId": "mergeManyCompanies"}},
    "/companies/groupBy": {"get": {"tags": ["companies"], "summary": "Group By companies", "operationId": "groupByCompanies"}},
    "/people": {
      "get": {"tags": ["people"], "summary": "Find Many people", "operationId": "findManyPeople"},
      "post": {"tags": ["people"], "summary": "Create One person", "operationId": "createOnePerson"}
    },
    "/batch/people": {"post": {"tags": ["people"], "summary": "Create Many people", "operationId": "createManyPeople"}},
    "/noteTargets": {
      "get": {"tags": ["noteTargets"], "summary": "Find Many noteTargets", "operationId": "findManyNoteTargets"},
      "post": {"tags": ["noteTargets"], "summary": "Create One noteTarget", "operationId": "createOneNoteTarget"}
    },
    "/invoices": {
      "get": {"tags": ["invoices"], "summary": "Find Many invoices", "operationId": "findManyInvoices"},
      "post": {"tags": ["invoices"], "summary": "Create One invoice", "operationId": "createOneInvoice"}
    },
    "/dashboards/{id}/duplicate": {"post": {"tags": ["dashboards"], "summary": "Duplicate a dashboard", "operationId": "duplicateDashboard"}}
  },
  "components": {
    "securitySchemes": {"bearerAuth": {"type": "http", "scheme": "bearer", "bearerFormat": "JWT"}},
    "schemas": {
      "Company": {
        "type": "object",
        "description": "A company",
        "properties": {
          "name": {"type": "string", "description": "The company name"},
          "domainName": {"type": "object", "properties": {"primaryLinkLabel": {"type": "string"}, "primaryLinkUrl": {"type": "string"},
            "secondaryLinks": {"type": "array", "items": {"type": "object", "properties": {"url": {"type": "string", "format": "uri"}, "label": {"type": "string"}}}}}},
          "employees": {"type": "integer", "description": "Number of employees in the company"},
          "stage": {"type": "string", "enum": ["LEAD", "CUSTOMER", "CHURNED"], "description": "Where the customer relationship stands"},
          "externalRef": {"type": "string"},
          "accountOwnerId": {"type": "string", "format": "uuid"},
          "position": {"type": "number"}
        },
        "required": ["name"],
        "example": {"name": "Company name"}
      },
      "CompanyForUpdate": {
        "type": "object",
        "description": "A company",
        "properties": {
          "name": {"type": "string", "description": "The company name"},
          "employees": {"type": "integer"},
          "stage": {"type": "string", "enum": ["LEAD", "CUSTOMER", "CHURNED"]},
          "externalRef": {"type": "string"},
          "accountOwnerId": {"type": "string", "format": "uuid"},
          "position": {"type": "number"}
        }
      },
      "CompanyForResponse": {
        "type": "object",
        "description": "A company",
        "properties": {
          "id": {"type": "string", "format": "uuid"},
          "createdAt": {"type": "string", "format": "date-time"},
          "updatedAt": {"type": "string", "format": "date-time"},
          "deletedAt": {"type": "string", "format": "date-time"},
          "name": {"type": "string", "description": "The company name"},
          "domainName": {"type": "object", "properties": {"primaryLinkLabel": {"type": "string"}, "primaryLinkUrl": {"type": "string"},
            "secondaryLinks": {"type": "array", "items": {"type": "object", "properties": {"url": {"type": "string", "format": "uri"}, "label": {"type": "string"}}}}}},
          "employees": {"type": "integer", "description": "Number of employees in the company"},
          "stage": {"type": "string", "enum": ["LEAD", "CUSTOMER", "CHURNED"], "description": "Where the customer relationship stands"},
          "externalRef": {"type": "string"},
          "accountOwnerId": {"type": "string", "format": "uuid"},
          "position": {"type": "number"},
          "people": {"type": "array", "items": {"$ref": "#/components/schemas/PersonForResponse"}, "description": "People linked to the company."},
          "accountOwner": {"type": "object", "oneOf": [{"$ref": "#/components/schemas/WorkspaceMemberForResponse"}]},
          "noteTargets": {"type": "array", "items": {"$ref": "#/components/schemas/NoteTargetForResponse"}},
          "invoices": {"type": "array", "items": {"$ref": "#/components/schemas/InvoiceForResponse"}}
        }
      },
      "Person": {
        "type": "object",
        "description": "A person",
        "properties": {
          "name": {"type": "object", "properties": {"firstName": {"type": "string"}, "lastName": {"type": "string"}}},
          "emails": {"type": "object", "properties": {"primaryEmail": {"type": "string"}, "additionalEmails": {"type": "array", "items": {"type": "string", "format": "email"}}}},
          "jobTitle": {"type": "string"},
          "city": {"type": "string"},
          "companyId": {"type": "string", "format": "uuid"},
          "position": {"type": "number"}
        }
      },
      "PersonForUpdate": {"type": "object", "description": "A person", "properties": {"jobTitle": {"type": "string"}, "companyId": {"type": "string", "format": "uuid"}}},
      "PersonForResponse": {
        "type": "object",
        "description": "A person",
        "properties": {
          "id": {"type": "string", "format": "uuid"},
          "createdAt": {"type": "string", "format": "date-time"},
          "name": {"type": "object", "properties": {"firstName": {"type": "string"}, "lastName": {"type": "string"}}},
          "emails": {"type": "object", "properties": {"primaryEmail": {"type": "string"}, "additionalEmails": {"type": "array", "items": {"type": "string", "format": "email"}}}},
          "jobTitle": {"type": "string"},
          "city": {"type": "string"},
          "companyId": {"type": "string", "format": "uuid"},
          "position": {"type": "number"},
          "company": {"type": "object", "oneOf": [{"$ref": "#/components/schemas/CompanyForResponse"}]},
          "noteTargets": {"type": "array", "items": {"$ref": "#/components/schemas/NoteTargetForResponse"}}
        }
      },
      "NoteTarget": {
        "type": "object",
        "description": "A note target",
        "properties": {
          "noteId": {"type": "string", "format": "uuid"},
          "personId": {"type": "string", "format": "uuid"},
          "companyId": {"type": "string", "format": "uuid"},
          "invoiceId": {"type": "string", "format": "uuid"}
        }
      },
      "NoteTargetForUpdate": {"type": "object", "description": "A note target", "properties": {"noteId": {"type": "string", "format": "uuid"}}},
      "NoteTargetForResponse": {
        "type": "object",
        "description": "A note target",
        "properties": {
          "id": {"type": "string", "format": "uuid"},
          "createdAt": {"type": "string", "format": "date-time"},
          "noteId": {"type": "string", "format": "uuid"},
          "personId": {"type": "string", "format": "uuid"},
          "companyId": {"type": "string", "format": "uuid"},
          "invoiceId": {"type": "string", "format": "uuid"},
          "note": {"type": "object", "oneOf": [{"$ref": "#/components/schemas/NoteForResponse"}]},
          "person": {"type": "object", "oneOf": [{"$ref": "#/components/schemas/PersonForResponse"}]},
          "company": {"type": "object", "oneOf": [{"$ref": "#/components/schemas/CompanyForResponse"}]},
          "invoice": {"type": "object", "oneOf": [{"$ref": "#/components/schemas/InvoiceForResponse"}]}
        }
      },
      "Invoice": {
        "type": "object",
        "description": "An invoice sent to a customer",
        "properties": {
          "name": {"type": "string"},
          "amount": {"type": "object", "properties": {"amountMicros": {"type": "number"}, "currencyCode": {"type": "string"}}},
          "status": {"type": "string", "enum": ["DRAFT", "SENT", "PAID"]},
          "tags": {"type": "array", "items": {"type": "string", "enum": ["URGENT", "RECURRING"]}},
          "dueDate": {"type": "string", "format": "date"},
          "companyId": {"type": "string", "format": "uuid"}
        },
        "required": ["name", "status"]
      },
      "InvoiceForUpdate": {"type": "object", "description": "An invoice sent to a customer", "properties": {"status": {"type": "string", "enum": ["DRAFT", "SENT", "PAID"]}}},
      "InvoiceForResponse": {
        "type": "object",
        "description": "An invoice sent to a customer",
        "properties": {
          "id": {"type": "string", "format": "uuid"},
          "createdAt": {"type": "string", "format": "date-time"},
          "name": {"type": "string"},
          "amount": {"type": "object", "properties": {"amountMicros": {"type": "number"}, "currencyCode": {"type": "string"}}},
          "status": {"type": "string", "enum": ["DRAFT", "SENT", "PAID"]},
          "tags": {"type": "array", "items": {"type": "string", "enum": ["URGENT", "RECURRING"]}},
          "dueDate": {"type": "string", "format": "date"},
          "companyId": {"type": "string", "format": "uuid"},
          "company": {"type": "object", "oneOf": [{"$ref": "#/components/schemas/CompanyForResponse"}]},
          "noteTargets": {"type": "array", "items": {"$ref": "#/components/schemas/NoteTargetForResponse"}}
        }
      }
    }
  },
  "webhooks": {}
}
```

- [ ] **Step 8: Write the failing model tests**

`internal/model/model_test.go`:

```go
package model

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func field(t *testing.T, o *Object, name string) Field {
	t.Helper()
	for _, f := range o.Fields {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("%s has no field %s", o.NamePlural, name)
	return Field{}
}

func TestCommandName(t *testing.T) {
	for in, want := range map[string]string{
		"companies": "companies", "noteTargets": "note-targets", "workspaceMembers": "workspace-members",
		"calendarChannelEventAssociations": "calendar-channel-event-associations", "invoices2024": "invoices2024", "person": "person",
	} {
		if got := CommandName(in); got != want {
			t.Errorf("CommandName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExtract(t *testing.T) {
	objs, err := Extract(fixture(t, "openapi-core.json"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, o := range objs {
		names = append(names, o.Command+"/"+o.NamePlural+"/"+o.NameSingular)
	}
	if got := strings.Join(names, " "); got != "companies/companies/company invoices/invoices/invoice note-targets/noteTargets/noteTarget people/people/person" {
		t.Fatalf("objects = %s", got)
	}
	m := &Model{Objects: objs}
	co := m.Find("companies")
	if co.Description != "A company" {
		t.Errorf("description = %q", co.Description)
	}
	for i := 1; i < len(co.Fields); i++ {
		if co.Fields[i-1].Name >= co.Fields[i].Name {
			t.Fatalf("fields not sorted: %s before %s", co.Fields[i-1].Name, co.Fields[i].Name)
		}
	}
	if f := field(t, co, "name"); !f.Required || f.ReadOnly || f.Type != "string" || f.Description != "The company name" {
		t.Errorf("name = %+v", f)
	}
	if f := field(t, co, "id"); !f.ReadOnly || f.Required || f.Format != "uuid" {
		t.Errorf("id = %+v", f)
	}
	if f := field(t, co, "createdAt"); !f.ReadOnly || f.Format != "date-time" {
		t.Errorf("createdAt = %+v", f)
	}
	if f := field(t, co, "stage"); strings.Join(f.Enum, ",") != "LEAD,CUSTOMER,CHURNED" || f.ReadOnly {
		t.Errorf("stage = %+v", f)
	}
	if f := field(t, co, "domainName"); f.Type != "object" || strings.Join(f.Subfields, ",") != "primaryLinkLabel,primaryLinkUrl,secondaryLinks" {
		t.Errorf("domainName = %+v", f)
	}
	if f := field(t, co, "accountOwnerId"); f.ReadOnly || f.Relation != nil || f.Format != "uuid" {
		t.Errorf("accountOwnerId = %+v", f)
	}
	if f := field(t, co, "people"); f.Relation == nil || *f.Relation != (Relation{Target: "people", Kind: "one_to_many"}) || f.ReadOnly {
		t.Errorf("people = %+v", f)
	}
	if f := field(t, co, "accountOwner"); f.Relation == nil || *f.Relation != (Relation{Target: "workspaceMember", Kind: "many_to_one"}) {
		t.Errorf("accountOwner (target outside the document) = %+v", f)
	}
	inv := m.Find("invoices")
	if f := field(t, inv, "status"); !f.Required || strings.Join(f.Enum, ",") != "DRAFT,SENT,PAID" {
		t.Errorf("status = %+v", f)
	}
	if f := field(t, inv, "tags"); f.Type != "array" || strings.Join(f.Enum, ",") != "URGENT,RECURRING" {
		t.Errorf("tags (multi-select) = %+v", f)
	}
	if f := field(t, inv, "company"); f.Relation == nil || *f.Relation != (Relation{Target: "companies", Kind: "many_to_one"}) {
		t.Errorf("company = %+v", f)
	}
	if f := field(t, m.Find("note-targets"), "note"); f.Relation == nil || f.Relation.Target != "note" {
		t.Errorf("note = %+v", f)
	}
}

func TestExtractSkeletonHasNoObjects(t *testing.T) {
	if _, err := Extract(fixture(t, "openapi-skeleton.json")); err != ErrNoObjects {
		t.Fatalf("err = %v, want ErrNoObjects", err)
	}
}

func TestExtractFailures(t *testing.T) {
	cases := map[string]struct{ doc, want string }{
		"not json":       {`{`, "not valid JSON"},
		"no paths":       {`{"openapi":"3.1.1"}`, "has no paths"},
		"missing schema": {`{"paths":{"/widgets":{"get":{"operationId":"findManyWidgets"},"post":{"operationId":"createOneWidget"}}},"components":{"schemas":{}}}`, "WidgetForResponse"},
	}
	for name, c := range cases {
		if _, err := Extract([]byte(c.doc)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want %q", name, err, c.want)
		}
	}
}

func TestModelFindMatchesAliases(t *testing.T) {
	objs, _ := Extract(fixture(t, "openapi-core.json"))
	m := &Model{Objects: objs}
	for _, name := range []string{"note-targets", "noteTargets", "note-target", "companies", "company", "people", "person"} {
		if m.Find(name) == nil {
			t.Errorf("Find(%q) = nil", name)
		}
	}
	for _, name := range []string{"notes", "Companies", ""} {
		if m.Find(name) != nil {
			t.Errorf("Find(%q) found something", name)
		}
	}
	var none *Model
	if none.Find("companies") != nil || !none.Stale(time.Now()) {
		t.Error("a nil model finds nothing and is stale")
	}
}

func TestStale(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	if (&Model{FetchedAt: now.Add(-23 * time.Hour)}).Stale(now) {
		t.Error("23 hours is fresh")
	}
	if !(&Model{FetchedAt: now.Add(-25 * time.Hour)}).Stale(now) {
		t.Error("25 hours is stale")
	}
}

func TestCache(t *testing.T) {
	root := t.TempDir()
	m := &Model{FetchedAt: time.Now().UTC().Truncate(time.Second), BaseURL: "https://a.example.com", WorkspaceID: "ws-1",
		Objects: []Object{{Command: "companies", NamePlural: "companies", NameSingular: "company"}}}
	if got := LoadCache(root, m.BaseURL, m.WorkspaceID); got != nil {
		t.Fatal("a missing cache loads as nil")
	}
	if err := SaveCache(root, m); err != nil {
		t.Fatal(err)
	}
	got := LoadCache(root, m.BaseURL, m.WorkspaceID)
	if got == nil || !got.FetchedAt.Equal(m.FetchedAt) || got.Find("companies") == nil {
		t.Fatalf("LoadCache = %+v", got)
	}
	p := CachePath(root, m.BaseURL, m.WorkspaceID)
	if !strings.HasPrefix(filepath.Base(p), "model-") || len(filepath.Base(p)) != len("model-")+16+len(".json") {
		t.Errorf("cache file name = %s", filepath.Base(p))
	}
	if CachePath(root, "https://b.example.com", "ws-1") == p || CachePath(root, m.BaseURL, "ws-2") == p {
		t.Error("the cache must be keyed by base URL and workspace")
	}
	if LoadCache(root, "https://b.example.com", "ws-1") != nil {
		t.Error("another base URL must not load this cache")
	}
	if runtime.GOOS != "windows" {
		fi, _ := os.Stat(p)
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("cache mode = %v", fi.Mode().Perm())
		}
	}
	os.WriteFile(p, []byte("{broken"), 0o600)
	if LoadCache(root, m.BaseURL, m.WorkspaceID) != nil {
		t.Error("a corrupt cache loads as nil")
	}
	if LoadCache("", m.BaseURL, m.WorkspaceID) != nil || LoadCache(root, "", "ws-1") != nil {
		t.Error("no root or no base URL means no cache")
	}
}
```

- [ ] **Step 9: Run the model tests to verify they fail**

Run: `go test ./internal/model/`
Expected: FAIL (build error: undefined `Extract`, `Model`, ...).

- [ ] **Step 10: Create `internal/model/model.go`**

```go
// Package model is what the CLI knows about one Twenty workspace: its objects
// and their fields. It is read from the workspace's OpenAPI document and
// cached, so the command tree can be built without a network call.
package model

import (
	"strings"
	"time"
	"unicode"
)

// MaxAge is how long a cached model is trusted before it is refreshed.
const MaxAge = 24 * time.Hour

// Relation says where a relation field points.
type Relation struct {
	Target string `json:"target"` // the target object's plural name
	Kind   string `json:"kind"`   // "many_to_one" or "one_to_many"
}

// Field is one field of an object, as far as an agent needs it.
type Field struct {
	Name        string    `json:"name"`
	Type        string    `json:"type,omitempty"`
	Format      string    `json:"format,omitempty"`
	Enum        []string  `json:"enum,omitempty"`      // select and multi-select values
	Subfields   []string  `json:"subfields,omitempty"` // composite fields: emails.primaryEmail and the like
	Required    bool      `json:"required,omitempty"`  // needed on create
	ReadOnly    bool      `json:"read_only,omitempty"` // set by Twenty, not by the caller
	Relation    *Relation `json:"relation,omitempty"`
	Description string    `json:"description,omitempty"`
}

// Object is one object of the workspace.
type Object struct {
	Command      string  `json:"command"` // kebab-case plural: the CLI command
	NamePlural   string  `json:"name_plural"`
	NameSingular string  `json:"name_singular"`
	Description  string  `json:"description,omitempty"`
	Fields       []Field `json:"fields"`
}

// Model is the cached description of one workspace.
type Model struct {
	FetchedAt   time.Time `json:"fetched_at"`
	BaseURL     string    `json:"base_url"`
	WorkspaceID string    `json:"workspace_id"`
	Objects     []Object  `json:"objects"`
}

// Stale reports whether the model is missing or older than MaxAge.
func (m *Model) Stale(now time.Time) bool {
	return m == nil || now.Sub(m.FetchedAt) > MaxAge
}

// Find returns the object whose command, plural name or kebab-case singular
// is name, or nil.
func (m *Model) Find(name string) *Object {
	if m == nil || name == "" {
		return nil
	}
	for i := range m.Objects {
		o := &m.Objects[i]
		if o.Command == name || o.NamePlural == name || CommandName(o.NameSingular) == name {
			return o
		}
	}
	return nil
}

// CommandName turns an API name into a command: "noteTargets" becomes
// "note-targets".
func CommandName(apiName string) string {
	var b strings.Builder
	for i, r := range apiName {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
```

- [ ] **Step 11: Create `internal/model/extract.go`**

```go
package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ErrNoObjects means the document describes no object. Twenty serves an
// empty document, with status 200, to a caller whose token it does not
// accept, so this is almost always a key problem.
var ErrNoObjects = errors.New("the workspace's OpenAPI document lists no objects; Twenty sends an empty document " +
	"when it does not accept the API key, so check `twentycrm auth status` and the key in Twenty (Settings, APIs & Webhooks)")

type schema struct {
	Type        any               `json:"type"` // a string, or an array of strings in OpenAPI 3.1
	Format      string            `json:"format"`
	Enum        []any             `json:"enum"`
	Items       *schema           `json:"items"`
	Ref         string            `json:"$ref"`
	OneOf       []schema          `json:"oneOf"`
	Properties  map[string]schema `json:"properties"`
	Required    []string          `json:"required"`
	Description string            `json:"description"`
}

type document struct {
	Paths map[string]map[string]json.RawMessage `json:"paths"`
	Tags  []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"tags"`
	Components struct {
		Schemas map[string]schema `json:"schemas"`
	} `json:"components"`
}

// Extract reads the objects and fields out of GET /rest/open-api/core
// (spec: Workspace model). The result is sorted by command, each object's
// fields by name.
func Extract(raw []byte) ([]Object, error) {
	var d document
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("the workspace's OpenAPI document is not valid JSON: %v", err)
	}
	if d.Paths == nil {
		return nil, errors.New("the workspace's OpenAPI document has no paths")
	}
	tags := map[string]string{}
	for _, t := range d.Tags {
		tags[t.Name] = t.Description
	}
	type found struct{ plural, singular string }
	var objs []found
	for path, item := range d.Paths {
		plural, ok := strings.CutPrefix(path, "/")
		if !ok || plural == "" || strings.Contains(plural, "/") {
			continue
		}
		if operationID(item, "get") != "findMany"+upperFirst(plural) {
			continue
		}
		singular, ok := strings.CutPrefix(operationID(item, "post"), "createOne")
		if !ok || singular == "" {
			continue
		}
		objs = append(objs, found{plural, lowerFirst(singular)})
	}
	if len(objs) == 0 {
		return nil, ErrNoObjects
	}
	pluralOf := map[string]string{} // "Company" -> "companies"
	for _, o := range objs {
		pluralOf[upperFirst(o.singular)] = o.plural
	}
	out := make([]Object, 0, len(objs))
	for _, o := range objs {
		name := upperFirst(o.singular)
		create, okCreate := d.Components.Schemas[name]
		resp, okResp := d.Components.Schemas[name+"ForResponse"]
		if !okCreate || !okResp {
			return nil, fmt.Errorf("the workspace's OpenAPI document has no %s or %sForResponse schema for %s", name, name, o.plural)
		}
		required := map[string]bool{}
		for _, r := range create.Required {
			required[r] = true
		}
		obj := Object{Command: CommandName(o.plural), NamePlural: o.plural, NameSingular: o.singular, Description: tags[o.plural]}
		for fname, p := range resp.Properties {
			f := Field{Name: fname, Type: typeName(p.Type), Description: p.Description, Required: required[fname]}
			if rel := relationOf(p, pluralOf); rel != nil {
				f.Relation = rel
			} else {
				_, inCreate := create.Properties[fname]
				f.ReadOnly = !inCreate
				f.Format = p.Format
				f.Enum = enumOf(p)
				for sub := range p.Properties {
					f.Subfields = append(f.Subfields, sub)
				}
				sort.Strings(f.Subfields)
			}
			obj.Fields = append(obj.Fields, f)
		}
		sort.Slice(obj.Fields, func(i, j int) bool { return obj.Fields[i].Name < obj.Fields[j].Name })
		out = append(out, obj)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Command < out[j].Command })
	return out, nil
}

func operationID(item map[string]json.RawMessage, method string) string {
	var op struct {
		OperationID string `json:"operationId"`
	}
	if raw, ok := item[method]; ok {
		json.Unmarshal(raw, &op)
	}
	return op.OperationID
}

// relationOf recognises Twenty's two relation shapes: an object whose oneOf
// references <Target>ForResponse (many to one), and an array of them (one to
// many). A target outside the document keeps its singular name.
func relationOf(p schema, pluralOf map[string]string) *Relation {
	target := func(ref string) string {
		name := strings.TrimSuffix(strings.TrimPrefix(ref, "#/components/schemas/"), "ForResponse")
		if plural, ok := pluralOf[name]; ok {
			return plural
		}
		return lowerFirst(name)
	}
	if len(p.OneOf) > 0 && strings.HasSuffix(p.OneOf[0].Ref, "ForResponse") {
		return &Relation{Target: target(p.OneOf[0].Ref), Kind: "many_to_one"}
	}
	if p.Items != nil && strings.HasSuffix(p.Items.Ref, "ForResponse") {
		return &Relation{Target: target(p.Items.Ref), Kind: "one_to_many"}
	}
	return nil
}

func enumOf(p schema) []string {
	values := p.Enum
	if len(values) == 0 && p.Items != nil {
		values = p.Items.Enum
	}
	var out []string
	for _, v := range values {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func typeName(t any) string {
	switch v := t.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, x := range v {
			if s, ok := x.(string); ok && s != "null" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ",")
	}
	return ""
}

func upperFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[n:]
}

func lowerFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToLower(r)) + s[n:]
}
```

- [ ] **Step 12: Create `internal/model/cache.go`**

```go
package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
)

// CachePath is <root>/twentycrm/model-<id>.json, where id is the first 16 hex
// characters of the SHA-256 of "<base URL>|<workspace ID>". root is the
// user's cache directory.
func CachePath(root, baseURL, workspaceID string) string {
	sum := sha256.Sum256([]byte(baseURL + "|" + workspaceID))
	return filepath.Join(root, "twentycrm", "model-"+hex.EncodeToString(sum[:])[:16]+".json")
}

// LoadCache returns the cached model, or nil when there is none, when it
// cannot be read, or when it belongs to another base URL or workspace. A
// broken cache is simply refetched.
func LoadCache(root, baseURL, workspaceID string) *Model {
	if root == "" || baseURL == "" {
		return nil
	}
	raw, err := os.ReadFile(CachePath(root, baseURL, workspaceID))
	if err != nil {
		return nil
	}
	var m Model
	if json.Unmarshal(raw, &m) != nil || m.BaseURL != baseURL || m.WorkspaceID != workspaceID || len(m.Objects) == 0 {
		return nil
	}
	return &m
}

// SaveCache writes m through a temp file and a rename, 0600 in a 0700
// directory.
func SaveCache(root string, m *Model) error {
	p := CachePath(root, m.BaseURL, m.WorkspaceID)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".model-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p)
}
```

- [ ] **Step 13: Run all tests and vet**

Run: `go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 14: Commit**

```bash
git add internal/api internal/model
git commit -m "Add the API client and the workspace model

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---
### Task 3: CLI core: model refresh, object and metadata commands, paging, help

The binary becomes usable: `twentycrm companies list`, every verb of the table, `metadata`, `--all`, per-object help and `version`. The remaining built-in commands arrive in Task 4.

**Files:**
- Create: `cmd/twentycrm/main.go`
- Create: `internal/cli/app.go`, `internal/cli/refresh.go`, `internal/cli/root.go`, `internal/cli/util.go`, `internal/cli/version.go`
- Create: `internal/cli/objects.go`, `internal/cli/metadata.go`, `internal/cli/all.go`, `internal/cli/help.go`
- Create (copied): `internal/cli/body.go`, `internal/cli/output.go`
- Test: `internal/cli/app_test.go`, `internal/cli/objects_test.go`, `internal/cli/refresh_test.go`, `internal/cli/all_test.go`, `internal/cli/metadata_test.go`, `internal/cli/help_test.go`

**Interfaces:**
- Consumes (Tasks 1 and 2): `config.Resolve`, `config.Resolved` and its `BindingError`; `auth.Decode`; `routes.ObjectVerbs`, `routes.MetadataVerbs`, `routes.MetadataKinds`, `routes.MetadataBlocked`, `routes.ValidateID`, `routes.CheckBody`, `routes.Decision`, `routes.NeedsForce`, `routes.ClassRead`, `routes.BodyKind` constants, `routes.MaxBatch`, `(routes.Verb).Path`; `api.Client`, `api.Request`, `api.Response`, `api.Error`, `api.Usagef`, the `api.Kind*` constants; `model.Model`, `model.Object`, `model.Field`, `model.Extract`, `model.ErrNoObjects`, `model.LoadCache`, `model.SaveCache`, `model.CommandName`.
- Produces (Task 4 relies on these):
  - `type app struct { model *model.Model; client *api.Client; res config.Resolved; configErr error; stdout, stderr io.Writer; stdin io.Reader; cacheDir func() (string, error); now func() time.Time }`
  - `Execute(args []string) int`; `(*app).configure(getenv func(string) string, sleep func(time.Duration))`; `(*app).run(args []string) int`; `(*app).runContext(ctx, args) int`; `(*app).renderError(err error) int`
  - `(*app).clock() time.Time`, `(*app).cacheRoot() string`, `(*app).callable() error`, `(*app).workspaceID() string`
  - `(*app).fetchModel(ctx context.Context) (*model.Model, error)` (fetches, extracts, saves the cache, returns the model)
  - `builtinNames []string`, `isBuiltin(name string) bool`
  - `(*app).newRoot() *cobra.Command` (Task 4 adds its commands to the `root.AddCommand` call)
  - `(*app).send(cmd *cobra.Command, req api.Request, p pager) error`; `pager{rowsKey string; pageSize int}`
  - `(*app).readJSONBody(cmd *cobra.Command) ([]byte, error)`, `openOutput(path string) (*outputFile, error)`, `(*app).writeResponse(resp *api.Response, out *outputFile) error`, `(*app).applyGlobalFlags(cmd *cobra.Command)`
  - `flagBool(cmd, name) bool`, `flagString(cmd, name) string`, `groupRunE`, `jsonCompact(v any) ([]byte, error)`, `objectAliases(o *model.Object) []string`
  - `Version` (string var), `TestedTwentyVersion = "2.27"`
  - Test helpers in `app_test.go`: `newFakeTwenty(t) *fakeTwenty` (fields `openAPI []byte`, `handle func(w http.ResponseWriter, r *http.Request, body []byte)`, `reqs []recorded`; methods `calls() []recorded`, `fetches() int`), `recorded{Method, Path, RawQuery, Body string; Query url.Values}`, `readFixture(t, name string) []byte`, `testToken(t, payload map[string]any) string`, `testKey(t, workspace string, exp time.Time) string`, `newTestApp(t, srv, edit func(*config.Resolved)) (*app, *bytes.Buffer, *bytes.Buffer)`, `errLine(t, stderr string) api.Error`, `seedCache(t, a *app, age time.Duration, commands ...string)`

- [ ] **Step 1: Add the cobra dependency and copy the unchanged helpers**

Run: `go get github.com/spf13/cobra@v1.10.2`

Copy `/Users/daniel/Development/google-ads-cli/internal/cli/body.go` and `/Users/daniel/Development/google-ads-cli/internal/cli/output.go` into `internal/cli/`. In both, change the import `github.com/wir-drei-digital/google-ads-cli/internal/api` to `github.com/wir-drei-digital/twenty-crm-cli/internal/api`. In `output.go`, change "as Google sent it" to "as Twenty sent it" in the comment of `writeResponse`. Nothing else changes.

`cmd/twentycrm/main.go`:

```go
// Command twentycrm is the CLI entry point: it hands the process arguments
// to the cli package and turns its result into an exit code.
package main

import (
	"os"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/cli"
)

func main() { os.Exit(cli.Execute(os.Args[1:])) }
```

- [ ] **Step 2: Write the test helpers**

`internal/cli/app_test.go`:

```go
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
```

`seedCache` uses `contains`, which `objects.go` defines in Step 9; the helpers compile once that step is done.

- [ ] **Step 3: Write the failing object command tests**

`internal/cli/objects_test.go`:

```go
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
```

- [ ] **Step 4: Write the failing refresh, paging, metadata, help and version tests**

`internal/cli/refresh_test.go`:

```go
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
		"companies list":                    "companies",
		"--verbose companies list":          "companies",
		"--timeout 5s companies list":       "companies",
		"--timeout=5s people list":          "people",
		"--output x.json people list":       "people",
		"-- companies":                      "companies",
		"":                                  "",
		"--force":                           "",
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
```

`internal/cli/all_test.go`:

```go
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
		`{"data":{"objects":[{"id":"b"}]},"pageInfo":{"hasNextPage":false}}`:   `[{"id":"b"}]`,
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
```

`internal/cli/metadata_test.go`:

```go
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
```

`internal/cli/help_test.go`:

```go
package cli

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestObjectHelp(t *testing.T) {
	srv := newFakeTwenty(t)
	a, stdout, stderr := newTestApp(t, srv, nil)
	seedCache(t, a, time.Hour)
	if code := a.run([]string{"companies", "create", "--help"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	out := stdout.String()
	for _, want := range []string{"Request: POST /rest/companies", "Class: write", "stage", "one of: LEAD, CUSTOMER, CHURNED",
		"subfields: primaryLinkLabel, primaryLinkUrl, secondaryLinks", "one_to_many -> people", "twentycrm schema companies"} {
		if !strings.Contains(out, want) {
			t.Errorf("help lacks %q:\n%s", want, out)
		}
	}
	if !regexp.MustCompile(`(?m)^\s*name\s+string\s+required`).MatchString(out) {
		t.Errorf("name is not marked required:\n%s", out)
	}
	stdout.Reset()
	a.run([]string{"companies", "destroy", "--help"})
	if out := stdout.String(); !strings.Contains(out, "soft_delete=false") || !strings.Contains(out, "needs --force") {
		t.Errorf("destroy help:\n%s", out)
	}
	stdout.Reset()
	a.run([]string{"companies", "list", "--help"})
	if out := stdout.String(); !strings.Contains(out, "Filter syntax") || !strings.Contains(out, "ilike") || !strings.Contains(out, "DescNullsLast") {
		t.Errorf("list help:\n%s", out)
	}
}

func TestVersion(t *testing.T) {
	srv := newFakeTwenty(t)
	a, stdout, _ := newTestApp(t, srv, nil)
	if code := a.run([]string{"version"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if out := stdout.String(); !strings.HasPrefix(out, "twentycrm ") || !strings.Contains(out, "tested against Twenty 2.27") {
		t.Fatalf("version = %q", out)
	}
}
```

- [ ] **Step 5: Run the CLI tests to verify they fail**

Run: `go test ./internal/cli/`
Expected: FAIL (build error: undefined `app`, `firstWord`, ...).

- [ ] **Step 6: Implement `internal/cli/app.go`**

```go
// Package cli builds the twentycrm command tree from the workspace model and
// turns process arguments into one API call, or one merged page walk.
//
// Two rules shape everything here: stdout carries the API response and
// nothing else, and every failure leaves as one line of JSON on stderr so an
// agent can parse it without heuristics.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/api"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/config"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/model"
)

type app struct {
	model  *model.Model
	client *api.Client
	res    config.Resolved
	// configErr is why the configuration could not be resolved. Only
	// version, help and the config commands run without it, so a person can
	// find and repair the file; every other command reports it.
	configErr      error
	stdout, stderr io.Writer
	stdin          io.Reader
	// cacheDir locates the user cache directory; nil means os.UserCacheDir.
	cacheDir func() (string, error)
	// now is the clock for cache ages and key expiry; nil means time.Now.
	now func() time.Time
}

// Execute is the process entry point: it resolves the configuration, runs
// the command tree and returns the exit code (0 success, 1 API or network
// failure, 2 usage error).
func Execute(args []string) int {
	// Ctrl-C and SIGTERM cancel the request in flight and any retry wait.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	a := &app{stdout: os.Stdout, stderr: os.Stderr, stdin: os.Stdin}
	a.configure(os.Getenv, contextSleeper(ctx))
	return a.runContext(ctx, args)
}

// configure resolves the config file and the environment and builds the
// client. A failure is kept in configErr rather than returned.
func (a *app) configure(getenv func(string) string, sleep func(time.Duration)) {
	a.res, a.configErr = config.Resolve(getenv)
	a.client = &api.Client{BaseURL: a.res.BaseURL, APIKey: a.res.APIKey, ReadOnly: a.res.ReadOnly, Sleep: sleep}
}

func (a *app) clock() time.Time {
	if a.now != nil {
		return a.now()
	}
	return time.Now()
}

// cacheRoot is the user cache directory, or "" when there is none; the model
// cache is then skipped.
func (a *app) cacheRoot() string {
	dir := a.cacheDir
	if dir == nil {
		dir = os.UserCacheDir
	}
	root, err := dir()
	if err != nil {
		return ""
	}
	return root
}

// callable is the check every API call passes first: a key from the config
// file only goes to the base URL stored with it. Missing values are left to
// the client, which names the fix.
func (a *app) callable() error {
	if err := a.res.BindingError(); err != nil {
		return api.Usagef("%v", err)
	}
	return nil
}

// requireConfig is the root's persistent pre-run: with an unresolvable
// configuration only version, help and the config commands run.
func (a *app) requireConfig(cmd *cobra.Command, args []string) error {
	if a.configErr == nil {
		return nil
	}
	for c := cmd; c.HasParent(); c = c.Parent() {
		if !c.Parent().HasParent() {
			switch c.Name() {
			case "version", "config", "help":
				return nil
			}
		}
	}
	return api.Usagef("cannot load the configuration: %v", a.configErr)
}

// contextSleeper returns a sleep that gives up as soon as ctx is done.
func contextSleeper(ctx context.Context) func(time.Duration) {
	return func(d time.Duration) {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
		case <-t.C:
		}
	}
}

func (a *app) run(args []string) int { return a.runContext(context.Background(), args) }

func (a *app) runContext(ctx context.Context, args []string) int {
	if w := firstWord(args); a.configErr != nil && w != "" && !isBuiltin(w) {
		// Without a configuration there are no object commands; saying
		// "unknown command" would hide the real problem.
		return a.renderError(api.Usagef("cannot load the configuration: %v", a.configErr))
	}
	if a.configErr == nil {
		if err := a.prepareModel(ctx, args); err != nil {
			return a.renderError(err)
		}
	}
	root := a.newRoot()
	root.SetArgs(args)
	root.SetOut(a.stdout)
	root.SetErr(a.stderr)
	if err := root.ExecuteContext(ctx); err != nil {
		return a.renderError(err)
	}
	return 0
}

// renderError writes err as one line of JSON on stderr and returns the exit
// code: 2 for usage errors, 1 for everything the API or the network produced.
func (a *app) renderError(err error) int {
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		apiErr = api.Usagef("%v", err) // anything else is a cobra parse or usage failure
	}
	raw, mErr := jsonCompact(apiErr)
	if mErr != nil {
		raw, mErr = jsonCompact(&api.Error{Kind: apiErr.Kind, Message: apiErr.Message, Status: apiErr.Status})
		if mErr != nil {
			raw = []byte(`{"kind":"usage","error":"internal: cannot render error","details":null}`)
		}
	}
	fmt.Fprintln(a.stderr, string(raw))
	if apiErr.Kind == api.KindUsage {
		return 2
	}
	return 1
}
```

- [ ] **Step 7: Implement `internal/cli/refresh.go`**

```go
package cli

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/api"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/auth"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/model"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/routes"
)

// builtinNames are the top-level commands that are not objects. An object
// with one of these names is shadowed and stays reachable through `api`.
var builtinNames = []string{"version", "commands", "schema", "api", "config", "auth", "init", "metadata", "help", "completion"}

func isBuiltin(name string) bool {
	return slices.Contains(builtinNames, name) || strings.HasPrefix(name, "__complete")
}

// valueFlags are the global flags whose value may come as the next argument.
var valueFlags = map[string]bool{"--timeout": true, "--output": true}

// firstWord is the command's first argument that is not a flag.
func firstWord(args []string) string {
	for i := 0; i < len(args); i++ {
		s := args[i]
		switch {
		case s == "--":
			if i+1 < len(args) {
				return args[i+1]
			}
			return ""
		case strings.HasPrefix(s, "-"):
			if valueFlags[s] {
				i++
			}
		default:
			return s
		}
	}
	return ""
}

// workspaceID is the workspace named in the API key, or "" when the key
// cannot be read. It keys the model cache.
func (a *app) workspaceID() string {
	k, err := auth.Decode(a.res.APIKey)
	if err != nil {
		return ""
	}
	return k.WorkspaceID
}

// prepareModel loads the cached model and refreshes it when the command
// names an object the cache cannot vouch for (spec: Workspace model, Cache).
// It fails only when the command cannot be resolved without the refresh that
// just failed.
func (a *app) prepareModel(ctx context.Context, args []string) error {
	a.model = model.LoadCache(a.cacheRoot(), a.res.BaseURL, a.workspaceID())
	w := firstWord(args)
	if w == "" || isBuiltin(w) {
		return nil
	}
	if !a.model.Stale(a.clock()) && a.model.Find(w) != nil {
		return nil
	}
	fresh, err := a.fetchModel(ctx)
	if err == nil {
		a.model = fresh
		return nil
	}
	if a.model.Find(w) != nil {
		return nil // stale but usable; the call itself reports an outage
	}
	return err
}

// fetchModel reads the workspace's OpenAPI document, extracts the model and
// caches it. Saving is best effort: a read-only home must not break a call.
func (a *app) fetchModel(ctx context.Context) (*model.Model, error) {
	if err := a.callable(); err != nil {
		return nil, err
	}
	resp, err := a.client.Do(ctx, api.Request{Method: "GET", Path: "rest/open-api/core", Risk: routes.ClassRead})
	if err != nil {
		return nil, err
	}
	objs, err := model.Extract(resp.Body)
	if errors.Is(err, model.ErrNoObjects) {
		return nil, &api.Error{Kind: api.KindAuth, Message: "GET rest/open-api/core: " + err.Error()}
	}
	if err != nil {
		return nil, &api.Error{Kind: api.KindServer, Message: "GET rest/open-api/core: " + err.Error()}
	}
	m := &model.Model{FetchedAt: a.clock().UTC(), BaseURL: a.res.BaseURL, WorkspaceID: a.workspaceID(), Objects: objs}
	if root := a.cacheRoot(); root != "" {
		_ = model.SaveCache(root, m)
	}
	return m, nil
}
```

- [ ] **Step 8: Implement `internal/cli/util.go`, `internal/cli/root.go` and `internal/cli/version.go`**

`internal/cli/util.go`:

```go
package cli

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/api"
)

func flagBool(cmd *cobra.Command, name string) bool {
	v, _ := cmd.Flags().GetBool(name)
	return v
}

func flagString(cmd *cobra.Command, name string) string {
	v, _ := cmd.Flags().GetString(name)
	return v
}

// groupRunE answers a namespace invoked without a subcommand: that is a
// usage error, not a success that prints help.
func groupRunE(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return api.Usagef("unknown command %q for %q", args[0], cmd.CommandPath())
	}
	var names []string
	for _, sub := range cmd.Commands() {
		if !sub.Hidden && sub.Name() != "help" && sub.Name() != "completion" {
			names = append(names, sub.Name())
		}
	}
	if len(names) > 12 {
		names = append(names[:12], "...")
	}
	return api.Usagef("%s needs a subcommand: %s", cmd.CommandPath(), strings.Join(names, ", "))
}

// jsonCompact encodes v without HTML escaping and without a trailing newline.
func jsonCompact(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// setJSONField sets one top-level field of a JSON object body and keeps every
// other field's bytes as they were.
func setJSONField(body []byte, key string, value any) ([]byte, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, api.Usagef("--data must be a JSON object: %v", err)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, api.Usagef("%v", err)
	}
	m[key] = raw
	return jsonCompact(m)
}

func cloneValues(v url.Values) url.Values {
	out := url.Values{}
	for k, vs := range v {
		out[k] = append([]string(nil), vs...)
	}
	return out
}
```

`internal/cli/root.go`:

```go
package cli

import (
	"time"

	"github.com/spf13/cobra"
)

func (a *app) newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "twentycrm",
		Short: "CLI for the Twenty CRM API, built for agents: JSON in, JSON out",
		Long: "CLI for the Twenty CRM API, built for agents: JSON in, JSON out.\n\n" +
			"Every object of the connected workspace is a command: twentycrm companies list,\n" +
			"twentycrm people get <id>, twentycrm note-targets create --data @link.json.\n\n" +
			"stdout carries the API response and nothing else; errors are one line of JSON on stderr.\n" +
			"Exit codes: 0 success, 1 API or network error, 2 usage error.\n\n" +
			"Run `twentycrm schema` for the workspace's objects, `twentycrm schema <object>` for the\n" +
			"fields of one, and `twentycrm commands --json` for the machine-readable catalog.",
		// The root names no call. Printing help and exiting 0 would read as
		// success and break the stdout contract, so it is a usage error.
		RunE:              groupRunE,
		PersistentPreRunE: a.requireConfig,
		SilenceErrors:     true,
		SilenceUsage:      true,
	}
	pf := root.PersistentFlags()
	pf.Bool("verbose", false, "log requests to stderr (the API key is never logged)")
	pf.Duration("timeout", 30*time.Second, "per-attempt HTTP timeout")
	pf.Bool("force", false, "confirm a bulk-, destroy- or admin-class call")
	pf.String("output", "", "write the response body to a file instead of stdout")
	root.AddCommand(a.versionCommand(), a.metadataCommand())
	a.addObjectCommands(root)
	return root
}
```

`internal/cli/version.go`: copy `/Users/daniel/Development/google-ads-cli/internal/cli/version.go` and change it to:

```go
package cli

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// Version is the released version, set with -ldflags at build time.
var Version = "dev"

// TestedTwentyVersion is the Twenty release this build was checked against.
const TestedTwentyVersion = "2.27"

// resolveVersion prefers the linker-set version. A build without one, such
// as `go install .../cmd/twentycrm@v0.1.0`, falls back to the module version
// Go recorded; "(devel)" stays "dev".
func resolveVersion(linker string, info *debug.BuildInfo, ok bool) string {
	if linker != "dev" || !ok || info == nil {
		return linker
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	return linker
}

// commitOf returns the short VCS revision Go recorded, or "".
func commitOf(info *debug.BuildInfo, ok bool) string {
	if !ok || info == nil {
		return ""
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" && len(s.Value) >= 7 {
			return s.Value[:7]
		}
	}
	return ""
}

func (a *app) versionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the CLI version and the Twenty version it was tested against",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			info, ok := debug.ReadBuildInfo()
			commit := ""
			if c := commitOf(info, ok); c != "" {
				commit = "commit " + c + ", "
			}
			fmt.Fprintf(a.stdout, "twentycrm %s (%stested against Twenty %s)\n", resolveVersion(Version, info, ok), commit, TestedTwentyVersion)
		},
	}
}
```

- [ ] **Step 9: Implement `internal/cli/objects.go`**

```go
package cli

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/api"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/model"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/routes"
)

// flagSpec is one verb flag: its type, the query parameter it sets ("" for
// flags the CLI consumes itself) and its help.
type flagSpec struct {
	kind  string // "string", "int" or "bool"
	query string
	usage string
}

var verbFlags = map[string]flagSpec{
	"filter":                 {"string", "filter", `Twenty filter, e.g. name[ilike]:"%acme%" (see --help)`},
	"order-by":               {"string", "order_by", "sort order, e.g. createdAt[DescNullsLast],name"},
	"limit":                  {"int", "limit", "records per page"},
	"depth":                  {"int", "depth", "0: the record only (Twenty's default); 1: with its direct relations"},
	"starting-after":         {"string", "starting_after", "cursor: start after this pageInfo.endCursor"},
	"ending-before":          {"string", "ending_before", "cursor: end before this pageInfo.startCursor"},
	"group-by":               {"string", "group_by", `fields to group by, as JSON: [{"city":true}]`},
	"aggregate":              {"string", "aggregate", `aggregates per group, as JSON: ["countNotEmptyId"]`},
	"view-id":                {"string", "view_id", "apply the filters of this view (a UUID)"},
	"include-records-sample": {"bool", "include_records_sample", "include sample records in each group"},
	"order-by-for-records":   {"string", "order_by_for_records", "order of the sample records in each group"},
	"upsert":                 {"bool", "upsert", "update the record instead when it already exists"},
	"all":                    {"bool", "", "follow every page and print one JSON array of the records"},
	"max-pages":              {"int", "", "page cap for --all"},
	"dry-run":                {"bool", "", "preview the merge without changing anything (no --force needed)"},
}

// objectAliases are the other names an object answers to: its API plural
// and its kebab-case singular, when they differ from the command.
func objectAliases(o *model.Object) []string {
	var out []string
	for _, s := range []string{o.NamePlural, model.CommandName(o.NameSingular)} {
		if s != o.Command && !contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// addObjectCommands hangs one group per object off root, each with the full
// verb table. A built-in wins over an object of the same name, and an alias
// never takes a name another object or a built-in already has.
func (a *app) addObjectCommands(root *cobra.Command) {
	if a.model == nil {
		return
	}
	taken := map[string]bool{}
	for _, n := range builtinNames {
		taken[n] = true
	}
	for _, o := range a.model.Objects {
		taken[o.Command] = true
	}
	for i := range a.model.Objects {
		obj := &a.model.Objects[i]
		if isBuiltin(obj.Command) {
			continue // shadowed: reachable through api; the catalog says so
		}
		short := obj.Description
		if short == "" {
			short = "Records of " + obj.NamePlural
		}
		group := &cobra.Command{Use: obj.Command, Short: short, RunE: groupRunE}
		for _, alias := range objectAliases(obj) {
			if !taken[alias] {
				group.Aliases = append(group.Aliases, alias)
				taken[alias] = true
			}
		}
		for _, v := range routes.ObjectVerbs {
			group.AddCommand(a.objectVerbCommand(obj, v))
		}
		root.AddCommand(group)
	}
}

func (a *app) objectVerbCommand(obj *model.Object, v routes.Verb) *cobra.Command {
	use, args := v.Name, cobra.NoArgs
	if v.TakesID {
		use, args = v.Name+" <id>", cobra.ExactArgs(1)
	}
	cmd := &cobra.Command{
		Use: use, Short: v.Summary, Args: args,
		RunE: func(cmd *cobra.Command, args []string) error { return a.runObjectVerb(cmd, obj, v, args) },
	}
	// Help is built on demand: rendering field tables for every command on
	// every run would cost more than the call itself.
	cmd.SetHelpFunc(func(c *cobra.Command, s []string) {
		c.Long = objectHelp(obj, v)
		c.Parent().HelpFunc()(c, s)
	})
	registerVerbFlags(cmd, v)
	return cmd
}

func registerVerbFlags(cmd *cobra.Command, v routes.Verb) {
	for _, name := range v.Flags {
		spec := verbFlags[name]
		switch spec.kind {
		case "int":
			def := 0
			if name == "max-pages" {
				def = 100
			}
			cmd.Flags().Int(name, def, spec.usage)
		case "bool":
			cmd.Flags().Bool(name, false, spec.usage)
		default:
			cmd.Flags().String(name, "", spec.usage)
		}
	}
	if v.Body != routes.BodyNone {
		cmd.Flags().String("data", "", "JSON body: literal, @file, or - for stdin")
	}
}

// verbQuery turns the verb's flags into query parameters, checks their
// ranges and adds the verb's fixed soft_delete value.
func verbQuery(cmd *cobra.Command, v routes.Verb) (url.Values, error) {
	q := url.Values{}
	for _, name := range v.Flags {
		spec := verbFlags[name]
		if spec.query == "" || !cmd.Flags().Changed(name) {
			continue
		}
		switch spec.kind {
		case "int":
			n, _ := cmd.Flags().GetInt(name)
			if name == "depth" && n != 0 && n != 1 {
				return nil, api.Usagef("--depth takes 0 or 1, got %d", n)
			}
			if name == "limit" && (n < 1 || n > v.MaxLimit) {
				return nil, api.Usagef("--limit takes 1 to %d, got %d", v.MaxLimit, n)
			}
			q.Set(spec.query, strconv.Itoa(n))
		case "bool":
			if flagBool(cmd, name) {
				q.Set(spec.query, "true")
			}
		default:
			// A blank value is left out: Twenty rejects an empty filter,
			// and a required filter is caught by the gates.
			if s := flagString(cmd, name); strings.TrimSpace(s) != "" {
				q.Set(spec.query, s)
			}
		}
	}
	if v.RequiredFlag != "" && q.Get(verbFlags[v.RequiredFlag].query) == "" {
		return nil, api.Usagef("--%s is required", v.RequiredFlag)
	}
	if v.SoftDelete != "" {
		q.Set("soft_delete", v.SoftDelete)
	}
	return q, nil
}

// runObjectVerb turns one parsed command into one API call. Every check
// runs before any network I/O, so a refused call has no side effects.
func (a *app) runObjectVerb(cmd *cobra.Command, obj *model.Object, v routes.Verb, args []string) error {
	id := ""
	if v.TakesID {
		id = args[0]
		if err := routes.ValidateID(id); err != nil {
			return api.Usagef("%v", err)
		}
	}
	q, err := verbQuery(cmd, v)
	if err != nil {
		return err
	}
	var body []byte
	if v.Body != routes.BodyNone {
		if body, err = a.readJSONBody(cmd); err != nil {
			return err
		}
	}
	command := obj.Command + " " + v.Name
	if err := routes.CheckBody(v.Body, body); err != nil {
		return api.Usagef("%s: %v", command, err)
	}
	class := v.Class
	if v.Name == "merge" && flagBool(cmd, "dry-run") {
		class = routes.ClassRead
		if body, err = setJSONField(body, "dryRun", true); err != nil {
			return err
		}
	}
	d := routes.Decision{Command: command, Class: class, FilterRequired: v.FilterRequired,
		Filter: q.Get("filter"), ReadOnly: a.res.ReadOnly, Force: flagBool(cmd, "force")}
	if err := d.Check(); err != nil {
		return api.Usagef("%v", err)
	}
	req := api.Request{Method: v.Method, Path: v.Path(obj.NamePlural, id), Query: q, Body: body, Risk: class, Object: obj.Command}
	return a.send(cmd, req, pager{rowsKey: obj.NamePlural, pageSize: 200})
}

// send runs one checked request, or a page walk with --all, and writes the
// result.
func (a *app) send(cmd *cobra.Command, req api.Request, p pager) error {
	if err := a.callable(); err != nil {
		return err
	}
	a.applyGlobalFlags(cmd)
	out, err := openOutput(flagString(cmd, "output"))
	if err != nil {
		return err
	}
	defer out.discard()
	if cmd.Flags().Lookup("all") != nil && flagBool(cmd, "all") {
		return a.runAll(cmd, req, p, out)
	}
	resp, err := a.client.Do(cmd.Context(), req)
	if err != nil {
		return err
	}
	return a.writeResponse(resp, out)
}

func (a *app) applyGlobalFlags(cmd *cobra.Command) {
	if flagBool(cmd, "verbose") {
		a.client.Verbose = a.stderr
	}
	if d, err := cmd.Flags().GetDuration("timeout"); err == nil {
		a.client.Timeout = d
	}
}
```

- [ ] **Step 10: Implement `internal/cli/metadata.go`**

```go
package cli

import (
	"github.com/spf13/cobra"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/api"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/routes"
)

func (a *app) metadataCommand() *cobra.Command {
	md := &cobra.Command{
		Use:   "metadata",
		Short: "Data model, views, page layouts and webhooks (Twenty's metadata API)",
		Long: "Data model, views, page layouts and webhooks (Twenty's metadata API).\n\n" +
			"Reading is read-class. Creating, updating and deleting is admin-class and needs --force;\n" +
			"deleting an object or a field removes its data for good. API keys can be read but never\n" +
			"created, changed or revoked here. objects and fields need the Data Model permission on\n" +
			"the key's role.",
		RunE: groupRunE,
	}
	for _, k := range routes.MetadataKinds {
		kind := k
		g := &cobra.Command{Use: kind.Command, Short: "Metadata: " + kind.Command, RunE: groupRunE}
		for _, v := range routes.MetadataVerbs {
			g.AddCommand(a.metadataVerbCommand(kind, v))
		}
		md.AddCommand(g)
	}
	return md
}

func (a *app) metadataVerbCommand(kind routes.MetaKind, v routes.Verb) *cobra.Command {
	blocked := routes.MetadataBlocked(kind.Command, v.Name)
	short := v.Summary
	if blocked != "" {
		short = "[blocked] " + short
	}
	use, args := v.Name, cobra.NoArgs
	if v.TakesID {
		use, args = v.Name+" <id>", cobra.ExactArgs(1)
	}
	cmd := &cobra.Command{
		Use: use, Short: short, Args: args,
		RunE: func(cmd *cobra.Command, args []string) error {
			id := ""
			if v.TakesID {
				id = args[0]
				if err := routes.ValidateID(id); err != nil {
					return api.Usagef("%v", err)
				}
			}
			q, err := verbQuery(cmd, v)
			if err != nil {
				return err
			}
			var body []byte
			if v.Body != routes.BodyNone {
				if body, err = a.readJSONBody(cmd); err != nil {
					return err
				}
			}
			command := "metadata " + kind.Command + " " + v.Name
			if err := routes.CheckBody(v.Body, body); err != nil {
				return api.Usagef("%s: %v", command, err)
			}
			d := routes.Decision{Command: command, Class: v.Class, Blocked: blocked, ReadOnly: a.res.ReadOnly, Force: flagBool(cmd, "force")}
			if err := d.Check(); err != nil {
				return api.Usagef("%v", err)
			}
			req := api.Request{Method: v.Method, Path: v.Path(kind.Segment, id), Query: q, Body: body, Risk: v.Class}
			return a.send(cmd, req, pager{rowsKey: kind.Segment, pageSize: 1000})
		},
	}
	registerVerbFlags(cmd, v)
	return cmd
}
```

- [ ] **Step 11: Implement `internal/cli/all.go`**

```go
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/api"
)

// pager says where a list response keeps its rows and how large a page
// --all asks for when --limit was not given.
type pager struct {
	rowsKey  string // data.<rowsKey> holds the rows (records, legacy metadata)
	pageSize int
}

// runAll follows pageInfo.endCursor and writes ONE JSON array of the merged
// rows. It is the only place the CLI reshapes a response, so each row reaches
// the output exactly as Twenty sent it. Stopping early is never silent: the
// rows so far are written and the run ends as incomplete (the page cap) or
// server (a page that claims more but gives no cursor).
func (a *app) runAll(cmd *cobra.Command, base api.Request, p pager, out *outputFile) error {
	maxPages, _ := cmd.Flags().GetInt("max-pages")
	if maxPages < 1 {
		return api.Usagef("--max-pages must be at least 1, got %d", maxPages)
	}
	if base.Query.Get("ending_before") != "" {
		return api.Usagef("--all walks forward from the first page or from --starting-after; it cannot be combined with --ending-before")
	}
	q := cloneValues(base.Query)
	if q.Get("limit") == "" {
		q.Set("limit", strconv.Itoa(p.pageSize))
	}
	var rows []json.RawMessage
	var stop *api.Error
	complete := false
	for page := 0; page < maxPages && stop == nil && !complete; page++ {
		req := base
		req.Query = cloneValues(q)
		resp, err := a.client.Do(cmd.Context(), req)
		if err != nil {
			return err
		}
		pageRows, cursor, more, err := parsePage(resp.Body, p.rowsKey)
		if err != nil {
			stop = &api.Error{Kind: api.KindServer, Message: fmt.Sprintf("%s %s: %v", req.Method, req.Path, err)}
			break
		}
		rows = append(rows, pageRows...)
		switch {
		case !more:
			complete = true
		case cursor == "":
			stop = &api.Error{Kind: api.KindServer, Message: fmt.Sprintf(
				"%s %s: pageInfo.hasNextPage is true but endCursor is empty; stopped instead of fetching the first page again, the output is partial",
				req.Method, req.Path)}
		default:
			q.Set("starting_after", cursor)
		}
	}
	var buf bytes.Buffer
	buf.WriteByte('[')
	for i, r := range rows {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.Write(r)
	}
	buf.WriteString("]\n")
	if err := a.writeResponse(&api.Response{Body: buf.Bytes()}, out); err != nil {
		return err
	}
	if stop != nil {
		return stop
	}
	if !complete {
		return &api.Error{Kind: api.KindIncomplete, Message: "hit --max-pages before the last page; the output is partial"}
	}
	return nil
}

// parsePage reads one list response: rows from data.<rowsKey> or, in the
// newer metadata format, from data itself; the cursor from pageInfo.
func parsePage(body []byte, rowsKey string) (rows []json.RawMessage, endCursor string, hasNext bool, err error) {
	var env struct {
		Data     json.RawMessage `json:"data"`
		PageInfo *struct {
			HasNextPage bool   `json:"hasNextPage"`
			EndCursor   string `json:"endCursor"`
		} `json:"pageInfo"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, "", false, fmt.Errorf("--all expects a JSON object response: %v", err)
	}
	var inner map[string]json.RawMessage
	if json.Unmarshal(env.Data, &inner) == nil && inner != nil {
		raw, ok := inner[rowsKey]
		if !ok || json.Unmarshal(raw, &rows) != nil {
			return nil, "", false, fmt.Errorf("--all expected data.%s to be an array", rowsKey)
		}
	} else if json.Unmarshal(env.Data, &rows) != nil || env.Data == nil {
		return nil, "", false, fmt.Errorf("--all expected data.%s or data to be an array, got %.80s", rowsKey, env.Data)
	}
	if env.PageInfo == nil {
		return rows, "", false, nil
	}
	return rows, env.PageInfo.EndCursor, env.PageInfo.HasNextPage, nil
}
```

- [ ] **Step 12: Implement `internal/cli/help.go`**

```go
package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/model"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/routes"
)

const filterHelp = `
Filter syntax (--filter): field[comparator]:value, joined by commas (all must match), or
wrapped in or(...) and not(...). Composite fields take a dot: emails.primaryEmail[eq]:ana@example.com.
Quote values that hold commas or spaces.
Comparators: eq, neq, in, containsAny, is, gt, gte, lt, lte, startsWith, endsWith, like, ilike.
  --filter 'name[ilike]:"%acme%"'
  --filter 'or(stage[eq]:LEAD,employees[gt]:50)'
  --filter 'deletedAt[is]:NOT_NULL'
Order (--order-by): field[AscNullsFirst|AscNullsLast|DescNullsFirst|DescNullsLast], comma-separated.
`

// objectHelp is the long help of one object verb: the request it sends, its
// class and gates, and the object's fields from the cached model.
func objectHelp(obj *model.Object, v routes.Verb) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s.\n\nRequest: %s /%s", v.Summary, v.Method, v.Path(obj.NamePlural, "<id>"))
	if v.SoftDelete != "" {
		fmt.Fprintf(&b, "?soft_delete=%s", v.SoftDelete)
	}
	fmt.Fprintf(&b, "\nClass: %s", v.Class)
	if routes.NeedsForce(v.Class) {
		b.WriteString(" (needs --force)")
	}
	if v.FilterRequired {
		b.WriteString("; --filter is required")
	}
	if v.Name == "merge" {
		b.WriteString("; read-class with --dry-run")
	}
	b.WriteString("\n")
	if contains(v.Flags, "filter") {
		b.WriteString(filterHelp)
	}
	switch v.Body {
	case routes.BodyArray:
		fmt.Fprintf(&b, "\n--data takes a JSON array of 1 to %d records; the CLI never splits a batch.\n", routes.MaxBatch)
	case routes.BodyMerge:
		b.WriteString("\n--data: {\"ids\":[\"<id>\",\"<id>\"],\"conflictPriorityIndex\":0}; the record at that index wins conflicts.\n")
	case routes.BodyDuplicates:
		b.WriteString("\n--data: {\"ids\":[\"<id>\"]} or {\"data\":[{\"name\":\"Acme\"}]}.\n")
	}
	fmt.Fprintf(&b, "\nFields of %s (from the cached workspace model; `twentycrm schema %s` reads them live):\n", obj.NamePlural, obj.Command)
	tw := tabwriter.NewWriter(&b, 2, 4, 2, ' ', 0)
	for _, f := range obj.Fields {
		typ := f.Type
		if f.Format != "" {
			typ += "/" + f.Format
		}
		var notes []string
		if f.Required {
			notes = append(notes, "required")
		}
		if f.ReadOnly {
			notes = append(notes, "read-only")
		}
		if len(f.Enum) > 0 {
			notes = append(notes, "one of: "+strings.Join(f.Enum, ", "))
		}
		if len(f.Subfields) > 0 {
			notes = append(notes, "subfields: "+strings.Join(f.Subfields, ", "))
		}
		if f.Relation != nil {
			rel := f.Relation.Kind + " -> " + f.Relation.Target
			if f.Relation.Kind == "many_to_one" {
				rel += " (set " + f.Name + "Id)"
			}
			notes = append(notes, rel)
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\n", f.Name, typ, strings.Join(notes, "; "))
	}
	tw.Flush()
	return b.String()
}
```

- [ ] **Step 13: Run all tests and vet**

Run: `go mod tidy && go vet ./... && go test ./...`
Expected: PASS. If `TestObjectHelp` fails on the `name ... required` regexp, check the tabwriter columns: the line must read `  name  string  required`.

- [ ] **Step 14: Build and try the binary by hand**

Run: `go build -o bin/twentycrm ./cmd/twentycrm && ./bin/twentycrm version && ./bin/twentycrm metadata objects list; echo "exit $?"`
Expected: the version line; then one JSON error line on stderr naming the missing base URL or key, and `exit 2`.

- [ ] **Step 15: Commit**

```bash
git add cmd internal/cli go.mod go.sum
git commit -m "Add the command tree: objects, metadata, paging and help

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---
### Task 4: Built-in commands: commands, schema, api, config, auth, init

**Files:**
- Create: `internal/cli/catalog.go`, `internal/cli/schemacmd.go`, `internal/cli/apicmd.go`, `internal/cli/configcmd.go`, `internal/cli/authcmd.go`, `internal/cli/initcmd.go`
- Create (copied): `internal/cli/prompt.go`
- Modify: `internal/cli/app.go` (two fields on `app`), `internal/cli/root.go` (register the commands)
- Test: `internal/cli/catalog_test.go`, `internal/cli/schemacmd_test.go`, `internal/cli/apicmd_test.go`, `internal/cli/configcmd_test.go`, `internal/cli/authcmd_test.go`, `internal/cli/initcmd_test.go`

**Interfaces:**
- Consumes (Tasks 1 to 3): everything listed as produced there, in particular `app`, `(*app).fetchModel`, `(*app).send`, `(*app).readJSONBody`, `openOutput`, `(*app).writeResponse`, `(*app).applyGlobalFlags`, `(*app).callable`, `(*app).cacheRoot`, `(*app).clock`, `groupRunE`, `flagBool`, `flagString`, `jsonCompact`, `isBuiltin`, `routes.ClassifyRaw`, `routes.CleanRawPath`, `routes.Decision`, `routes.ObjectVerbs`, `routes.MetadataKinds`, `routes.MetadataVerbs`, `routes.MetadataBlocked`, `routes.MaxBatch`, `config.Load`, `config.Save`, `config.Path`, `config.NormalizeBaseURL`, `auth.CleanKey`, `auth.ParseAPIKey`, `auth.Decode`, `model.Extract`, `model.ErrNoObjects`, `model.SaveCache`, and the test helpers of `app_test.go`.
- Produces: `prompter` (interface `Line`, `Secret`, `Confirm`), `ask`, `isTerminal`, `stdioIsTerminal` (from `prompt.go`); `app.prompt prompter`, `app.isTerminal func() bool`; `(*app).commandsCommand()`, `(*app).schemaCommand()`, `(*app).apiCommand()`, `(*app).configCommand()`, `(*app).authCommand()`, `(*app).initCommand()`; `(*app).emit(cmd *cobra.Command, v any) error`.

- [ ] **Step 1: Add the terminal dependency, copy the prompter, extend `app`**

Run: `go get golang.org/x/term@v0.45.0`

Copy `/Users/daniel/Development/google-ads-cli/internal/cli/prompt.go` into `internal/cli/prompt.go`. Change the mentions of `googleads init` in comments to `twentycrm init`, and rewrite the two em dashes in the comment above `isTerminal` as a colon and a semicolon (the repository has none; Task 5 checks). Nothing else changes.

In `internal/cli/app.go`, add to the `app` struct after `now`:

```go
	// prompt and isTerminal are seams for `init`, the one interactive
	// command; nil in production, where init uses the real terminal.
	prompt     prompter
	isTerminal func() bool
```

In `internal/cli/root.go`, change the `AddCommand` line to:

```go
	root.AddCommand(a.versionCommand(), a.commandsCommand(), a.schemaCommand(), a.apiCommand(),
		a.configCommand(), a.authCommand(), a.initCommand(), a.metadataCommand())
```

- [ ] **Step 2: Write the failing tests**

`internal/cli/catalog_test.go`:

```go
package cli

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/model"
)

// catalogDoc mirrors the published JSON keys, so the test pins the contract
// rather than the Go field names.
type catalogDoc struct {
	SchemaVersion  int    `json:"schema_version"`
	ModelFetchedAt string `json:"model_fetched_at"`
	ModelMissing   bool   `json:"model_missing"`
	Objects        []struct {
		Command      string `json:"command"`
		NamePlural   string `json:"name_plural"`
		NameSingular string `json:"name_singular"`
		ShadowedBy   string `json:"shadowed_by"`
	} `json:"objects"`
	ObjectVerbs []struct {
		Verb           string   `json:"verb"`
		Method         string   `json:"method"`
		Path           string   `json:"path"`
		Class          string   `json:"class"`
		SoftDelete     string   `json:"soft_delete"`
		TakesID        bool     `json:"takes_id"`
		FilterRequired bool     `json:"filter_required"`
		MaxRecords     int      `json:"max_records"`
		Flags          []string `json:"flags"`
	} `json:"object_verbs"`
	Metadata []struct {
		Command       string `json:"command"`
		Method        string `json:"method"`
		Path          string `json:"path"`
		Class         string `json:"class"`
		Blocked       bool   `json:"blocked"`
		BlockedReason string `json:"blocked_reason"`
	} `json:"metadata"`
	Commands []struct {
		Command string `json:"command"`
		Summary string `json:"summary"`
	} `json:"commands"`
}

func decodeCatalog(t *testing.T, raw []byte) catalogDoc {
	t.Helper()
	var d catalogDoc
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatalf("catalog is not JSON: %s", raw)
	}
	return d
}

func TestCommandsCatalog(t *testing.T) {
	srv := newFakeTwenty(t)
	a, stdout, stderr := newTestApp(t, srv, nil)
	seedCache(t, a, time.Hour)
	if code := a.run([]string{"commands", "--json"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	d := decodeCatalog(t, stdout.Bytes())
	if d.SchemaVersion != 1 || d.ModelMissing || d.ModelFetchedAt == "" || srv.fetches() != 0 {
		t.Fatalf("header: %+v, %d fetches", d, srv.fetches())
	}
	var objs []string
	for _, o := range d.Objects {
		objs = append(objs, o.Command)
	}
	if len(objs) != 4 || objs[2] != "note-targets" {
		t.Fatalf("objects = %v", objs)
	}
	if len(d.ObjectVerbs) != 15 {
		t.Fatalf("%d object verbs", len(d.ObjectVerbs))
	}
	verbs := map[string]int{}
	for i, v := range d.ObjectVerbs {
		verbs[v.Verb] = i
	}
	if v := d.ObjectVerbs[verbs["batch-create"]]; v.MaxRecords != 60 || v.Path != "rest/batch/{plural}" {
		t.Errorf("batch-create = %+v", v)
	}
	if v := d.ObjectVerbs[verbs["destroy"]]; v.Class != "destroy" || v.SoftDelete != "false" || !v.TakesID {
		t.Errorf("destroy = %+v", v)
	}
	if v := d.ObjectVerbs[verbs["update-many"]]; !v.FilterRequired || !contains(v.Flags, "data") || !contains(v.Flags, "filter") {
		t.Errorf("update-many = %+v", v)
	}
	var blocked, admin bool
	for _, m := range d.Metadata {
		if m.Command == "metadata api-keys delete" && m.Blocked && m.BlockedReason != "" {
			blocked = true
		}
		if m.Command == "metadata fields create" && m.Class == "admin" && m.Path == "rest/metadata/fields" {
			admin = true
		}
	}
	if !blocked || !admin || len(d.Metadata) != 13*5 {
		t.Errorf("metadata: blocked %v admin %v, %d entries", blocked, admin, len(d.Metadata))
	}
	var cmds []string
	for _, c := range d.Commands {
		cmds = append(cmds, c.Command)
	}
	for _, want := range []string{"api", "auth", "commands", "config", "init", "metadata", "schema", "version"} {
		if !contains(cmds, want) {
			t.Errorf("commands lack %s: %v", want, cmds)
		}
	}
}

func TestCommandsCatalogWithoutModel(t *testing.T) {
	srv := newFakeTwenty(t)
	a, stdout, _ := newTestApp(t, srv, nil)
	a.run([]string{"commands", "--json"})
	d := decodeCatalog(t, stdout.Bytes())
	if !d.ModelMissing || len(d.Objects) != 0 || len(d.ObjectVerbs) != 15 || srv.fetches() != 0 {
		t.Fatalf("%+v, %d fetches", d, srv.fetches())
	}
}

func TestCatalogMarksShadowedObjects(t *testing.T) {
	srv := newFakeTwenty(t)
	a, stdout, _ := newTestApp(t, srv, nil)
	model.SaveCache(a.cacheRoot(), &model.Model{FetchedAt: time.Now(), BaseURL: a.res.BaseURL, WorkspaceID: a.workspaceID(),
		Objects: []model.Object{{Command: "config", NamePlural: "config", NameSingular: "configEntry"}}})
	a.run([]string{"commands", "--json"})
	d := decodeCatalog(t, stdout.Bytes())
	if len(d.Objects) != 1 || d.Objects[0].ShadowedBy != "config" {
		t.Fatalf("objects = %+v", d.Objects)
	}
}
```

`internal/cli/schemacmd_test.go`:

```go
package cli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/model"
)

func TestSchemaListsObjectsLive(t *testing.T) {
	srv := newFakeTwenty(t)
	a, stdout, stderr := newTestApp(t, srv, nil)
	seedCache(t, a, time.Hour, "companies")
	if code := a.run([]string{"schema"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var list []struct {
		Command     string `json:"command"`
		NamePlural  string `json:"name_plural"`
		Description string `json:"description"`
		Fields      int    `json:"fields"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &list); err != nil || len(list) != 4 || list[0].Command != "companies" || list[0].Fields == 0 {
		t.Fatalf("schema = %s (%v)", stdout, err)
	}
	if srv.fetches() != 1 {
		t.Fatalf("schema must read live: %d fetches", srv.fetches())
	}
	if m := model.LoadCache(a.cacheRoot(), a.res.BaseURL, "ws-test"); m == nil || m.Find("invoices") == nil {
		t.Fatal("schema must rewrite the cache")
	}
}

func TestSchemaObject(t *testing.T) {
	for _, name := range []string{"note-targets", "noteTargets"} {
		srv := newFakeTwenty(t)
		a, stdout, stderr := newTestApp(t, srv, nil)
		if code := a.run([]string{"schema", name}); code != 0 {
			t.Fatalf("%s: exit %d: %s", name, code, stderr)
		}
		var obj model.Object
		if err := json.Unmarshal(stdout.Bytes(), &obj); err != nil || obj.NamePlural != "noteTargets" || len(obj.Fields) == 0 {
			t.Fatalf("%s: %s", name, stdout)
		}
	}
	srv := newFakeTwenty(t)
	a, stdout, _ := newTestApp(t, srv, nil)
	a.run([]string{"schema", "companies"})
	if !strings.Contains(stdout.String(), `"enum":["LEAD","CUSTOMER","CHURNED"]`) {
		t.Fatalf("companies schema lacks the select values: %s", stdout)
	}
}

func TestSchemaUnknownObject(t *testing.T) {
	srv := newFakeTwenty(t)
	a, _, stderr := newTestApp(t, srv, nil)
	if code := a.run([]string{"schema", "widgets"}); code != 2 || !strings.Contains(errLine(t, stderr.String()).Message, "twentycrm schema") {
		t.Fatalf("exit %d: %s", code, stderr)
	}
}
```

`internal/cli/apicmd_test.go`:

```go
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
```

`internal/cli/configcmd_test.go`:

```go
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
```

`internal/cli/authcmd_test.go`:

```go
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
```

`internal/cli/initcmd_test.go`:

```go
package cli

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/config"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/model"
)

type fakePrompter struct {
	lines, secrets []string
	confirms       []bool
}

func (f *fakePrompter) Line(string) (string, error) {
	if len(f.lines) == 0 {
		return "", io.EOF
	}
	s := f.lines[0]
	f.lines = f.lines[1:]
	return s, nil
}

func (f *fakePrompter) Secret(string) (string, error) {
	if len(f.secrets) == 0 {
		return "", io.EOF
	}
	s := f.secrets[0]
	f.secrets = f.secrets[1:]
	return s, nil
}

func (f *fakePrompter) Confirm(_ string, def bool) (bool, error) {
	if len(f.confirms) == 0 {
		return def, nil
	}
	b := f.confirms[0]
	f.confirms = f.confirms[1:]
	return b, nil
}

// initApp builds an app over the isolated home with a terminal and scripted
// answers; companies answers GET /rest/companies with the given status.
func initApp(t *testing.T, companies int, p *fakePrompter) (*app, *fakeTwenty, *strings.Builder, *strings.Builder) {
	t.Helper()
	srv := newFakeTwenty(t)
	srv.handle = func(w http.ResponseWriter, r *http.Request, _ []byte) {
		w.WriteHeader(companies)
		if companies == 200 {
			io.WriteString(w, `{"data":{"companies":[]},"totalCount":7,"pageInfo":{"hasNextPage":false}}`)
			return
		}
		io.WriteString(w, `{"statusCode":403,"messages":["Forbidden"],"error":"FORBIDDEN"}`)
	}
	var out, errb strings.Builder
	a := &app{stdout: &out, stderr: &errb, stdin: strings.NewReader("")}
	a.configure(func(string) string { return "" }, func(time.Duration) {})
	a.isTerminal = func() bool { return true }
	a.prompt = p
	return a, srv, &out, &errb
}

func TestInitRefusesWithoutTerminal(t *testing.T) {
	isolate(t)
	a, _, _, errb := initApp(t, 200, &fakePrompter{})
	a.isTerminal = func() bool { return false }
	if code := a.run([]string{"init"}); code != 2 || !strings.Contains(errb.String(), "terminal") {
		t.Fatalf("exit %d %s", code, errb)
	}
}

func TestInitHappyPath(t *testing.T) {
	isolate(t)
	key := testKey(t, "ws-test", time.Now().Add(90*24*time.Hour))
	p := &fakePrompter{secrets: []string{key}, confirms: []bool{true}}
	a, srv, out, errb := initApp(t, 200, p)
	p.lines = []string{srv.URL}
	if code := a.run([]string{"init"}); code != 0 {
		t.Fatalf("exit %d: %s\n%s", code, errb, out)
	}
	for _, want := range []string{"ws-test", "4 objects", "7 companies"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	c := mustLoad(t)
	if c.BaseURL != srv.URL || c.APIKey != key || !c.ReadOnly {
		t.Fatalf("config = %+v", c)
	}
	root, _ := os.UserCacheDir()
	if model.LoadCache(root, srv.URL, "ws-test") == nil {
		t.Fatal("init must write the model cache")
	}
}

func TestInitSkipsTheCountWhenForbidden(t *testing.T) {
	isolate(t)
	p := &fakePrompter{secrets: []string{testKey(t, "ws-test", time.Time{})}}
	a, srv, out, errb := initApp(t, 403, p)
	p.lines = []string{srv.URL}
	if code := a.run([]string{"init"}); code != 0 || !strings.Contains(out.String(), "company count unavailable") {
		t.Fatalf("exit %d: %s\n%s", code, errb, out)
	}
}

func TestInitSavesNothingWhenTheKeyIsNotAccepted(t *testing.T) {
	isolate(t)
	p := &fakePrompter{secrets: []string{testKey(t, "ws-test", time.Time{})}}
	a, srv, _, errb := initApp(t, 200, p)
	srv.openAPI = readFixture(t, "openapi-skeleton.json")
	p.lines = []string{srv.URL}
	if code := a.run([]string{"init"}); code != 1 || errLine(t, errb.String()).Kind != "auth" {
		t.Fatalf("exit %d: %s", code, errb)
	}
	if mustLoad(t) != (config.Config{}) {
		t.Fatal("init saved a configuration that did not verify")
	}
}

func TestInitRefusesExpiredAndUnreadableKeys(t *testing.T) {
	isolate(t)
	expired := testKey(t, "ws-test", time.Now().Add(-time.Hour))
	p := &fakePrompter{secrets: []string{expired, "garbage", expired}}
	a, srv, out, errb := initApp(t, 200, p)
	p.lines = []string{srv.URL}
	if code := a.run([]string{"init"}); code != 2 || !strings.Contains(out.String(), "expired") ||
		!strings.Contains(out.String(), "three base64url parts") {
		t.Fatalf("exit %d: %s\n%s", code, errb, out)
	}
	if len(srv.reqs) != 0 {
		t.Fatal("a refused key reached the server")
	}
}

func TestInitAsksBeforeReplacingAKeyForAnotherHost(t *testing.T) {
	isolate(t)
	old := config.Config{BaseURL: "https://old.example.com", APIKey: testKey(t, "ws-old", time.Time{})}
	config.Save(old)
	p := &fakePrompter{secrets: []string{testKey(t, "ws-test", time.Time{})}, confirms: []bool{false}}
	a, srv, _, errb := initApp(t, 200, p)
	p.lines = []string{srv.URL}
	if code := a.run([]string{"init"}); code != 2 || !strings.Contains(errLine(t, errb.String()).Message, "nothing was saved") {
		t.Fatalf("exit %d: %s", code, errb)
	}
	if mustLoad(t) != old {
		t.Fatal("the old configuration changed")
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/cli/`
Expected: FAIL (build error: undefined `commandsCommand`, `schemaCommand`, `prompter`, ...).

- [ ] **Step 4: Implement `internal/cli/catalog.go`**

```go
package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/api"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/routes"
)

// The catalog is `twentycrm commands --json`: schema_version is the contract
// a consumer pins to; new optional keys may appear under the same version.
type catalogObject struct {
	Command      string `json:"command"`
	NamePlural   string `json:"name_plural"`
	NameSingular string `json:"name_singular"`
	ShadowedBy   string `json:"shadowed_by,omitempty"`
}

type catalogVerb struct {
	Verb           string   `json:"verb"`
	Method         string   `json:"method"`
	Path           string   `json:"path"`
	Class          string   `json:"class"`
	TakesID        bool     `json:"takes_id"`
	FilterRequired bool     `json:"filter_required"`
	SoftDelete     string   `json:"soft_delete,omitempty"`
	Body           string   `json:"body,omitempty"`
	MaxRecords     int      `json:"max_records,omitempty"`
	Flags          []string `json:"flags"`
	Summary        string   `json:"summary"`
}

type catalogMeta struct {
	Command       string `json:"command"`
	Method        string `json:"method"`
	Path          string `json:"path"`
	Class         string `json:"class"`
	TakesID       bool   `json:"takes_id"`
	Blocked       bool   `json:"blocked,omitempty"`
	BlockedReason string `json:"blocked_reason,omitempty"`
}

type catalogCommand struct {
	Command string `json:"command"`
	Summary string `json:"summary"`
}

type catalog struct {
	SchemaVersion  int              `json:"schema_version"`
	ModelFetchedAt *time.Time       `json:"model_fetched_at,omitempty"`
	ModelMissing   bool             `json:"model_missing,omitempty"`
	Objects        []catalogObject  `json:"objects"`
	ObjectVerbs    []catalogVerb    `json:"object_verbs"`
	Metadata       []catalogMeta    `json:"metadata"`
	Commands       []catalogCommand `json:"commands"`
}

func verbCatalog(v routes.Verb) catalogVerb {
	flags := append([]string{}, v.Flags...)
	if v.Body != routes.BodyNone {
		flags = append(flags, "data")
	}
	e := catalogVerb{Verb: v.Name, Method: v.Method, Path: v.PathTemplate, Class: v.Class, TakesID: v.TakesID,
		FilterRequired: v.FilterRequired, SoftDelete: v.SoftDelete, Body: string(v.Body), Flags: flags, Summary: v.Summary}
	if v.Body == routes.BodyArray {
		e.MaxRecords = routes.MaxBatch
	}
	return e
}

// buildCatalog describes the tree from the cached model; it never fetches.
func (a *app) buildCatalog(root *cobra.Command) catalog {
	c := catalog{SchemaVersion: 1, Objects: []catalogObject{}, ObjectVerbs: []catalogVerb{}, Metadata: []catalogMeta{}, Commands: []catalogCommand{}}
	if a.model == nil {
		c.ModelMissing = true
	} else {
		t := a.model.FetchedAt
		c.ModelFetchedAt = &t
		for _, o := range a.model.Objects {
			e := catalogObject{Command: o.Command, NamePlural: o.NamePlural, NameSingular: o.NameSingular}
			if isBuiltin(o.Command) {
				e.ShadowedBy = o.Command
			}
			c.Objects = append(c.Objects, e)
		}
	}
	for _, v := range routes.ObjectVerbs {
		c.ObjectVerbs = append(c.ObjectVerbs, verbCatalog(v))
	}
	for _, k := range routes.MetadataKinds {
		for _, v := range routes.MetadataVerbs {
			reason := routes.MetadataBlocked(k.Command, v.Name)
			c.Metadata = append(c.Metadata, catalogMeta{Command: "metadata " + k.Command + " " + v.Name, Method: v.Method,
				Path: v.Path(k.Segment, "{id}"), Class: v.Class, TakesID: v.TakesID, Blocked: reason != "", BlockedReason: reason})
		}
	}
	for _, cmd := range root.Commands() {
		if isBuiltin(cmd.Name()) && cmd.Name() != "help" && cmd.Name() != "completion" {
			c.Commands = append(c.Commands, catalogCommand{Command: cmd.Name(), Summary: cmd.Short})
		}
	}
	return c
}

func (a *app) commandsCommand() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "commands",
		Short: "List the object verbs, metadata commands and objects (--json for the machine-readable catalog)",
		Long: "List the command grammar. The verb table is the same for every object, so the catalog lists it once,\n" +
			"next to the workspace's objects from the cached model. When no model is cached yet, model_missing is\n" +
			"true; `twentycrm schema` loads it.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c := a.buildCatalog(cmd.Root())
			if asJSON {
				return a.emit(cmd, c)
			}
			for _, o := range c.Objects {
				fmt.Fprintf(a.stdout, "object  %s\n", o.Command)
			}
			for _, v := range c.ObjectVerbs {
				fmt.Fprintf(a.stdout, "verb    %-16s %-6s %-30s [%s]\n", v.Verb, v.Method, v.Path, v.Class)
			}
			for _, m := range c.Metadata {
				tag := ""
				if m.Blocked {
					tag = " blocked"
				}
				fmt.Fprintf(a.stdout, "%-40s %-6s [%s]%s\n", m.Command, m.Method, m.Class, tag)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit the machine-readable catalog")
	return cmd
}

// emit writes v as one line of JSON to stdout, or to --output.
func (a *app) emit(cmd *cobra.Command, v any) error {
	raw, err := jsonCompact(v)
	if err != nil {
		return err
	}
	out, err := openOutput(flagString(cmd, "output"))
	if err != nil {
		return err
	}
	defer out.discard()
	return a.writeResponse(&api.Response{Body: append(raw, '\n')}, out)
}
```

- [ ] **Step 5: Implement `internal/cli/schemacmd.go`**

```go
package cli

import (
	"github.com/spf13/cobra"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/api"
)

type objectSummary struct {
	Command      string `json:"command"`
	NamePlural   string `json:"name_plural"`
	NameSingular string `json:"name_singular"`
	Description  string `json:"description,omitempty"`
	Fields       int    `json:"fields"`
}

func (a *app) schemaCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "schema [object]",
		Short: "List the workspace's objects, or print one object's fields (read live)",
		Long: "Without an argument, list the workspace's objects. With one, print that object's fields as JSON:\n" +
			"type, format, the allowed values of select fields (enum), subfields of composite fields, required\n" +
			"(needed on create), read_only (set by Twenty) and relations. The object is its command name or its\n" +
			"API name (note-targets or noteTargets). Always reads the workspace live and refreshes the cache;\n" +
			"works with any valid key, whatever its role.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a.applyGlobalFlags(cmd)
			m, err := a.fetchModel(cmd.Context())
			if err != nil {
				return err
			}
			a.model = m
			if len(args) == 0 {
				list := make([]objectSummary, 0, len(m.Objects))
				for _, o := range m.Objects {
					list = append(list, objectSummary{Command: o.Command, NamePlural: o.NamePlural, NameSingular: o.NameSingular,
						Description: o.Description, Fields: len(o.Fields)})
				}
				return a.emit(cmd, list)
			}
			obj := m.Find(args[0])
			if obj == nil {
				return api.Usagef("this workspace has no object %q; `twentycrm schema` lists them", args[0])
			}
			return a.emit(cmd, obj)
		},
	}
}
```

- [ ] **Step 6: Implement `internal/cli/apicmd.go`**

```go
package cli

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/api"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/routes"
)

// apiCommand is the escape hatch for anything the tree lacks. It is not a
// bypass: the path takes its class from the route grammar, and the gates are
// the same as for commands (spec: The api escape hatch).
func (a *app) apiCommand() *cobra.Command {
	var queries, headers []string
	cmd := &cobra.Command{
		Use:   "api <METHOD> <path>",
		Short: "Raw authenticated request (escape hatch); the guardrails still apply",
		Long: "Raw authenticated request to a path under rest/, relative to the base URL, for example\n" +
			"rest/companies or rest/metadata/objects. Query parameters come from --query k=v, never from the path.\n\n" +
			"The path takes its class from Twenty's route grammar: a DELETE without soft_delete=true deletes\n" +
			"permanently and needs --force, a PATCH or DELETE on a whole collection needs --filter and --force,\n" +
			"metadata changes need --force, API keys cannot be changed, and an unknown non-GET path needs --force.\n" +
			"Read-only mode allows read-class calls only. Authorization and the method-override headers cannot\n" +
			"be set; GET and DELETE take no --data; each path segment must be non-empty and must not be . or ..\n" +
			"or contain %, whitespace or a backslash. GraphQL is not supported.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			method := strings.ToUpper(args[0])
			q := url.Values{}
			for _, kv := range queries {
				k, v, ok := strings.Cut(kv, "=")
				if !ok || k == "" {
					return api.Usagef("--query wants k=v, got %q", kv)
				}
				q.Add(k, v)
			}
			hdr := http.Header{}
			for _, kv := range headers {
				k, v, ok := strings.Cut(kv, ":")
				if !ok || strings.TrimSpace(k) == "" {
					return api.Usagef("--header wants k:v, got %q", kv)
				}
				hdr.Add(strings.TrimSpace(k), strings.TrimSpace(v))
			}
			raw, err := routes.ClassifyRaw(method, args[1], q)
			if err != nil {
				return api.Usagef("%v", err)
			}
			path, _ := routes.CleanRawPath(args[1])
			body, err := a.readJSONBody(cmd)
			if err != nil {
				return err
			}
			if body != nil && (method == http.MethodGet || method == http.MethodDelete) {
				return api.Usagef("%s takes no request body; drop --data", method)
			}
			d := routes.Decision{Command: "api " + method + " " + path, Class: raw.Class, Blocked: raw.Blocked,
				FilterRequired: raw.FilterRequired, Filter: q.Get("filter"), ReadOnly: a.res.ReadOnly, Force: flagBool(cmd, "force")}
			if err := d.Check(); err != nil {
				return api.Usagef("%v", err)
			}
			return a.send(cmd, api.Request{Method: method, Path: path, Query: q, Body: body, Risk: raw.Class, Headers: hdr}, pager{})
		},
	}
	cmd.Flags().String("data", "", "JSON body: literal, @file, or - for stdin")
	cmd.Flags().StringArrayVar(&queries, "query", nil, "query parameter k=v (repeatable)")
	cmd.Flags().StringArrayVar(&headers, "header", nil, "extra header k:v (repeatable; Authorization cannot be set)")
	return cmd
}
```

- [ ] **Step 7: Implement `internal/cli/configcmd.go`**

```go
package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/api"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/auth"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/config"
)

func (a *app) configCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Manage the twentycrm config file", RunE: groupRunE}
	set := &cobra.Command{Use: "set", Short: "Set a config value", Args: cobra.ArbitraryArgs, RunE: keyGroupRunE("set")}
	set.AddCommand(
		&cobra.Command{
			Use:   "base-url <url>",
			Short: "Save the Twenty base URL, e.g. https://crm.example.com (Twenty Cloud: https://api.twenty.com)",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				n, err := config.NormalizeBaseURL(args[0])
				if err != nil {
					return api.Usagef("base-url: %v", err)
				}
				c, err := config.Load()
				if err != nil {
					return api.Usagef("%v", err)
				}
				if c.APIKey != "" && c.BaseURL != "" && c.BaseURL != n {
					return api.Usagef("the stored API key belongs to %s; run `twentycrm config unset api-key` first, then set the new base URL and its key", c.BaseURL)
				}
				c.BaseURL = n
				if err := config.Save(c); err != nil {
					return api.Usagef("%v", err)
				}
				a.warnEnv("TWENTY_BASE_URL")
				fmt.Fprintf(a.stdout, "base_url = %s\n", n)
				return nil
			},
		},
		&cobra.Command{
			Use:   "api-key",
			Short: "Read an API key from stdin, check it and store it (0600), bound to the stored base URL",
			Long: "Read a Twenty API key from stdin, check that it is an API key, and store it in the config file (0600).\n" +
				"The key never comes from the command line, which every process on the machine can see:\n" +
				"  twentycrm config set api-key < key.txt\n\n" +
				"Set the base URL first: the key is only ever sent to the base URL stored with it.\n" +
				"TWENTY_API_KEY, when set, wins.",
			Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := config.Load()
				if err != nil {
					return api.Usagef("%v", err)
				}
				if c.BaseURL == "" {
					return api.Usagef("set the base URL first: `twentycrm config set base-url <url>`; the key is bound to it")
				}
				raw, err := io.ReadAll(io.LimitReader(a.stdin, 64<<10))
				if err != nil {
					return api.Usagef("reading stdin: %v", err)
				}
				keyText := auth.CleanKey(string(raw))
				if keyText == "" {
					return api.Usagef("no key on stdin; usage: twentycrm config set api-key < key.txt")
				}
				key, err := auth.ParseAPIKey(keyText)
				if err != nil {
					return api.Usagef("%v", err)
				}
				c.APIKey = keyText
				if err := config.Save(c); err != nil {
					return api.Usagef("%v", err)
				}
				a.warnEnv("TWENTY_API_KEY")
				fmt.Fprintf(a.stdout, "api key saved for %s (workspace %s, key id %s, %s)\n", c.BaseURL, key.WorkspaceID, key.KeyID, expiryText(key))
				return nil
			},
		},
		&cobra.Command{
			Use:   "read-only <true|false>",
			Short: "Persist read-only mode",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				v, err := strconv.ParseBool(args[0])
				if err != nil {
					return api.Usagef("read-only wants true or false, got %q", args[0])
				}
				c, err := config.Load()
				if err != nil {
					return api.Usagef("%v", err)
				}
				c.ReadOnly = v
				if err := config.Save(c); err != nil {
					return api.Usagef("%v", err)
				}
				fmt.Fprintf(a.stdout, "read_only = %v\n", v)
				return nil
			},
		},
	)

	unset := &cobra.Command{Use: "unset", Short: "Remove a config value", Args: cobra.ArbitraryArgs, RunE: keyGroupRunE("unset")}
	for _, k := range []struct {
		use, short, done string
		stored           func(config.Config) bool
		refuse           func(config.Config) string
		clear            func(*config.Config)
	}{
		{"base-url", "Remove the stored base URL", "base-url removed",
			func(c config.Config) bool { return c.BaseURL != "" },
			func(c config.Config) string {
				if c.APIKey != "" {
					return "the stored API key is bound to this base URL; run `twentycrm config unset api-key` first"
				}
				return ""
			},
			func(c *config.Config) { c.BaseURL = "" }},
		{"api-key", "Remove the stored API key from this machine",
			"api key removed from this machine; it stays valid until you revoke it in Twenty under Settings, APIs & Webhooks",
			func(c config.Config) bool { return c.APIKey != "" }, func(config.Config) string { return "" },
			func(c *config.Config) { c.APIKey = "" }},
		{"read-only", "Stop persisting read-only mode", "read-only removed",
			func(c config.Config) bool { return c.ReadOnly }, func(config.Config) string { return "" },
			func(c *config.Config) { c.ReadOnly = false }},
	} {
		unset.AddCommand(&cobra.Command{
			Use: k.use, Short: k.short, Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := config.Load()
				if err != nil {
					return api.Usagef("%v", err)
				}
				if !k.stored(c) {
					fmt.Fprintf(a.stdout, "%s was not set\n", k.use)
					return nil
				}
				if why := k.refuse(c); why != "" {
					return api.Usagef("%s", why)
				}
				k.clear(&c)
				if err := config.Save(c); err != nil {
					return api.Usagef("%v", err)
				}
				fmt.Fprintln(a.stdout, k.done)
				return nil
			},
		})
	}

	path := &cobra.Command{
		Use: "path", Short: "Print the config file location", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := config.Path()
			if err != nil {
				return api.Usagef("%v", err)
			}
			fmt.Fprintln(a.stdout, p)
			return nil
		},
	}
	cmd.AddCommand(set, unset, path)
	return cmd
}

// expiryText renders a key's expiry for people.
func expiryText(k *auth.Key) string {
	if k.ExpiresAt.IsZero() {
		return "never expires"
	}
	return "expires " + k.ExpiresAt.Format("2006-01-02")
}

// warnEnv says on stderr when an environment variable overrides what was
// just saved.
func (a *app) warnEnv(name string) {
	if a.res.FromEnv[name] {
		fmt.Fprintf(a.stderr, "note: %s is set in this environment and wins over the config file\n", name)
	}
}

// keyGroupRunE answers `config set` or `config unset` given something that is
// not one of their keys. An environment variable name is the expected
// mistake, so it is answered with the key it plainly means.
func keyGroupRunE(verb string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			if k := suggestConfigKey(args[0]); k != "" {
				return api.Usagef("config keys are base-url, api-key and read-only, not environment variable names; "+
					"did you mean `twentycrm config %s %s`?", verb, k)
			}
		}
		return groupRunE(cmd, args)
	}
}

func suggestConfigKey(given string) string {
	switch strings.TrimPrefix(strings.ToLower(given), "twenty_") {
	case "base_url", "baseurl", "url", "base":
		return "base-url"
	case "api_key", "apikey", "key", "token":
		return "api-key"
	case "read_only", "readonly":
		return "read-only"
	}
	return ""
}
```

- [ ] **Step 8: Implement `internal/cli/authcmd.go`**

```go
package cli

import (
	"math"
	"time"

	"github.com/spf13/cobra"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/auth"
)

// authStatus is `twentycrm auth status`: what is configured, never the key.
type authStatus struct {
	Mode          string   `json:"mode"` // api_key or none
	Source        string   `json:"source,omitempty"`
	BaseURL       string   `json:"base_url,omitempty"`
	BaseURLSource string   `json:"base_url_source,omitempty"`
	WorkspaceID   string   `json:"workspace_id,omitempty"`
	KeyID         string   `json:"key_id,omitempty"`
	ExpiresAt     string   `json:"expires_at,omitempty"`
	ExpiresInDays *int     `json:"expires_in_days,omitempty"`
	ExpiresSoon   bool     `json:"expires_soon"`
	Expired       bool     `json:"expired"`
	ReadOnly      bool     `json:"read_only"`
	KeyError      string   `json:"key_error,omitempty"`
	Missing       []string `json:"missing,omitempty"`
	Hint          string   `json:"hint,omitempty"`
}

const expirySoon = 14 * 24 * time.Hour

func (a *app) authCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "auth", Short: "Inspect the configured API key", RunE: groupRunE}
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show the configured key as one line of JSON: workspace, key ID, expiry (offline; never prints the key)",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, args []string) error { return a.emit(cmd, a.authStatus()) },
	})
	return cmd
}

func (a *app) authStatus() authStatus {
	r := a.res
	s := authStatus{Mode: "none", BaseURL: r.BaseURL, BaseURLSource: r.BaseURLSource, ReadOnly: r.ReadOnly, Missing: r.Missing()}
	if r.APIKey != "" {
		s.Mode, s.Source = "api_key", r.KeySource
		if k, err := auth.ParseAPIKey(r.APIKey); err != nil {
			s.KeyError = err.Error()
		} else {
			s.WorkspaceID, s.KeyID = k.WorkspaceID, k.KeyID
			if !k.ExpiresAt.IsZero() {
				now := a.clock()
				left := k.ExpiresAt.Sub(now)
				days := int(math.Floor(left.Hours() / 24))
				s.ExpiresAt, s.ExpiresInDays = k.ExpiresAt.Format(time.RFC3339), &days
				s.Expired = k.Expired(now)
				s.ExpiresSoon = !s.Expired && left < expirySoon
			}
		}
	}
	switch {
	case len(s.Missing) > 0:
		s.Hint = "run `twentycrm init`, or set TWENTY_BASE_URL and TWENTY_API_KEY"
	case r.BindingError() != nil:
		s.Hint = r.BindingError().Error()
	case s.KeyError != "":
		s.Hint = "the key cannot be read; create an API key in Twenty under Settings, APIs & Webhooks"
	case s.Expired:
		s.Hint = "the key has expired: create a new one in Twenty under Settings, APIs & Webhooks, then run `twentycrm config set api-key`"
	case s.ExpiresSoon:
		s.Hint = "the key expires soon: create a new one in Twenty under Settings, APIs & Webhooks before " + s.ExpiresAt[:10]
	}
	return s
}
```

- [ ] **Step 9: Implement `internal/cli/initcmd.go`**

```go
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"

	"github.com/spf13/cobra"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/api"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/auth"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/config"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/model"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/routes"
)

func (a *app) initCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Interactive setup: base URL, API key, read-only mode (needs a terminal)",
		Long: "Interactive setup. Asks for the Twenty base URL and an API key (hidden input), checks both with two\n" +
			"read-only calls, asks whether to switch on read-only mode, and saves everything to the config file\n" +
			"(0600). Needs a terminal: agents configure twentycrm with `twentycrm config set` or the environment.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			isTTY := a.isTerminal
			if isTTY == nil {
				isTTY = stdioIsTerminal
			}
			if !isTTY() {
				return api.Usagef("twentycrm init is interactive and needs a terminal; use `twentycrm config set ...` instead")
			}
			p := a.prompt
			if p == nil {
				p = newTermPrompter(os.Stdin, a.stdout)
			}
			return a.runInit(cmd.Context(), p)
		},
	}
}

func (a *app) runInit(ctx context.Context, p prompter) error {
	out := a.stdout
	fmt.Fprintln(out, "twentycrm setup: you need your Twenty address and an API key (Twenty: Settings, APIs & Webhooks).")
	for _, name := range []string{"TWENTY_BASE_URL", "TWENTY_API_KEY", "TWENTY_READ_ONLY"} {
		if a.res.FromEnv[name] {
			fmt.Fprintf(out, "Note: %s is set in this shell and wins over what you save here.\n", name)
		}
	}
	cur, err := config.Load()
	if err != nil {
		return api.Usagef("%v", err)
	}
	base, err := askValid(p, out, "Twenty base URL (e.g. https://crm.example.com; Twenty Cloud: https://api.twenty.com): ", config.NormalizeBaseURL)
	if err != nil {
		return err
	}
	if cur.APIKey != "" && cur.BaseURL != "" && cur.BaseURL != base {
		ok, err := p.Confirm(fmt.Sprintf("The config file holds a key for %s. Replace it?", cur.BaseURL), false)
		if err != nil || !ok {
			return api.Usagef("init: nothing was saved")
		}
	}
	keyText, key, err := a.askKey(p)
	if err != nil {
		return err
	}
	client := &api.Client{BaseURL: base, APIKey: keyText, Sleep: a.client.Sleep}
	resp, err := client.Do(ctx, api.Request{Method: "GET", Path: "rest/open-api/core", Risk: routes.ClassRead})
	if err != nil {
		fmt.Fprintln(out, "Verification failed; nothing was saved.")
		return err
	}
	objs, err := model.Extract(resp.Body)
	if err != nil {
		fmt.Fprintln(out, "Verification failed; nothing was saved.")
		if errors.Is(err, model.ErrNoObjects) {
			return &api.Error{Kind: api.KindAuth, Message: "init: " + err.Error()}
		}
		return &api.Error{Kind: api.KindServer, Message: "init: " + err.Error()}
	}
	count := "company count unavailable"
	if r, err := client.Do(ctx, api.Request{Method: "GET", Path: "rest/companies", Risk: routes.ClassRead,
		Query: url.Values{"limit": {"1"}, "depth": {"0"}}}); err == nil {
		var env struct {
			TotalCount *int `json:"totalCount"`
		}
		if json.Unmarshal(r.Body, &env) == nil && env.TotalCount != nil {
			count = fmt.Sprintf("%d companies", *env.TotalCount)
		}
	}
	fmt.Fprintf(out, "Workspace %s: %d objects, %s. Key %s %s.\n", key.WorkspaceID, len(objs), count, key.KeyID, expiryText(key))
	ro, err := p.Confirm("Switch on read-only mode (reads only, no changes)?", false)
	if err != nil {
		return api.Usagef("init: %v; nothing was saved", err)
	}
	cur.BaseURL, cur.APIKey, cur.ReadOnly = base, keyText, ro
	if err := config.Save(cur); err != nil {
		return api.Usagef("%v", err)
	}
	if root := a.cacheRoot(); root != "" {
		_ = model.SaveCache(root, &model.Model{FetchedAt: a.clock().UTC(), BaseURL: base, WorkspaceID: key.WorkspaceID, Objects: objs})
	}
	p2, _ := config.Path()
	fmt.Fprintf(out, "Saved to %s. Next: twentycrm schema\n", p2)
	return nil
}

// askKey asks for the key until it is an unexpired API key, three times at
// most. Nothing is sent before it passes.
func (a *app) askKey(p prompter) (string, *auth.Key, error) {
	for range 3 {
		raw, err := ask(p, "API key (input hidden): ", true)
		if err != nil {
			return "", nil, api.Usagef("init: %v", err)
		}
		text := auth.CleanKey(raw)
		key, err := auth.ParseAPIKey(text)
		if err != nil {
			fmt.Fprintln(a.stdout, err)
			continue
		}
		if key.Expired(a.clock()) {
			fmt.Fprintf(a.stdout, "This key expired on %s; create a new one in Twenty under Settings, APIs & Webhooks.\n", key.ExpiresAt.Format("2006-01-02"))
			continue
		}
		return text, key, nil
	}
	return "", nil, api.Usagef("init: no usable API key after 3 attempts; nothing was saved")
}

// askValid asks until parse accepts the answer, three times at most.
func askValid[T any](p prompter, out io.Writer, prompt string, parse func(string) (T, error)) (T, error) {
	var zero T
	for range 3 {
		s, err := ask(p, prompt, false)
		if err != nil {
			return zero, api.Usagef("init: %v", err)
		}
		v, err := parse(s)
		if err == nil {
			return v, nil
		}
		fmt.Fprintln(out, err)
	}
	return zero, api.Usagef("init: no valid answer after 3 attempts")
}
```

- [ ] **Step 10: Run all tests and vet**

Run: `go mod tidy && go vet ./... && go test ./...`
Expected: PASS.

- [ ] **Step 11: Commit**

```bash
git add internal/cli go.mod go.sum
git commit -m "Add commands, schema, api, config, auth and init

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---
### Task 5: End-to-end tests, live check, documentation, CI and release

**Files:**
- Create: `e2e/helpers_test.go`, `e2e/smoke_test.go`, `e2e/live_test.go`
- Create: `README.md`, `SECURITY.md`, `docs/agents.md`
- Create: `.github/workflows/ci.yml`, `.github/workflows/release.yml`, `.goreleaser.yaml`

**Interfaces:**
- Consumes: the built binary (`github.com/wir-drei-digital/twenty-crm-cli/cmd/twentycrm`) and the fixture `internal/model/testdata/openapi-core.json`. The e2e package imports nothing from `internal/`.
- Produces: nothing other tasks use.

- [ ] **Step 1: Write `e2e/helpers_test.go`**

```go
// Package e2e drives the real binary as a subprocess: argv parsing, exit
// codes, stdout and stderr separation, the config file and the model cache,
// exactly as a user gets them. Everything else in the repo tests in-process.
package e2e

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "twentycrm")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	out, err := exec.Command("go", "build", "-o", bin, "github.com/wir-drei-digital/twenty-crm-cli/cmd/twentycrm").CombinedOutput()
	if err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}

// run executes the binary with exactly env and returns stdout, stderr and the exit code.
func run(t *testing.T, bin string, env []string, stdin string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return stdout.String(), stderr.String(), code
}

// scrubbedEnv drops every TWENTY_* variable and points every per-user
// directory into a temp dir, so a developer's real key, config or cache can
// never turn these tests green or red.
func scrubbedEnv(t *testing.T, extra ...string) []string {
	t.Helper()
	home := t.TempDir()
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "TWENTY_") {
			env = append(env, kv)
		}
	}
	env = append(env, "HOME="+home, "USERPROFILE="+home,
		"XDG_CONFIG_HOME="+filepath.Join(home, "config"), "XDG_CACHE_HOME="+filepath.Join(home, "cache"),
		"AppData="+filepath.Join(home, "appdata"), "LocalAppData="+filepath.Join(home, "localappdata"))
	return append(env, extra...)
}

// apiKey is an unsigned API key for workspace ws-e2e; the CLI never verifies
// signatures, and the fake Twenty compares the string.
func apiKey() string {
	enc := func(v any) string {
		raw, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	return enc(map[string]string{"alg": "HS256", "typ": "JWT"}) + "." +
		enc(map[string]any{"sub": "ws-e2e", "type": "API_KEY", "workspaceId": "ws-e2e", "jti": "key-e2e",
			"exp": time.Now().Add(60 * 24 * time.Hour).Unix()}) + ".c2ln"
}

// errorLine parses the single JSON error line on stderr.
func errorLine(t *testing.T, stderr string) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimRight(stderr, "\n"), "\n")
	var m map[string]any
	if len(lines) != 1 || json.Unmarshal([]byte(lines[0]), &m) != nil {
		t.Fatalf("stderr is not one JSON line: %q", stderr)
	}
	return m
}
```

- [ ] **Step 2: Write `e2e/smoke_test.go`**

```go
package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakeTwenty is an in-memory Twenty with one object, companies: enough of
// the record routes to drive the binary through create, read, update, trash,
// restore and destroy. It serves the fixture OpenAPI document for the model.
func fakeTwenty(t *testing.T) *httptest.Server {
	t.Helper()
	doc, err := os.ReadFile(filepath.Join("..", "internal", "model", "testdata", "openapi-core.json"))
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	records := map[string]map[string]any{}
	trashed := map[string]bool{}
	var order []string
	seq := 0
	write := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(v)
	}
	notFound := func(w http.ResponseWriter) {
		write(w, 404, map[string]any{"statusCode": 404, "messages": []string{"Record not found"}, "error": "NOT_FOUND"})
	}
	key := apiKey()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/rest/open-api/core" {
			w.Write(doc) // public endpoint: answers everyone
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+key {
			write(w, 401, map[string]any{"statusCode": 401, "messages": []string{"Token invalid"}, "error": "UNAUTHENTICATED"})
			return
		}
		p := r.URL.Path
		switch {
		case p == "/rest/companies" && r.Method == http.MethodGet:
			rows := []map[string]any{}
			for _, id := range order {
				if !trashed[id] {
					rows = append(rows, records[id])
				}
			}
			write(w, 200, map[string]any{"data": map[string]any{"companies": rows}, "totalCount": len(rows),
				"pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""}})
		case p == "/rest/companies" && r.Method == http.MethodPost:
			var rec map[string]any
			json.NewDecoder(r.Body).Decode(&rec)
			seq++
			id := fmt.Sprintf("00000000-0000-4000-8000-%012d", seq)
			rec["id"] = id
			records[id] = rec
			order = append(order, id)
			write(w, 201, map[string]any{"data": map[string]any{"createCompany": rec}})
		case strings.HasPrefix(p, "/rest/restore/companies/") && r.Method == http.MethodPatch:
			id := path.Base(p)
			if records[id] == nil {
				notFound(w)
				return
			}
			delete(trashed, id)
			write(w, 200, map[string]any{"data": map[string]any{"restoreCompany": records[id]}})
		case strings.HasPrefix(p, "/rest/companies/"):
			id := path.Base(p)
			rec := records[id]
			if rec == nil || (trashed[id] && r.Method != http.MethodDelete) {
				notFound(w)
				return
			}
			switch r.Method {
			case http.MethodGet:
				write(w, 200, map[string]any{"data": map[string]any{"company": rec}})
			case http.MethodPatch:
				var patch map[string]any
				json.NewDecoder(r.Body).Decode(&patch)
				for k, v := range patch {
					rec[k] = v
				}
				write(w, 200, map[string]any{"data": map[string]any{"updateCompany": rec}})
			case http.MethodDelete:
				if r.URL.Query().Get("soft_delete") == "true" {
					trashed[id] = true
				} else {
					delete(records, id)
					delete(trashed, id)
					for i, o := range order {
						if o == id {
							order = append(order[:i], order[i+1:]...)
							break
						}
					}
				}
				write(w, 200, map[string]any{"data": map[string]any{"deleteCompany": map[string]any{"id": id}}})
			}
		default:
			notFound(w)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSmokeRecordLifecycle(t *testing.T) {
	bin := buildBinary(t)
	srv := fakeTwenty(t)
	env := scrubbedEnv(t, "TWENTY_BASE_URL="+srv.URL, "TWENTY_API_KEY="+apiKey())

	if out, _, code := run(t, bin, env, "", "version"); code != 0 || !strings.HasPrefix(out, "twentycrm ") {
		t.Fatalf("version: %d %q", code, out)
	}
	out, _, code := run(t, bin, env, "", "commands", "--json")
	if code != 0 || !strings.Contains(out, `"model_missing":true`) {
		t.Fatalf("catalog before any model: %d %s", code, out)
	}
	out, errOut, code := run(t, bin, env, "", "schema")
	var objs []map[string]any
	if code != 0 || json.Unmarshal([]byte(out), &objs) != nil || len(objs) != 4 {
		t.Fatalf("schema: %d %s %s", code, out, errOut)
	}

	out, errOut, code = run(t, bin, env, `{"name":"Acme"}`, "companies", "create", "--data", "-")
	var created struct {
		Data struct {
			CreateCompany struct {
				ID string `json:"id"`
			} `json:"createCompany"`
		} `json:"data"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &created) != nil || created.Data.CreateCompany.ID == "" {
		t.Fatalf("create: %d %s %s", code, out, errOut)
	}
	id := created.Data.CreateCompany.ID

	steps := []struct {
		args []string
		code int
		kind string
	}{
		{[]string{"companies", "get", id}, 0, ""},
		{[]string{"companies", "update", id, "--data", `{"employees":5}`}, 0, ""},
		{[]string{"companies", "delete", id}, 0, ""},
		{[]string{"companies", "get", id}, 1, "not_found"},
		{[]string{"companies", "restore", id}, 0, ""},
		{[]string{"companies", "get", id}, 0, ""},
		{[]string{"companies", "destroy", id}, 2, "usage"},
		{[]string{"companies", "destroy", id, "--force"}, 0, ""},
		{[]string{"companies", "get", id}, 1, "not_found"},
		{[]string{"companies", "update-many", "--data", `{}`, "--force"}, 2, "usage"},
		{[]string{"companies", "get", "not-a-uuid"}, 2, "usage"},
		{[]string{"widgets", "list"}, 2, "usage"},
	}
	for _, s := range steps {
		out, errOut, code := run(t, bin, env, "", s.args...)
		if code != s.code {
			t.Fatalf("%v: exit %d, want %d\n%s\n%s", s.args, code, s.code, out, errOut)
		}
		if s.kind != "" {
			if e := errorLine(t, errOut); e["kind"] != s.kind || out != "" {
				t.Fatalf("%v: error %v, stdout %q", s.args, e, out)
			}
		}
	}

	out, _, code = run(t, bin, env, `{"name":"Beta"}`, "companies", "create", "--data", "-")
	if code != 0 {
		t.Fatal("second create failed")
	}
	out, _, code = run(t, bin, env, "", "companies", "list", "--all")
	var rows []map[string]any
	if code != 0 || json.Unmarshal([]byte(out), &rows) != nil || len(rows) != 1 || rows[0]["name"] != "Beta" {
		t.Fatalf("list --all: %d %s", code, out)
	}

	ro := append(append([]string{}, env...), "TWENTY_READ_ONLY=1")
	if _, errOut, code := run(t, bin, ro, `{"name":"X"}`, "companies", "create", "--data", "-"); code != 2 || errorLine(t, errOut)["kind"] != "usage" {
		t.Fatalf("read-only create: %d %s", code, errOut)
	}
}

func TestSmokeConfigFileAndBinding(t *testing.T) {
	bin := buildBinary(t)
	srv := fakeTwenty(t)
	env := scrubbedEnv(t)
	if _, errOut, code := run(t, bin, env, "", "config", "set", "base-url", srv.URL); code != 0 {
		t.Fatalf("set base-url: %d %s", code, errOut)
	}
	if _, errOut, code := run(t, bin, env, "Bearer "+apiKey()+"\n", "config", "set", "api-key"); code != 0 {
		t.Fatalf("set api-key: %d %s", code, errOut)
	}
	out, _, code := run(t, bin, env, "", "auth", "status")
	var st map[string]any
	if code != 0 || json.Unmarshal([]byte(out), &st) != nil || st["source"] != "config" || st["workspace_id"] != "ws-e2e" || strings.Contains(out, apiKey()) {
		t.Fatalf("auth status: %d %s", code, out)
	}
	if _, errOut, code := run(t, bin, env, "", "companies", "list"); code != 0 {
		t.Fatalf("list with the stored key: %d %s", code, errOut)
	}
	elsewhere := append(append([]string{}, env...), "TWENTY_BASE_URL=https://elsewhere.example.com")
	if _, errOut, code := run(t, bin, elsewhere, "", "companies", "list"); code != 2 || !strings.Contains(errorLine(t, errOut)["error"].(string), "belongs to") {
		t.Fatalf("binding: %d %s", code, errOut)
	}
}
```

- [ ] **Step 3: Write `e2e/live_test.go`**

```go
package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLive runs against a real Twenty with the operator's own configuration
// (config file or TWENTY_* variables). It never runs in CI.
//
//	TWENTY_LIVE=1                     read-only checks
//	TWENTY_LIVE_SELECT_FIELD=<field>  a select field of companies whose values must be listed
//	TWENTY_LIVE_RESTRICTED_KEY=<key>  a key whose role lacks the Data Model permission
//	TWENTY_LIVE_WRITE=1               also create, change and destroy one marked test company
func TestLive(t *testing.T) {
	if os.Getenv("TWENTY_LIVE") != "1" {
		t.Skip("set TWENTY_LIVE=1 with a configured key to run against a real Twenty")
	}
	bin := buildBinary(t)
	env := os.Environ()
	jsonOf := func(args ...string) map[string]any {
		t.Helper()
		out, errOut, code := run(t, bin, env, "", args...)
		var m map[string]any
		if code != 0 || json.Unmarshal([]byte(out), &m) != nil {
			t.Fatalf("%v: exit %d\n%s\n%s", args, code, out, errOut)
		}
		return m
	}

	st := jsonOf("auth", "status")
	if st["mode"] != "api_key" || st["workspace_id"] == nil {
		t.Fatalf("auth status: %v", st)
	}
	t.Logf("workspace %v, key expires %v", st["workspace_id"], st["expires_at"])

	schema := jsonOf("schema", "companies")
	fields, _ := schema["fields"].([]any)
	if len(fields) == 0 {
		t.Fatalf("schema companies: %v", schema)
	}
	if name := os.Getenv("TWENTY_LIVE_SELECT_FIELD"); name != "" {
		found := false
		for _, f := range fields {
			fm := f.(map[string]any)
			if fm["name"] == name {
				enum, _ := fm["enum"].([]any)
				found = len(enum) > 0
				t.Logf("%s values: %v", name, enum)
			}
		}
		if !found {
			t.Fatalf("companies.%s lists no select values", name)
		}
	}

	total := int(jsonOf("companies", "list", "--limit", "1", "--depth", "0")["totalCount"].(float64))
	if total <= 20000 {
		out, errOut, code := run(t, bin, env, "", "companies", "list", "--all", "--depth", "0")
		var rows []any
		if code != 0 || json.Unmarshal([]byte(out), &rows) != nil || len(rows) != total {
			t.Fatalf("list --all: exit %d, %d rows, totalCount %d\n%s", code, len(rows), total, errOut)
		}
	}

	if key := os.Getenv("TWENTY_LIVE_RESTRICTED_KEY"); key != "" {
		restricted := append(append([]string{}, env...), "TWENTY_API_KEY="+key, fmt.Sprintf("TWENTY_BASE_URL=%v", st["base_url"]))
		if _, errOut, code := run(t, bin, restricted, "", "schema"); code != 0 {
			t.Fatalf("schema with a restricted key: %d %s", code, errOut)
		}
		_, errOut, code := run(t, bin, restricted, "", "metadata", "objects", "list")
		if code != 1 || errorLine(t, errOut)["kind"] != "forbidden" || !strings.Contains(errOut, "Data Model") {
			t.Fatalf("metadata objects with a restricted key: %d %s", code, errOut)
		}
	}

	if os.Getenv("TWENTY_LIVE_WRITE") != "1" {
		return
	}
	name := fmt.Sprintf("twentycrm-live-%d", time.Now().Unix())
	out, errOut, code := run(t, bin, env, fmt.Sprintf(`{"name":%q}`, name), "companies", "create", "--data", "-")
	var created map[string]map[string]map[string]any
	if code != 0 || json.Unmarshal([]byte(out), &created) != nil {
		t.Fatalf("create: %d %s %s", code, out, errOut)
	}
	id, _ := created["data"]["createCompany"]["id"].(string)
	if id == "" {
		t.Fatalf("create returned no id: %s", out)
	}
	t.Cleanup(func() { run(t, bin, env, "", "companies", "destroy", id, "--force") })
	filter := fmt.Sprintf("name[eq]:%q", name)
	count := func() int {
		m := jsonOf("companies", "list", "--filter", filter, "--depth", "0")
		return int(m["totalCount"].(float64))
	}
	jsonOf("companies", "get", id)
	jsonOf("companies", "update", id, "--data", `{"employees":1}`)
	jsonOf("companies", "delete", id)
	if n := count(); n != 0 {
		t.Fatalf("a trashed company is still listed (%d)", n)
	}
	jsonOf("companies", "restore", id)
	if n := count(); n != 1 {
		t.Fatalf("the restored company is not listed (%d)", n)
	}
	jsonOf("companies", "destroy", id, "--force")
	if _, errOut, code := run(t, bin, env, "", "companies", "get", id); code != 1 || errorLine(t, errOut)["kind"] != "not_found" {
		t.Fatalf("get after destroy: %d %s", code, errOut)
	}
}
```

- [ ] **Step 4: Run the e2e tests**

Run: `go test ./e2e/ -v`
Expected: `TestSmokeRecordLifecycle` and `TestSmokeConfigFileAndBinding` PASS; `TestLive` SKIP.

- [ ] **Step 5: Write `docs/agents.md`**

````markdown
# twentycrm: agent reference

Paste the block below into your agent's instructions. It is the whole contract.

```markdown
# twentycrm (Twenty CRM CLI): agent reference

Setup is a person's job. Never run `twentycrm init` or `twentycrm config set`, and never pass
`--force` unless a person told you to, in this conversation, for this exact call.

Before you write
- `twentycrm schema <object>`: the object's fields as JSON, read live. `enum` lists the only
  values a select field accepts; `required` fields are needed on create; `read_only` fields are
  set by Twenty; `subfields` are the parts of a composite field (emails.primaryEmail); a
  many_to_one relation is set through `<name>Id`.
- `twentycrm schema` lists the objects; `twentycrm commands --json` is the full catalog.

Commands: `twentycrm <object> <verb> [id] [flags]`, object = plural name in kebab case
(companies, people, tasks, notes, note-targets, task-targets, or a custom object).
- Read: `list` (--filter, --order-by, --limit up to 200, --depth 0|1, --all), `get <id>`,
  `group-by --group-by '[{"city":true}]'`, `find-duplicates --data '{"ids":["<id>"]}'`.
- Write: `create --data '{...}'`, `batch-create --data '[...]'` (at most 60 records),
  `update <id> --data '{...}'`, `delete <id>` (to the trash), `restore <id>`.
- Needs --force: `update-many`, `delete-many`, `restore-many` (all need --filter), `merge`,
  `destroy <id>` and `destroy-many` (permanent), and `metadata ... create/update/delete`.
- `merge --dry-run` previews a merge without --force.
- `--data` takes a JSON literal, `@file.json` or `-` for stdin. Build bulk payloads with a
  script from the source file; never type records from memory.

Filters: field[comparator]:value, commas mean "and", or(...) and not(...) combine.
Comparators: eq neq in containsAny is gt gte lt lte startsWith endsWith like ilike.
  --filter 'name[ilike]:"%acme%"'
  --filter 'or(stage[eq]:LEAD,employees[gt]:50)'
Order: --order-by 'createdAt[DescNullsLast],name'. Use --depth 0 unless you need relations.

Linking: a note or task is linked to a record through note-targets or task-targets:
  twentycrm notes create --data '{"title":"Call","bodyV2":{"markdown":"..."}}'
  twentycrm note-targets create --data '{"noteId":"<note id>","companyId":"<company id>"}'

Output and errors
- stdout is the API response, untouched; `--all` prints one JSON array of the records.
- Exit 0 success, 1 API or network error, 2 usage error or refused by a guard (nothing sent).
- stderr is one JSON line: {"kind","error","status","details"}. Kinds: auth, forbidden,
  not_found, validation, conflict, rate_limited, server, transport, outcome_unknown,
  incomplete, usage, output_failed.
- outcome_unknown: a write may or may not have happened; read before you retry.
- incomplete: --all stopped at --max-pages; the output is partial.
- The error message ends with a hint when one applies (expired key, missing permission, unknown
  field). Twenty allows 100 requests per minute; the CLI waits and retries on 429 by itself.
```

## Notes for the person wiring this up

- Give the agent a key whose role has exactly the objects it needs (Twenty: Settings, Roles).
  The role is the real boundary; `--force` only stops mistakes.
- `twentycrm auth status` shows the key's expiry and flags it 14 days ahead. Put the date in a
  calendar.
- `TWENTY_READ_ONLY=1` makes a runner read-only whatever its key allows.
````

The `notes create` example uses the field names of Twenty's standard `notes` object; check them with `twentycrm schema notes` against a real workspace during the live check and correct the example if they differ.

- [ ] **Step 6: Write `SECURITY.md`**

Sections, in this order, in plain prose without em dashes:

1. `# Security`.
2. `## What an API key is`: a JWT that Twenty signs; it belongs to exactly one workspace; it carries the expiry chosen when it was created (or none) and the role assigned to it; anyone who holds it can do what its role allows, from any machine; it is shown once when it is created; it is revoked in Twenty under Settings, APIs & Webhooks. `twentycrm auth status` reads the workspace, the key ID and the expiry from it without sending it anywhere.
3. `## Where the key lives`: in the config file (`twentycrm config path`, 0600 in a 0700 directory, written through a temp file and rename) or in `TWENTY_API_KEY`; the environment wins. It never comes from the command line, which other processes can read. One key per machine or runner is easier to revoke than one shared key.
4. `## The base URL binding`: a key from the config file is only ever sent to the base URL stored with it; `TWENTY_BASE_URL` pointing elsewhere refuses every call. A key and a base URL that both come from the environment are used as given. HTTPS is required except for localhost, 127.0.0.1 and ::1.
5. `## What the CLI does not do`: follow redirects; log the key (`--verbose` prints methods and paths only); take a key from argv; create, change or revoke API keys; delete permanently or change records in bulk without `--force`; verify the key's signature (it cannot; only Twenty can).
6. `## Reporting a vulnerability`: the same text as google-ads-cli's `SECURITY.md` with `twentycrm version` in place of `googleads version`.

- [ ] **Step 7: Write `README.md`**

Sections, in this order, in plain prose without em dashes. Every command shown must work as written against the fixture workspace (objects companies, people, note-targets, invoices) or a standard Twenty workspace.

1. `# twentycrm`: one paragraph (a CLI for the Twenty CRM REST API, built for agents: JSON in, JSON out; every object of your workspace, custom ones included, becomes a command; nothing is deleted for good, changed in bulk or restructured without `--force`), then: "Not affiliated with or endorsed by Twenty." Name the tested Twenty version (2.27) and state that the binary is `twentycrm` because Twenty's own `twenty-sdk` installs `twenty`.
2. `## Install`: `go install github.com/wir-drei-digital/twenty-crm-cli/cmd/twentycrm@latest`, or a release binary from GitHub Releases (darwin amd64/arm64, linux amd64/arm64, windows amd64) with `checksums.txt`.
3. `## Setup (once per workspace)`: the four operator steps from the spec (*Operator setup*): a role with the needed objects, an API key with that role and an expiry, `twentycrm init` or `config set base-url` plus `config set api-key < key.txt` or the environment variables, note the expiry. Twenty Cloud's base URL is `https://api.twenty.com`; a self-hosted one is the address you open in the browser, without a path.
4. `## Usage`: the command grammar and the verb table from the spec (*Object commands*), then examples: `companies list --filter 'name[ilike]:"%acme%"' --depth 0`, `companies list --all`, `people get <id>`, `companies create --data @company.json`, linking a note through `note-targets create`, `companies update-many --filter 'city[eq]:Bern' --data '{"tagline":"x"}' --force`, `delete` versus `destroy --force`, `merge --dry-run`, `schema companies`, `commands --json`, `metadata objects list`, `api GET rest/companies --query limit=1`. Filter and order syntax with the comparator list.
5. `## The workspace model`: where it comes from (`GET /rest/open-api/core`, which any valid key may read), what it holds, where it is cached (`<user cache dir>/twentycrm/`), when it refreshes (missing, older than 24 hours, an unknown object name, every `schema` call), and name collisions with built-ins.
6. `## Guardrails`: the risk class table, the local checks, read-only mode, the `api` classification in short, and the section *What the CLI cannot do* (the key's role is the boundary).
7. `## Errors and exit codes`: exit codes, the error line, the kinds and the status mapping, the note that Twenty answers unhandled server errors with 400, the hints, the retry policy, `--timeout`.
8. `## Configuration`: file location, keys (`base-url`, `api-key`, `read-only`), the three environment variables, the binding rule, `auth status` fields.
9. `## Compatibility`: tested against Twenty 2.27; SemVer; the public contract (command grammar, flags, exit codes, error schema, `commands --json` schema); after a Twenty upgrade run the read-only live check.
10. `## Development`: `make check`; `TWENTY_LIVE=1 go test ./e2e -run TestLive -v` and the other `TWENTY_LIVE_*` variables from `e2e/live_test.go`.
11. `## License`: MIT.

- [ ] **Step 8: Write the CI and release configuration**

`.github/workflows/ci.yml`:

```yaml
name: CI
on:
  push:
    branches: [main]
  pull_request:

permissions:
  contents: read

jobs:
  test:
    strategy:
      fail-fast: false
      matrix:
        os: [ubuntu-latest, macos-latest, windows-latest]
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v5
      - uses: actions/setup-go@v6
        with:
          go-version: stable
      - run: go vet ./...
      - run: go test ./...

  cross-compile:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v5
      - uses: actions/setup-go@v6
        with:
          go-version: stable
      - name: cross-compile
        env:
          CGO_ENABLED: "0"
        run: |
          GOOS=linux  GOARCH=arm64 go build ./cmd/twentycrm
          GOOS=darwin GOARCH=arm64 go build ./cmd/twentycrm
          GOOS=windows GOARCH=amd64 go build ./cmd/twentycrm
```

`.github/workflows/release.yml`: copy `/Users/daniel/Development/google-ads-cli/.github/workflows/release.yml` unchanged.

`.goreleaser.yaml`:

```yaml
version: 2
project_name: twentycrm

builds:
  - main: ./cmd/twentycrm
    binary: twentycrm
    env: [CGO_ENABLED=0]
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
    ignore:
      - goos: windows
        goarch: arm64
    ldflags:
      - -s -w -X github.com/wir-drei-digital/twenty-crm-cli/internal/cli.Version={{.Version}}

archives:
  - formats: [tar.gz]
    format_overrides:
      - goos: windows
        formats: [zip]

checksum:
  name_template: checksums.txt
```

- [ ] **Step 9: Final check**

Run: `go mod tidy && go vet ./... && go test ./...`
Expected: all tests PASS (the live test skips).

Run: `grep -rn "—" --include="*.go" --include="*.md" --include="*.yml" --include="*.yaml" . | grep -v docs/superpowers`
Expected: no output.

- [ ] **Step 10: Commit**

```bash
git add e2e README.md SECURITY.md docs/agents.md .github .goreleaser.yaml go.mod go.sum
git commit -m "Add end-to-end and live tests, documentation, CI and release configuration

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

## Around the tasks (controller, not a subagent task)

1. Before Task 1: the public repository `wir-drei-digital/twenty-crm-cli` exists on GitHub with `git@github.com:wir-drei-digital/twenty-crm-cli.git` as `origin`, and `main` (spec and plan) is pushed.
2. After each task's review passes: push `main`.
3. After Task 5: watch the CI run on Ubuntu, macOS and Windows until it is green, then run a final whole-branch review.
4. The live check needs an API key for `crm.wirdrei.digital` that a person creates. Until it has run, do not tag `v0.1.0` (spec: *CI and release*).
