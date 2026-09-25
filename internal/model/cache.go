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
