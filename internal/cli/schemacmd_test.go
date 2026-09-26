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
