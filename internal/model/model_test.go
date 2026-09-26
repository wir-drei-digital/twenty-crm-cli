package model

import (
	"compress/gzip"
	"io"
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

// TestExtractLiveRecording runs the extraction over a real Twenty 2.27
// document, recorded from a live workspace during the live check and
// sanitised: host, custom field names, their descriptions and select values
// are replaced by neutral ones. It pins the shapes the hand-built fixture
// only imitates.
func TestExtractLiveRecording(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "openapi-live-2.27.json.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	objs, err := Extract(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 28 {
		t.Fatalf("%d objects, want 28", len(objs))
	}
	m := &Model{Objects: objs}
	for _, name := range []string{"companies", "people", "notes", "note-targets", "tasks", "task-targets",
		"opportunities", "workspace-members", "workflows", "workflow-runs", "timeline-activities"} {
		if m.Find(name) == nil {
			t.Errorf("no object %s", name)
		}
	}
	if f := field(t, m.Find("companies"), "stage"); len(f.Enum) != 7 || f.Enum[1] != "LEAD" {
		t.Errorf("companies.stage = %+v", f)
	}
	nt := m.Find("note-targets")
	if f := field(t, nt, "targetCompany"); f.Relation == nil || *f.Relation != (Relation{Target: "companies", Kind: "many_to_one"}) {
		t.Errorf("note-targets.targetCompany = %+v", f)
	}
	if f := field(t, nt, "targetCompanyId"); f.Format != "uuid" || f.ReadOnly {
		t.Errorf("note-targets.targetCompanyId = %+v", f)
	}
	if f := field(t, m.Find("notes"), "bodyV2"); strings.Join(f.Subfields, ",") != "blocknote,markdown" {
		t.Errorf("notes.bodyV2 = %+v", f)
	}
	people := m.Find("people")
	if f := field(t, people, "emails"); !contains(f.Subfields, "primaryEmail") {
		t.Errorf("people.emails = %+v", f)
	}
	if f := field(t, people, "company"); f.Relation == nil || *f.Relation != (Relation{Target: "companies", Kind: "many_to_one"}) {
		t.Errorf("people.company = %+v", f)
	}
	if f := field(t, m.Find("companies"), "id"); !f.ReadOnly {
		t.Errorf("companies.id = %+v", f)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
