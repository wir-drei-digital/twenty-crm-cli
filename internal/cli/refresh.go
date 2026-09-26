package cli

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/api"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/auth"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/model"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/routes"
)

// builtinNames are the top-level commands that are not objects. An object
// with one of these names is shadowed and stays reachable through `api`.
var builtinNames = []string{"version", "commands", "schema", "api", "config", "auth", "init", "metadata", "help", "completion"}

func isBuiltin(name string) bool {
	return slices.Contains(builtinNames, name) || strings.HasPrefix(name, "__complete")
}

// valueFlags are the global flags whose value may come as the next argument.
var valueFlags = map[string]bool{"--timeout": true, "--output": true}

// firstWord is the command's first argument that is not a flag.
func firstWord(args []string) string {
	for i := 0; i < len(args); i++ {
		s := args[i]
		switch {
		case s == "--":
			if i+1 < len(args) {
				return args[i+1]
			}
			return ""
		case strings.HasPrefix(s, "-"):
			if valueFlags[s] {
				i++
			}
		default:
			return s
		}
	}
	return ""
}

// workspaceID is the workspace named in the API key, or "" when the key
// cannot be read. It keys the model cache.
func (a *app) workspaceID() string {
	k, err := auth.Decode(a.res.APIKey)
	if err != nil {
		return ""
	}
	return k.WorkspaceID
}

// prepareModel loads the cached model and refreshes it when the command
// names an object the cache cannot vouch for (spec: Workspace model, Cache).
// It fails only when the command cannot be resolved without the refresh that
// just failed.
func (a *app) prepareModel(ctx context.Context, args []string) error {
	a.model = model.LoadCache(a.cacheRoot(), a.res.BaseURL, a.workspaceID())
	w := firstWord(args)
	if w == "" || isBuiltin(w) {
		return nil
	}
	if !a.model.Stale(a.clock()) && a.model.Find(w) != nil {
		return nil
	}
	fresh, err := a.fetchModel(ctx)
	if err == nil {
		a.model = fresh
		return nil
	}
	if a.model.Find(w) != nil {
		return nil // stale but usable; the call itself reports an outage
	}
	return err
}

// fetchModel reads the workspace's OpenAPI document, extracts the model and
// caches it. Saving is best effort: a read-only home must not break a call.
func (a *app) fetchModel(ctx context.Context) (*model.Model, error) {
	if err := a.callable(); err != nil {
		return nil, err
	}
	resp, err := a.client.Do(ctx, api.Request{Method: "GET", Path: "rest/open-api/core", Risk: routes.ClassRead})
	if err != nil {
		return nil, err
	}
	objs, err := model.Extract(resp.Body)
	if errors.Is(err, model.ErrNoObjects) {
		return nil, &api.Error{Kind: api.KindAuth, Message: "GET rest/open-api/core: " + err.Error()}
	}
	if err != nil {
		return nil, &api.Error{Kind: api.KindServer, Message: "GET rest/open-api/core: " + err.Error()}
	}
	m := &model.Model{FetchedAt: a.clock().UTC(), BaseURL: a.res.BaseURL, WorkspaceID: a.workspaceID(), Objects: objs}
	if root := a.cacheRoot(); root != "" {
		_ = model.SaveCache(root, m)
	}
	return m, nil
}
