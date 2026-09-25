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
