package cli

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestObjectHelp(t *testing.T) {
	srv := newFakeTwenty(t)
	a, stdout, stderr := newTestApp(t, srv, nil)
	seedCache(t, a, time.Hour)
	if code := a.run([]string{"companies", "create", "--help"}); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	out := stdout.String()
	for _, want := range []string{"Request: POST /rest/companies", "Class: write", "stage", "one of: LEAD, CUSTOMER, CHURNED",
		"subfields: primaryLinkLabel, primaryLinkUrl, secondaryLinks", "one_to_many -> people", "twentycrm schema companies"} {
		if !strings.Contains(out, want) {
			t.Errorf("help lacks %q:\n%s", want, out)
		}
	}
	if !regexp.MustCompile(`(?m)^\s*name\s+string\s+required`).MatchString(out) {
		t.Errorf("name is not marked required:\n%s", out)
	}
	stdout.Reset()
	a.run([]string{"companies", "destroy", "--help"})
	if out := stdout.String(); !strings.Contains(out, "soft_delete=false") || !strings.Contains(out, "needs --force") {
		t.Errorf("destroy help:\n%s", out)
	}
	stdout.Reset()
	a.run([]string{"companies", "list", "--help"})
	if out := stdout.String(); !strings.Contains(out, "Filter syntax") || !strings.Contains(out, "ilike") || !strings.Contains(out, "DescNullsLast") {
		t.Errorf("list help:\n%s", out)
	}
}

func TestVersion(t *testing.T) {
	srv := newFakeTwenty(t)
	a, stdout, _ := newTestApp(t, srv, nil)
	if code := a.run([]string{"version"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if out := stdout.String(); !strings.HasPrefix(out, "twentycrm ") || !strings.Contains(out, "tested against Twenty 2.27") {
		t.Fatalf("version = %q", out)
	}
}
