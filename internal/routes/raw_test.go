package routes

import (
	"net/url"
	"strings"
	"testing"
)

func TestClassifyRaw(t *testing.T) {
	const soft, filter, both = "soft_delete=true", "filter=a[eq]:1", "soft_delete=true&filter=a[eq]:1"
	cases := []struct {
		method, path, query string
		class               string
		blocked, filter     bool
	}{
		// Fixed prefixes: API keys, metadata, webhooks, the OpenAPI documents.
		{"GET", "rest/apiKeys", "", ClassRead, false, false},
		{"POST", "rest/apiKeys", "", ClassAdmin, true, false},
		{"DELETE", "rest/metadata/apiKeys/" + id, "", ClassAdmin, true, false},
		{"GET", "rest/metadata/objects", "", ClassRead, false, false},
		{"POST", "rest/metadata/fields", "", ClassAdmin, false, false},
		{"PATCH", "rest/webhooks/" + id, "", ClassAdmin, false, false},
		{"GET", "rest/open-api/core", "", ClassRead, false, false},
		{"POST", "rest", "", ClassAdmin, false, false},

		// GET is always a read: find one, find many, group by.
		{"GET", "rest/companies", "limit=5", ClassRead, false, false},
		{"GET", "rest/companies/" + id, "", ClassRead, false, false},
		{"GET", "rest/companies/groupBy", "group_by=x", ClassRead, false, false},
		{"GET", "rest/batch/companies", "", ClassRead, false, false},
		{"GET", "rest/companies/not-a-uuid", "", ClassRead, false, false},
		{"GET", "rest/dashboards/" + id + "/duplicate", "", ClassRead, false, false},

		// POST: batch, then duplicates, then create one (which parses the path).
		{"POST", "rest/companies", "", ClassWrite, false, false},
		{"POST", "rest/batch/companies", "", ClassWrite, false, false},
		{"POST", "rest/batch", "", ClassWrite, false, false},
		{"POST", "rest/companies/duplicates", "", ClassRead, false, false},
		{"POST", "rest/a/b/duplicates", "", ClassRead, false, false},
		{"POST", "rest/duplicates", "", ClassWrite, false, false},
		{"POST", "rest/companies/Duplicates", "", ClassAdmin, false, false},
		{"POST", "rest/companies/" + id, "", ClassWrite, false, false},
		{"POST", "rest/companies/not-a-uuid", "", ClassAdmin, false, false},
		{"POST", "rest/restore/companies", "", ClassWrite, false, false},
		{"POST", "rest/restore/companies/" + id, "", ClassAdmin, false, false},
		{"POST", "rest/dashboards/" + id + "/duplicate", "", ClassAdmin, false, false},

		// DELETE: one record by ID, or every record the filter matches.
		{"DELETE", "rest/companies/" + id, soft, ClassWrite, false, false},
		{"DELETE", "rest/companies/" + id, "", ClassDestroy, false, false},
		{"DELETE", "rest/companies/" + id, "soft_delete=TRUE", ClassDestroy, false, false},
		{"DELETE", "rest/companies/" + id, "soft_delete=false", ClassDestroy, false, false},
		{"DELETE", "rest/companies", both, ClassBulk, false, true},
		{"DELETE", "rest/companies", filter, ClassDestroy, false, true},
		{"DELETE", "rest/batch/companies", both, ClassBulk, false, true},
		{"DELETE", "rest/batch/companies", "", ClassDestroy, false, true},
		{"DELETE", "rest/companies/duplicates", soft, ClassBulk, false, true},
		{"DELETE", "rest/companies/groupBy", "", ClassDestroy, false, true},
		{"DELETE", "rest/companies/merge", soft, ClassBulk, false, true},
		{"DELETE", "rest/restore/companies", "", ClassDestroy, false, true},
		{"DELETE", "rest/restore/companies", soft, ClassBulk, false, true},
		{"DELETE", "rest/companies/not-a-uuid", soft, ClassAdmin, false, false},
		{"DELETE", "rest/companies/" + id + "/x", soft, ClassAdmin, false, false},

		// PATCH: restore, then merge, then update.
		{"PATCH", "rest/restore/companies", filter, ClassBulk, false, true},
		{"PATCH", "rest/restore/companies/" + id, "", ClassAdmin, false, false},
		{"PATCH", "rest/restore/" + id, "", ClassBulk, false, true},
		{"PATCH", "rest/restore", "", ClassBulk, false, true},
		{"PATCH", "rest/restore/companies/merge", "", ClassAdmin, false, false},
		{"PATCH", "rest/companies/merge", "", ClassBulk, false, false},
		{"PATCH", "rest/a/b/merge", "", ClassBulk, false, false},
		{"PATCH", "rest/merge", "", ClassBulk, false, true},
		{"PATCH", "rest/companies/Merge", "", ClassAdmin, false, false},
		{"PATCH", "rest/companies/" + id, "", ClassWrite, false, false},
		{"PATCH", "rest/companies", filter, ClassBulk, false, true},
		{"PATCH", "rest/batch/companies", "", ClassBulk, false, true},
		{"PATCH", "rest/companies/duplicates", "", ClassBulk, false, true},
		{"PATCH", "rest/companies/groupBy", "", ClassBulk, false, true},
		{"PATCH", "rest/companies/not-a-uuid", "", ClassAdmin, false, false},

		// PUT never reaches restore or merge: it always updates.
		{"PUT", "rest/companies/" + id, "", ClassWrite, false, false},
		{"PUT", "rest/companies", "", ClassBulk, false, true},
		{"PUT", "rest/batch/companies", "", ClassBulk, false, true},
		{"PUT", "rest/companies/duplicates", "", ClassBulk, false, true},
		{"PUT", "rest/companies/groupBy", "", ClassBulk, false, true},
		{"PUT", "rest/companies/merge", "", ClassBulk, false, true},
		{"PUT", "rest/restore/companies", "", ClassBulk, false, true},
		{"PUT", "rest/restore/companies/" + id, "", ClassAdmin, false, false},
		{"PUT", "rest/companies/not-a-uuid", "", ClassAdmin, false, false},
	}
	for _, c := range cases {
		q, _ := url.ParseQuery(c.query)
		got, err := ClassifyRaw(c.method, c.path, q)
		if err != nil {
			t.Errorf("%s %s?%s: %v", c.method, c.path, c.query, err)
			continue
		}
		if got.Class != c.class || (got.Blocked != "") != c.blocked || got.FilterRequired != c.filter {
			t.Errorf("%s %s?%s = %+v, want class %s blocked %v filter %v", c.method, c.path, c.query, got, c.class, c.blocked, c.filter)
		}
	}
}

