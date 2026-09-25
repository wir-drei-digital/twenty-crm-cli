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
