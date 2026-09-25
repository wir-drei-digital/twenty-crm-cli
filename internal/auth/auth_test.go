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
		"not a jwt":    {"abc", "three base64url parts"},
		"empty part":   {"a..c", "three base64url parts"},
		"bad base64":   {"a.!!!.c", "not base64url"},
		"not json":     {"a." + base64.RawURLEncoding.EncodeToString([]byte("[1]")) + ".c", "not a JSON object"},
		"user token":   {jwt(t, map[string]any{"type": "ACCESS", "workspaceId": "ws-1"}), "ACCESS token, not an API key"},
		"untyped":      {jwt(t, map[string]any{"workspaceId": "ws-1"}), "untyped token"},
		"no workspace": {jwt(t, map[string]any{"type": "API_KEY"}), "names no workspace"},
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