func TestClassifyRawAcceptsLeadingSlashAndLowercaseMethod(t *testing.T) {
	got, err := ClassifyRaw("delete", "/rest/companies/"+id, url.Values{"soft_delete": {"true"}})
	if err != nil || got.Class != ClassWrite {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestClassifyRawRefusals(t *testing.T) {
	cases := []struct {
		method, path string
		q            url.Values
		want         string
	}{
		{"GET", "graphql", nil, "GraphQL is not supported"},
		{"POST", "metadata", nil, "GraphQL is not supported"},
		{"GET", "healthz", nil, "only paths under rest/"},
		{"GET", "rest/companies?limit=1", nil, "--query"},
		{"GET", "rest/companies#x", nil, "--query"},
		{"DELETE", "rest/companies/%2e%2e", nil, "not allowed"},
		{"DELETE", "rest/./companies", nil, "not allowed"},
		{"DELETE", "rest/../rest/companies", nil, "not allowed"},
		{"DELETE", "rest//companies", nil, "not allowed"},
		{"DELETE", "rest/companies/", nil, "not allowed"},
		{"GET", `rest\companies`, nil, "not allowed"},
		{"GET", "rest/com panies", nil, "not allowed"},
		{"TRACE", "rest/companies", nil, "method"},
		{"DELETE", "rest/companies/" + id, url.Values{"soft_delete": {"true", "true"}}, "soft_delete may appear only once"},
		{"PATCH", "rest/companies", url.Values{"filter": {"a[eq]:1", "b[eq]:2"}}, "filter may appear only once"},
		// Express would parse these keys into objects or leave them unread,
		// so Twenty would see no filter and act on every record.
		{"PATCH", "rest/companies", url.Values{"filter[id]": {"a"}}, `key "filter[id]"`},
		{"GET", "rest/companies", url.Values{"order_by]": {"name"}}, `key "order_by]"`},
		{"DELETE", "rest/companies/" + id, url.Values{"soft_delete[]": {"true"}}, `key "soft_delete[]"`},
		{"PATCH", "rest/companies", url.Values{"Filter": {"a[eq]:1"}}, `key "Filter"`},
		{"DELETE", "rest/companies/" + id, url.Values{"SOFT_DELETE": {"true"}}, `key "SOFT_DELETE"`},
	}
	for _, c := range cases {
		if _, err := ClassifyRaw(c.method, c.path, c.q); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s %s %v: err = %v, want %q", c.method, c.path, c.q, err, c.want)
		}
	}
}
