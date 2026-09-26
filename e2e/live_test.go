package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLive runs against a real Twenty with the operator's own configuration
// (config file or TWENTY_* variables). It never runs in CI.
//
//	TWENTY_LIVE=1                     read-only checks
//	TWENTY_LIVE_SELECT_FIELD=<field>  a select field of companies whose values must be listed
//	TWENTY_LIVE_RESTRICTED_KEY=<key>  a key whose role lacks the Data Model permission
//	TWENTY_LIVE_WRITE=1               also create, change and destroy one marked test company
func TestLive(t *testing.T) {
	if os.Getenv("TWENTY_LIVE") != "1" {
		t.Skip("set TWENTY_LIVE=1 with a configured key to run against a real Twenty")
	}
	bin := buildBinary(t)
	env := os.Environ()
	jsonOf := func(args ...string) map[string]any {
		t.Helper()
		out, errOut, code := run(t, bin, env, "", args...)
		var m map[string]any
		if code != 0 || json.Unmarshal([]byte(out), &m) != nil {
			t.Fatalf("%v: exit %d\n%s\n%s", args, code, out, errOut)
		}
		return m
	}

	st := jsonOf("auth", "status")
	if st["mode"] != "api_key" || st["workspace_id"] == nil {
		t.Fatalf("auth status: %v", st)
	}
	t.Logf("workspace %v, key expires %v", st["workspace_id"], st["expires_at"])

	schema := jsonOf("schema", "companies")
	fields, _ := schema["fields"].([]any)
	if len(fields) == 0 {
		t.Fatalf("schema companies: %v", schema)
	}
	if name := os.Getenv("TWENTY_LIVE_SELECT_FIELD"); name != "" {
		found := false
		for _, f := range fields {
			fm := f.(map[string]any)
			if fm["name"] == name {
				enum, _ := fm["enum"].([]any)
				found = len(enum) > 0
				t.Logf("%s values: %v", name, enum)
			}
		}
		if !found {
			t.Fatalf("companies.%s lists no select values", name)
		}
	}

	total := int(jsonOf("companies", "list", "--limit", "1", "--depth", "0")["totalCount"].(float64))
	if total <= 20000 {
		out, errOut, code := run(t, bin, env, "", "companies", "list", "--all", "--depth", "0")
		var rows []any
		if code != 0 || json.Unmarshal([]byte(out), &rows) != nil || len(rows) != total {
			t.Fatalf("list --all: exit %d, %d rows, totalCount %d\n%s", code, len(rows), total, errOut)
		}
	}

	if key := os.Getenv("TWENTY_LIVE_RESTRICTED_KEY"); key != "" {
		restricted := append(append([]string{}, env...), "TWENTY_API_KEY="+key, fmt.Sprintf("TWENTY_BASE_URL=%v", st["base_url"]))
		if _, errOut, code := run(t, bin, restricted, "", "schema"); code != 0 {
			t.Fatalf("schema with a restricted key: %d %s", code, errOut)
		}
		_, errOut, code := run(t, bin, restricted, "", "metadata", "objects", "list")
		if code != 1 || errorLine(t, errOut)["kind"] != "forbidden" || !strings.Contains(errOut, "Data Model") {
			t.Fatalf("metadata objects with a restricted key: %d %s", code, errOut)
		}
	}

	if os.Getenv("TWENTY_LIVE_WRITE") != "1" {
		return
	}
	name := fmt.Sprintf("twentycrm-live-%d", time.Now().Unix())
	out, errOut, code := run(t, bin, env, fmt.Sprintf(`{"name":%q}`, name), "companies", "create", "--data", "-")
	var created map[string]map[string]map[string]any
	if code != 0 || json.Unmarshal([]byte(out), &created) != nil {
		t.Fatalf("create: %d %s %s", code, out, errOut)
	}
	id, _ := created["data"]["createCompany"]["id"].(string)
	if id == "" {
		t.Fatalf("create returned no id: %s", out)
	}
	t.Cleanup(func() { run(t, bin, env, "", "companies", "destroy", id, "--force") })
	jsonOf("companies", "get", id)
	// The update renames the company: name is the one field every workspace
	// has, and the listings below find the record by its new name only.
	name += "-updated"
	jsonOf("companies", "update", id, "--data", fmt.Sprintf(`{"name":%q}`, name))
	filter := fmt.Sprintf("name[eq]:%q", name)
	count := func() int {
		m := jsonOf("companies", "list", "--filter", filter, "--depth", "0")
		return int(m["totalCount"].(float64))
	}
	jsonOf("companies", "delete", id)
	if n := count(); n != 0 {
		t.Fatalf("a trashed company is still listed (%d)", n)
	}
	// Twenty 2.27 cannot restore by path, so restore is restore-many limited
	// to the one ID and answers with an array.
	data, _ := jsonOf("companies", "restore", id)["data"].(map[string]any)
	restored, _ := data["restoreCompanies"].([]any)
	if len(restored) != 1 {
		t.Fatalf("restore did not return exactly one company: %v", data)
	}
	if rec, _ := restored[0].(map[string]any); rec["id"] != id {
		t.Fatalf("restore returned another company: %v", rec)
	}
	if n := count(); n != 1 {
		t.Fatalf("the restored company is not listed (%d)", n)
	}
	jsonOf("companies", "destroy", id, "--force")
	if _, errOut, code := run(t, bin, env, "", "companies", "get", id); code != 1 || errorLine(t, errOut)["kind"] != "not_found" {
		t.Fatalf("get after destroy: %d %s", code, errOut)
	}
}
