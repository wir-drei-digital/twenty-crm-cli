package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// endless is a reader that never ends and counts what was read from it.
type endless struct{ n int }

func (e *endless) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = ' '
	}
	e.n += len(p)
	return len(p), nil
}

func (e *endless) Close() error { return nil }

func TestDataFileOverTheLimit(t *testing.T) {
	p := filepath.Join(t.TempDir(), "big.json")
	if err := os.WriteFile(p, []byte("{}"+strings.Repeat(" ", maxJSONBody-1)), 0o600); err != nil {
		t.Fatal(err)
	}
	srv := newFakeTwenty(t)
	a, _, stderr := newTestApp(t, srv, nil)
	code := a.run([]string{"api", "POST", "rest/companies", "--data", "@" + p})
	if code != 2 || !strings.Contains(errLine(t, stderr.String()).Message, "exceeds the 20 MB limit") || len(srv.calls()) != 0 {
		t.Fatalf("exit %d %s, %d calls", code, stderr, len(srv.calls()))
	}
}

func TestDataFileReadingStopsAtTheLimit(t *testing.T) {
	// Stands in for --data @/dev/zero; the path does not exist, so a
	// reader that bypasses openDataFile fails with "no such file" instead.
	p := filepath.Join(t.TempDir(), "endless.json")
	src := &endless{}
	old := openDataFile
	openDataFile = func(name string) (io.ReadCloser, error) {
		if name != p {
			t.Errorf("opened %q, want %q", name, p)
		}
		return src, nil
	}
	t.Cleanup(func() { openDataFile = old })
	srv := newFakeTwenty(t)
	a, _, stderr := newTestApp(t, srv, nil)
	code := a.run([]string{"api", "POST", "rest/companies", "--data", "@" + p})
	if code != 2 || !strings.Contains(errLine(t, stderr.String()).Message, "exceeds the 20 MB limit") {
		t.Fatalf("exit %d %s", code, stderr)
	}
	if src.n > maxJSONBody+1 {
		t.Fatalf("read %d bytes, want at most %d", src.n, maxJSONBody+1)
	}
}
