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
		Filter         string   `json:"filter"`
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
	if v := d.ObjectVerbs[verbs["restore"]]; v.Path != "rest/restore/{plural}" || v.Filter != "id[eq]:{id}" || !v.TakesID || v.FilterRequired {
		t.Errorf("restore = %+v", v)
	}
	if v := d.ObjectVerbs[verbs["restore-many"]]; v.Filter != "" || !v.FilterRequired {
		t.Errorf("restore-many = %+v", v)
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
