package routes

import (
	"fmt"
	"strings"
	"testing"
)

const id = "3f2b8c1e-5d4a-4c1b-9e8f-1a2b3c4d5e6f"

func TestObjectVerbTable(t *testing.T) {
	want := []struct {
		name, method, path, class, soft string
		filter                          bool
		body                            BodyKind
	}{
		{"list", "GET", "rest/companies", ClassRead, "", false, BodyNone},
		{"get", "GET", "rest/companies/" + id, ClassRead, "", false, BodyNone},
		{"group-by", "GET", "rest/companies/groupBy", ClassRead, "", false, BodyNone},
		{"find-duplicates", "POST", "rest/companies/duplicates", ClassRead, "", false, BodyDuplicates},
		{"create", "POST", "rest/companies", ClassWrite, "", false, BodyObject},
		{"batch-create", "POST", "rest/batch/companies", ClassWrite, "", false, BodyArray},
		{"update", "PATCH", "rest/companies/" + id, ClassWrite, "", false, BodyObject},
		{"delete", "DELETE", "rest/companies/" + id, ClassWrite, "true", false, BodyNone},
		{"restore", "PATCH", "rest/restore/companies/" + id, ClassWrite, "", false, BodyNone},
		{"update-many", "PATCH", "rest/companies", ClassBulk, "", true, BodyObject},
		{"delete-many", "DELETE", "rest/companies", ClassBulk, "true", true, BodyNone},
		{"restore-many", "PATCH", "rest/restore/companies", ClassBulk, "", true, BodyNone},
		{"merge", "PATCH", "rest/companies/merge", ClassBulk, "", false, BodyMerge},
		{"destroy", "DELETE", "rest/companies/" + id, ClassDestroy, "false", false, BodyNone},
		{"destroy-many", "DELETE", "rest/companies", ClassDestroy, "false", true, BodyNone},
	}
	if len(ObjectVerbs) != len(want) {
		t.Fatalf("%d object verbs, want %d", len(ObjectVerbs), len(want))
	}
	for i, w := range want {
		v := ObjectVerbs[i]
		got := v.Path("companies", id)
		if v.Name != w.name || v.Method != w.method || got != w.path || v.Class != w.class ||
			v.SoftDelete != w.soft || v.FilterRequired != w.filter || v.Body != w.body {
			t.Errorf("verb %d = %+v (path %s), want %+v", i, v, got, w)
		}
		if v.TakesID != strings.Contains(v.PathTemplate, "{id}") {
			t.Errorf("%s: TakesID disagrees with the path template", v.Name)
		}
		if v.Summary == "" {
			t.Errorf("%s: no summary", v.Name)
		}
	}
	if v, _ := FindVerb(ObjectVerbs, "list"); v.MaxLimit != 200 {
		t.Errorf("list MaxLimit = %d, want 200", v.MaxLimit)
	}
	if v, _ := FindVerb(ObjectVerbs, "group-by"); v.RequiredFlag != "group-by" {
		t.Errorf("group-by must require --group-by")
	}
	if _, ok := FindVerb(ObjectVerbs, "nope"); ok {
		t.Error("FindVerb found a verb that does not exist")
	}
}

func TestMetadataTables(t *testing.T) {
	if len(MetadataKinds) != 13 {
		t.Fatalf("%d metadata kinds, want 13", len(MetadataKinds))
	}
	seg := map[string]string{}
	for _, k := range MetadataKinds {
		seg[k.Command] = k.Segment
	}
	if seg["view-filter-groups"] != "viewFilterGroups" || seg["api-keys"] != "apiKeys" || seg["objects"] != "objects" {
		t.Fatalf("segments = %v", seg)
	}
	list, _ := FindVerb(MetadataVerbs, "list")
	if list.Path("viewFields", "") != "rest/metadata/viewFields" || list.MaxLimit != 1000 || list.Class != ClassRead {
		t.Fatalf("metadata list = %+v", list)
	}
	del, _ := FindVerb(MetadataVerbs, "delete")
	if del.Path("fields", id) != "rest/metadata/fields/"+id || del.Class != ClassAdmin || del.Method != "DELETE" {
		t.Fatalf("metadata delete = %+v", del)
	}
	for _, verb := range []string{"create", "update", "delete"} {
		if MetadataBlocked("api-keys", verb) == "" {
			t.Errorf("api-keys %s must be blocked", verb)
		}
		if MetadataBlocked("webhooks", verb) != "" {
			t.Errorf("webhooks %s must not be blocked", verb)
		}
	}
	for _, verb := range []string{"list", "get"} {
		if MetadataBlocked("api-keys", verb) != "" {
			t.Errorf("api-keys %s must stay readable", verb)
		}
	}
}

