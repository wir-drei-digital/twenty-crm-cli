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
	"sync"
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
// signatures, and the fake Twenty compares the string. It is built once: the
// expiry comes from the clock, so two keys built in different seconds would
// differ, and the fake Twenty would refuse the second.
var apiKey = sync.OnceValue(func() string {
	enc := func(v any) string {
		raw, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	return enc(map[string]string{"alg": "HS256", "typ": "JWT"}) + "." +
		enc(map[string]any{"sub": "ws-e2e", "type": "API_KEY", "workspaceId": "ws-e2e", "jti": "key-e2e",
			"exp": time.Now().Add(60 * 24 * time.Hour).Unix()}) + ".c2ln"
})

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
