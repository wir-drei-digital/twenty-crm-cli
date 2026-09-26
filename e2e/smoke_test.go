package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakeTwenty is an in-memory Twenty with one object, companies: enough of
// the record routes to drive the binary through create, read, update, trash,
// restore and destroy. It serves the fixture OpenAPI document for the model.
func fakeTwenty(t *testing.T) *httptest.Server {
	t.Helper()
	doc, err := os.ReadFile(filepath.Join("..", "internal", "model", "testdata", "openapi-core.json"))
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	records := map[string]map[string]any{}
	trashed := map[string]bool{}
	var order []string
	seq := 0
	write := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(v)
	}
	notFound := func(w http.ResponseWriter) {
		write(w, 404, map[string]any{"statusCode": 404, "messages": []string{"Record not found"}, "error": "NOT_FOUND"})
	}
	key := apiKey()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/rest/open-api/core" {
			w.Write(doc) // public endpoint: answers everyone
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+key {
			write(w, 401, map[string]any{"statusCode": 401, "messages": []string{"Token invalid"}, "error": "UNAUTHENTICATED"})
			return
		}
		p := r.URL.Path
		switch {
		case p == "/rest/companies" && r.Method == http.MethodGet:
			rows := []map[string]any{}
			for _, id := range order {
				if !trashed[id] {
					rows = append(rows, records[id])
				}
			}
			write(w, 200, map[string]any{"data": map[string]any{"companies": rows}, "totalCount": len(rows),
				"pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""}})
		case p == "/rest/companies" && r.Method == http.MethodPost:
			var rec map[string]any
			json.NewDecoder(r.Body).Decode(&rec)
			seq++
			id := fmt.Sprintf("00000000-0000-4000-8000-%012d", seq)
			rec["id"] = id
			records[id] = rec
			order = append(order, id)
			write(w, 201, map[string]any{"data": map[string]any{"createCompany": rec}})
		case strings.HasPrefix(p, "/rest/restore/") && r.Method == http.MethodPatch:
			// Twenty 2.27 answers 400 for rest/restore/<o>/<id> (parseCorePath
			// takes at most two segments) and restores what the filter
			// matches; this fake knows only the one filter the CLI sends.
			id, ok := strings.CutPrefix(r.URL.Query().Get("filter"), "id[eq]:")
			if p != "/rest/restore/companies" || !ok {
				write(w, 400, map[string]any{"statusCode": 400, "messages": []string{"Query path invalid"}, "error": "BAD_REQUEST"})
				return
			}
			rows := []map[string]any{}
			if records[id] != nil {
				delete(trashed, id)
				rows = append(rows, records[id])
			}
			write(w, 200, map[string]any{"data": map[string]any{"restoreCompanies": rows}})
		case strings.HasPrefix(p, "/rest/companies/"):
			id := path.Base(p)
			rec := records[id]
			if rec == nil || (trashed[id] && r.Method != http.MethodDelete) {
				notFound(w)
				return
			}
			switch r.Method {
			case http.MethodGet:
				write(w, 200, map[string]any{"data": map[string]any{"company": rec}})
			case http.MethodPatch:
				var patch map[string]any
				json.NewDecoder(r.Body).Decode(&patch)
				for k, v := range patch {
					rec[k] = v
				}
				write(w, 200, map[string]any{"data": map[string]any{"updateCompany": rec}})
			case http.MethodDelete:
				if r.URL.Query().Get("soft_delete") == "true" {
					trashed[id] = true
				} else {
					delete(records, id)
					delete(trashed, id)
					for i, o := range order {
						if o == id {
							order = append(order[:i], order[i+1:]...)
							break
						}
					}
				}
				write(w, 200, map[string]any{"data": map[string]any{"deleteCompany": map[string]any{"id": id}}})
			}
		default:
			notFound(w)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSmokeRecordLifecycle(t *testing.T) {
	bin := buildBinary(t)
	srv := fakeTwenty(t)
	env := scrubbedEnv(t, "TWENTY_BASE_URL="+srv.URL, "TWENTY_API_KEY="+apiKey())

	if out, _, code := run(t, bin, env, "", "version"); code != 0 || !strings.HasPrefix(out, "twentycrm ") {
		t.Fatalf("version: %d %q", code, out)
	}
	out, _, code := run(t, bin, env, "", "commands", "--json")
	if code != 0 || !strings.Contains(out, `"model_missing":true`) {
		t.Fatalf("catalog before any model: %d %s", code, out)
	}
	out, errOut, code := run(t, bin, env, "", "schema")
	var objs []map[string]any
	if code != 0 || json.Unmarshal([]byte(out), &objs) != nil || len(objs) != 4 {
		t.Fatalf("schema: %d %s %s", code, out, errOut)
	}

	out, errOut, code = run(t, bin, env, `{"name":"Acme"}`, "companies", "create", "--data", "-")
	var created struct {
		Data struct {
			CreateCompany struct {
				ID string `json:"id"`
			} `json:"createCompany"`
		} `json:"data"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &created) != nil || created.Data.CreateCompany.ID == "" {
		t.Fatalf("create: %d %s %s", code, out, errOut)
	}
	id := created.Data.CreateCompany.ID

	steps := []struct {
		args []string
		code int
		kind string
	}{
		{[]string{"companies", "get", id}, 0, ""},
		{[]string{"companies", "update", id, "--data", `{"employees":5}`}, 0, ""},
		{[]string{"companies", "delete", id}, 0, ""},
		{[]string{"companies", "get", id}, 1, "not_found"},
		{[]string{"companies", "restore", id}, 0, ""},
		{[]string{"companies", "get", id}, 0, ""},
		{[]string{"companies", "destroy", id}, 2, "usage"},
		{[]string{"companies", "destroy", id, "--force"}, 0, ""},
		{[]string{"companies", "get", id}, 1, "not_found"},
		{[]string{"companies", "update-many", "--data", `{}`, "--force"}, 2, "usage"},
		{[]string{"companies", "get", "not-a-uuid"}, 2, "usage"},
		{[]string{"widgets", "list"}, 2, "usage"},
	}
	for _, s := range steps {
		out, errOut, code := run(t, bin, env, "", s.args...)
		if code != s.code {
			t.Fatalf("%v: exit %d, want %d\n%s\n%s", s.args, code, s.code, out, errOut)
		}
		if s.kind != "" {
			if e := errorLine(t, errOut); e["kind"] != s.kind || out != "" {
				t.Fatalf("%v: error %v, stdout %q", s.args, e, out)
			}
		}
		if s.args[1] == "restore" {
			// restore answers like restore-many: an array of the restored records.
			var restored struct {
				Data struct {
					RestoreCompanies []map[string]any `json:"restoreCompanies"`
				} `json:"data"`
			}
			if json.Unmarshal([]byte(out), &restored) != nil || len(restored.Data.RestoreCompanies) != 1 ||
				restored.Data.RestoreCompanies[0]["id"] != id {
				t.Fatalf("restore: %s", out)
			}
		}
	}

	out, _, code = run(t, bin, env, `{"name":"Beta"}`, "companies", "create", "--data", "-")
	if code != 0 {
		t.Fatal("second create failed")
	}
	out, _, code = run(t, bin, env, "", "companies", "list", "--all")
	var rows []map[string]any
	if code != 0 || json.Unmarshal([]byte(out), &rows) != nil || len(rows) != 1 || rows[0]["name"] != "Beta" {
		t.Fatalf("list --all: %d %s", code, out)
	}

	ro := append(append([]string{}, env...), "TWENTY_READ_ONLY=1")
	if _, errOut, code := run(t, bin, ro, `{"name":"X"}`, "companies", "create", "--data", "-"); code != 2 || errorLine(t, errOut)["kind"] != "usage" {
		t.Fatalf("read-only create: %d %s", code, errOut)
	}
}

func TestSmokeConfigFileAndBinding(t *testing.T) {
	bin := buildBinary(t)
	srv := fakeTwenty(t)
	env := scrubbedEnv(t)
	if _, errOut, code := run(t, bin, env, "", "config", "set", "base-url", srv.URL); code != 0 {
		t.Fatalf("set base-url: %d %s", code, errOut)
	}
	if _, errOut, code := run(t, bin, env, "Bearer "+apiKey()+"\n", "config", "set", "api-key"); code != 0 {
		t.Fatalf("set api-key: %d %s", code, errOut)
	}
	out, _, code := run(t, bin, env, "", "auth", "status")
	var st map[string]any
	if code != 0 || json.Unmarshal([]byte(out), &st) != nil || st["source"] != "config" || st["workspace_id"] != "ws-e2e" || strings.Contains(out, apiKey()) {
		t.Fatalf("auth status: %d %s", code, out)
	}
	if _, errOut, code := run(t, bin, env, "", "companies", "list"); code != 0 {
		t.Fatalf("list with the stored key: %d %s", code, errOut)
	}
	elsewhere := append(append([]string{}, env...), "TWENTY_BASE_URL=https://elsewhere.example.com")
	if _, errOut, code := run(t, bin, elsewhere, "", "companies", "list"); code != 2 || !strings.Contains(errorLine(t, errOut)["error"].(string), "belongs to") {
		t.Fatalf("binding: %d %s", code, errOut)
	}
}
