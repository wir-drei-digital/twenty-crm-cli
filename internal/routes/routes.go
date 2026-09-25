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
