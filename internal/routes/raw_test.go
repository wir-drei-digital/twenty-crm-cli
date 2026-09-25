package routes

import (
	"net/url"
	"strings"
	"testing"
)

func TestClassifyRaw(t *testing.T) {
	cases := []struct {
		method, path, query string
		class               string
		blocked, filter     bool
	}{
		{"GET", "rest/apiKeys", "", ClassRead, false, false},
		{"POST", "rest/apiKeys", "", ClassAdmin, true, false},
		{"DELETE", "rest/metadata/apiKeys/" + id, "", ClassAdmin, true, false},
		{"GET", "rest/metadata/objects", "", ClassRead, false, false},
		{"POST", "rest/metadata/fields", "", ClassAdmin, false, false},
		{"PATCH", "rest/webhooks/" + id, "", ClassAdmin, false, false},
		{"GET", "rest/open-api/core", "", ClassRead, false, false},
		{"POST", "rest/batch/companies", "", ClassWrite, false, false},
		{"PATCH", "rest/restore/companies/" + id, "", ClassWrite, false, false},
		{"PATCH", "rest/restore/companies", "filter=a[eq]:1", ClassBulk, false, true},
		{"POST", "rest/companies/duplicates", "", ClassRead, false, false},
		{"PATCH", "rest/companies/merge", "", ClassBulk, false, false},
		{"GET", "rest/companies/groupBy", "group_by=x", ClassRead, false, false},
		{"GET", "rest/companies/" + id, "", ClassRead, false, false},
		{"PATCH", "rest/companies/" + id, "", ClassWrite, false, false},
		{"PUT", "rest/companies/" + id, "", ClassWrite, false, false},
		{"DELETE", "rest/companies/" + id, "soft_delete=true", ClassWrite, false, false},
		{"DELETE", "rest/companies/" + id, "", ClassDestroy, false, false},
		{"DELETE", "rest/companies/" + id, "soft_delete=TRUE", ClassDestroy, false, false},
		{"DELETE", "rest/companies/" + id, "soft_delete=false", ClassDestroy, false, false},
		{"GET", "rest/companies", "limit=5", ClassRead, false, false},
		{"POST", "rest/companies", "", ClassWrite, false, false},
		{"PATCH", "rest/companies", "filter=a[eq]:1", ClassBulk, false, true},
		{"PUT", "rest/companies", "", ClassBulk, false, true},
		{"DELETE", "rest/companies", "soft_delete=true&filter=a[eq]:1", ClassBulk, false, true},
		{"DELETE", "rest/companies", "filter=a[eq]:1", ClassDestroy, false, true},
		{"POST", "rest/companies/" + id, "", ClassAdmin, false, false},
		{"POST", "rest/dashboards/" + id + "/duplicate", "", ClassAdmin, false, false},
		{"GET", "rest/dashboards/" + id + "/duplicate", "", ClassRead, false, false},
		{"POST", "rest", "", ClassAdmin, false, false},
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
	}
	for _, c := range cases {
		if _, err := ClassifyRaw(c.method, c.path, c.q); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s %s %v: err = %v, want %q", c.method, c.path, c.q, err, c.want)
		}
	}
}