func TestNeedsForce(t *testing.T) {
	for class, want := range map[string]bool{ClassRead: false, ClassWrite: false, ClassBulk: true, ClassDestroy: true, ClassAdmin: true} {
		if NeedsForce(class) != want {
			t.Errorf("NeedsForce(%s) = %v", class, !want)
		}
	}
}

func TestValidateID(t *testing.T) {
	for _, ok := range []string{id, strings.ToUpper(id)} {
		if err := ValidateID(ok); err != nil {
			t.Errorf("ValidateID(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"", "42", id + "/x", "../" + id, id[:35], " " + id, "3f2b8c1e5d4a4c1b9e8f1a2b3c4d5e6f"} {
		if err := ValidateID(bad); err == nil {
			t.Errorf("ValidateID(%q) accepted", bad)
		}
	}
}

func TestCheckBody(t *testing.T) {
	sixty, sixtyOne := batch(60), batch(61)
	cases := []struct {
		kind BodyKind
		body string // "<nil>" means --data was not given
		want string // "" means accepted
	}{
		{BodyNone, "<nil>", ""},
		{BodyNone, `{}`, "takes no --data"},
		{BodyObject, "<nil>", "--data is required"},
		{BodyObject, `{"name":"Acme"}`, ""},
		{BodyObject, `[]`, "JSON object"},
		{BodyObject, `"x"`, "JSON object"},
		{BodyArray, `[]`, "at least one record"},
		{BodyArray, `{"name":"A"}`, "JSON array"},
		{BodyArray, `[{"name":"A"},3]`, "record 2 is not a JSON object"},
		{BodyArray, sixty, ""},
		{BodyArray, sixtyOne, "at most 60"},
		{BodyMerge, `{"ids":["a","b"],"conflictPriorityIndex":0}`, ""},
		{BodyMerge, `{"ids":["a"]}`, "at least two"},
		{BodyMerge, `{"ids":["a",2]}`, "at least two"},
		{BodyMerge, `{"conflictPriorityIndex":0}`, "at least two"},
		{BodyDuplicates, `{"ids":["a"]}`, ""},
		{BodyDuplicates, `{"data":[{"name":"A"}]}`, ""},
		{BodyDuplicates, `{"ids":[]}`, `"ids" or "data"`},
		{BodyDuplicates, `{}`, `"ids" or "data"`},
	}
	for _, c := range cases {
		var body []byte
		if c.body != "<nil>" {
			body = []byte(c.body)
		}
		err := CheckBody(c.kind, body)
		if c.want == "" && err != nil || c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
			t.Errorf("CheckBody(%s, %.40s) = %v, want %q", c.kind, c.body, err, c.want)
		}
	}
}

func batch(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = fmt.Sprintf(`{"name":"C%d"}`, i)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func TestDecisionOrderAndMessages(t *testing.T) {
	cases := []struct {
		d    Decision
		want string
	}{
		{Decision{Command: "companies list", Class: ClassRead, ReadOnly: true}, ""},
		{Decision{Command: "companies create", Class: ClassWrite}, ""},
		{Decision{Command: "companies create", Class: ClassWrite, ReadOnly: true}, "read-only mode"},
		{Decision{Command: "companies update-many", Class: ClassBulk, FilterRequired: true}, "needs --filter"},
		{Decision{Command: "companies update-many", Class: ClassBulk, FilterRequired: true, Filter: "  "}, "needs --filter"},
		{Decision{Command: "companies update-many", Class: ClassBulk, FilterRequired: true, Filter: "a[eq]:1"}, "needs --force"},
		{Decision{Command: "companies update-many", Class: ClassBulk, FilterRequired: true, Filter: "a[eq]:1", Force: true}, ""},
		{Decision{Command: "companies destroy", Class: ClassDestroy}, "deletes permanently"},
		{Decision{Command: "metadata fields create", Class: ClassAdmin}, "needs --force"},
		{Decision{Command: "metadata api-keys delete", Class: ClassAdmin, Blocked: "reason", Force: true}, "is blocked: reason"},
		// Order: the filter comes before the block, the block before read-only, read-only before --force.
		{Decision{Command: "x", Class: ClassBulk, FilterRequired: true, Blocked: "b", ReadOnly: true}, "needs --filter"},
		{Decision{Command: "x", Class: ClassAdmin, Blocked: "b", ReadOnly: true}, "is blocked"},
		{Decision{Command: "x", Class: ClassDestroy, ReadOnly: true}, "read-only mode"},
	}
	for _, c := range cases {
		err := c.d.Check()
		if c.want == "" && err != nil || c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
			t.Errorf("%+v: Check = %v, want %q", c.d, err, c.want)
		}
	}
}
