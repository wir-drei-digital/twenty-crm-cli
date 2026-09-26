package cli

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/config"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/model"
)

type fakePrompter struct {
	lines, secrets []string
	confirms       []bool
}

func (f *fakePrompter) Line(string) (string, error) {
	if len(f.lines) == 0 {
		return "", io.EOF
	}
	s := f.lines[0]
	f.lines = f.lines[1:]
	return s, nil
}

func (f *fakePrompter) Secret(string) (string, error) {
	if len(f.secrets) == 0 {
		return "", io.EOF
	}
	s := f.secrets[0]
	f.secrets = f.secrets[1:]
	return s, nil
}

func (f *fakePrompter) Confirm(_ string, def bool) (bool, error) {
	if len(f.confirms) == 0 {
		return def, nil
	}
	b := f.confirms[0]
	f.confirms = f.confirms[1:]
	return b, nil
}

// initApp builds an app over the isolated home with a terminal and scripted
// answers; companies answers GET /rest/companies with the given status.
func initApp(t *testing.T, companies int, p *fakePrompter) (*app, *fakeTwenty, *strings.Builder, *strings.Builder) {
	t.Helper()
	srv := newFakeTwenty(t)
	srv.handle = func(w http.ResponseWriter, r *http.Request, _ []byte) {
		w.WriteHeader(companies)
		if companies == 200 {
			io.WriteString(w, `{"data":{"companies":[]},"totalCount":7,"pageInfo":{"hasNextPage":false}}`)
			return
		}
		io.WriteString(w, `{"statusCode":403,"messages":["Forbidden"],"error":"FORBIDDEN"}`)
	}
	var out, errb strings.Builder
	a := &app{stdout: &out, stderr: &errb, stdin: strings.NewReader("")}
	a.configure(func(string) string { return "" }, func(time.Duration) {})
	a.isTerminal = func() bool { return true }
	a.prompt = p
	return a, srv, &out, &errb
}

func TestInitRefusesWithoutTerminal(t *testing.T) {
	isolate(t)
	a, _, _, errb := initApp(t, 200, &fakePrompter{})
	a.isTerminal = func() bool { return false }
	if code := a.run([]string{"init"}); code != 2 || !strings.Contains(errb.String(), "terminal") {
		t.Fatalf("exit %d %s", code, errb)
	}
}

func TestInitHappyPath(t *testing.T) {
	isolate(t)
	key := testKey(t, "ws-test", time.Now().Add(90*24*time.Hour))
	p := &fakePrompter{secrets: []string{key}, confirms: []bool{true}}
	a, srv, out, errb := initApp(t, 200, p)
	p.lines = []string{srv.URL}
	if code := a.run([]string{"init"}); code != 0 {
		t.Fatalf("exit %d: %s\n%s", code, errb, out)
	}
	for _, want := range []string{"ws-test", "4 objects", "7 companies"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	c := mustLoad(t)
	if c.BaseURL != srv.URL || c.APIKey != key || !c.ReadOnly {
		t.Fatalf("config = %+v", c)
	}
	root, _ := os.UserCacheDir()
	if model.LoadCache(root, srv.URL, "ws-test") == nil {
		t.Fatal("init must write the model cache")
	}
}

func TestInitSkipsTheCountWhenForbidden(t *testing.T) {
	isolate(t)
	p := &fakePrompter{secrets: []string{testKey(t, "ws-test", time.Time{})}}
	a, srv, out, errb := initApp(t, 403, p)
	p.lines = []string{srv.URL}
	if code := a.run([]string{"init"}); code != 0 || !strings.Contains(out.String(), "company count unavailable") {
		t.Fatalf("exit %d: %s\n%s", code, errb, out)
	}
}

func TestInitSavesNothingWhenTheKeyIsNotAccepted(t *testing.T) {
	isolate(t)
	p := &fakePrompter{secrets: []string{testKey(t, "ws-test", time.Time{})}}
	a, srv, _, errb := initApp(t, 200, p)
	srv.openAPI = readFixture(t, "openapi-skeleton.json")
	p.lines = []string{srv.URL}
	if code := a.run([]string{"init"}); code != 1 || errLine(t, errb.String()).Kind != "auth" {
		t.Fatalf("exit %d: %s", code, errb)
	}
	if mustLoad(t) != (config.Config{}) {
		t.Fatal("init saved a configuration that did not verify")
	}
}

func TestInitRefusesExpiredAndUnreadableKeys(t *testing.T) {
	isolate(t)
	expired := testKey(t, "ws-test", time.Now().Add(-time.Hour))
	p := &fakePrompter{secrets: []string{expired, "garbage", expired}}
	a, srv, out, errb := initApp(t, 200, p)
	p.lines = []string{srv.URL}
	if code := a.run([]string{"init"}); code != 2 || !strings.Contains(out.String(), "expired") ||
		!strings.Contains(out.String(), "three base64url parts") {
		t.Fatalf("exit %d: %s\n%s", code, errb, out)
	}
	if len(srv.reqs) != 0 {
		t.Fatal("a refused key reached the server")
	}
}

func TestInitAsksBeforeReplacingAKeyForAnotherHost(t *testing.T) {
	isolate(t)
	old := config.Config{BaseURL: "https://old.example.com", APIKey: testKey(t, "ws-old", time.Time{})}
	config.Save(old)
	p := &fakePrompter{secrets: []string{testKey(t, "ws-test", time.Time{})}, confirms: []bool{false}}
	a, srv, _, errb := initApp(t, 200, p)
	p.lines = []string{srv.URL}
	if code := a.run([]string{"init"}); code != 2 || !strings.Contains(errLine(t, errb.String()).Message, "nothing was saved") {
		t.Fatalf("exit %d: %s", code, errb)
	}
	if mustLoad(t) != old {
		t.Fatal("the old configuration changed")
	}
}
