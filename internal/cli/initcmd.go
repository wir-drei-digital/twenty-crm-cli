package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"

	"github.com/spf13/cobra"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/api"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/auth"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/config"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/model"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/routes"
)

func (a *app) initCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Interactive setup: base URL, API key, read-only mode (needs a terminal)",
		Long: "Interactive setup. Asks for the Twenty base URL and an API key (hidden input), checks both with two\n" +
			"read-only calls, asks whether to switch on read-only mode, and saves everything to the config file\n" +
			"(0600). Needs a terminal: agents configure twentycrm with `twentycrm config set` or the environment.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			isTTY := a.isTerminal
			if isTTY == nil {
				isTTY = stdioIsTerminal
			}
			if !isTTY() {
				return api.Usagef("twentycrm init is interactive and needs a terminal; use `twentycrm config set ...` instead")
			}
			p := a.prompt
			if p == nil {
				p = newTermPrompter(os.Stdin, a.stdout)
			}
			return a.runInit(cmd.Context(), p)
		},
	}
}

func (a *app) runInit(ctx context.Context, p prompter) error {
	out := a.stdout
	fmt.Fprintln(out, "twentycrm setup: you need your Twenty address and an API key (Twenty: Settings, APIs & Webhooks).")
	for _, name := range []string{"TWENTY_BASE_URL", "TWENTY_API_KEY", "TWENTY_READ_ONLY"} {
		if a.res.FromEnv[name] {
			fmt.Fprintf(out, "Note: %s is set in this shell and wins over what you save here.\n", name)
		}
	}
	cur, err := config.Load()
	if err != nil {
		return api.Usagef("%v", err)
	}
	base, err := askValid(p, out, "Twenty base URL (e.g. https://crm.example.com; Twenty Cloud: https://api.twenty.com): ", config.NormalizeBaseURL)
	if err != nil {
		return err
	}
	if cur.APIKey != "" && cur.BaseURL != "" && cur.BaseURL != base {
		ok, err := p.Confirm(fmt.Sprintf("The config file holds a key for %s. Replace it?", cur.BaseURL), false)
		if err != nil || !ok {
			return api.Usagef("init: nothing was saved")
		}
	}
	keyText, key, err := a.askKey(p)
	if err != nil {
		return err
	}
	client := &api.Client{BaseURL: base, APIKey: keyText, Sleep: a.client.Sleep}
	resp, err := client.Do(ctx, api.Request{Method: "GET", Path: "rest/open-api/core", Risk: routes.ClassRead})
	if err != nil {
		fmt.Fprintln(out, "Verification failed; nothing was saved.")
		return err
	}
	objs, err := model.Extract(resp.Body)
	if err != nil {
		fmt.Fprintln(out, "Verification failed; nothing was saved.")
		if errors.Is(err, model.ErrNoObjects) {
			return &api.Error{Kind: api.KindAuth, Message: "init: " + err.Error()}
		}
		return &api.Error{Kind: api.KindServer, Message: "init: " + err.Error()}
	}
	count := "company count unavailable"
	if r, err := client.Do(ctx, api.Request{Method: "GET", Path: "rest/companies", Risk: routes.ClassRead,
		Query: url.Values{"limit": {"1"}, "depth": {"0"}}}); err == nil {
		var env struct {
			TotalCount *int `json:"totalCount"`
		}
		if json.Unmarshal(r.Body, &env) == nil && env.TotalCount != nil {
			count = fmt.Sprintf("%d companies", *env.TotalCount)
		}
	}
	fmt.Fprintf(out, "Workspace %s: %d objects, %s. Key %s %s.\n", key.WorkspaceID, len(objs), count, key.KeyID, expiryText(key))
	ro, err := p.Confirm("Switch on read-only mode (reads only, no changes)?", false)
	if err != nil {
		return api.Usagef("init: %v; nothing was saved", err)
	}
	cur.BaseURL, cur.APIKey, cur.ReadOnly = base, keyText, ro
	if err := config.Save(cur); err != nil {
		return api.Usagef("%v", err)
	}
	if root := a.cacheRoot(); root != "" {
		_ = model.SaveCache(root, &model.Model{FetchedAt: a.clock().UTC(), BaseURL: base, WorkspaceID: key.WorkspaceID, Objects: objs})
	}
	p2, _ := config.Path()
	fmt.Fprintf(out, "Saved to %s. Next: twentycrm schema\n", p2)
	return nil
}

// askKey asks for the key until it is an unexpired API key, three times at
// most. Nothing is sent before it passes.
func (a *app) askKey(p prompter) (string, *auth.Key, error) {
	for range 3 {
		raw, err := ask(p, "API key (input hidden): ", true)
		if err != nil {
			return "", nil, api.Usagef("init: %v", err)
		}
		text := auth.CleanKey(raw)
		key, err := auth.ParseAPIKey(text)
		if err != nil {
			fmt.Fprintln(a.stdout, err)
			continue
		}
		if key.Expired(a.clock()) {
			fmt.Fprintf(a.stdout, "This key expired on %s; create a new one in Twenty under Settings, APIs & Webhooks.\n", key.ExpiresAt.Format("2006-01-02"))
			continue
		}
		return text, key, nil
	}
	return "", nil, api.Usagef("init: no usable API key after 3 attempts; nothing was saved")
}

// askValid asks until parse accepts the answer, three times at most.
func askValid[T any](p prompter, out io.Writer, prompt string, parse func(string) (T, error)) (T, error) {
	var zero T
	for range 3 {
		s, err := ask(p, prompt, false)
		if err != nil {
			return zero, api.Usagef("init: %v", err)
		}
		v, err := parse(s)
		if err == nil {
			return v, nil
		}
		fmt.Fprintln(out, err)
	}
	return zero, api.Usagef("init: no valid answer after 3 attempts")
}
