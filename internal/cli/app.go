// Package cli builds the twentycrm command tree from the workspace model and
// turns process arguments into one API call, or one merged page walk.
//
// Two rules shape everything here: stdout carries the API response and
// nothing else, and every failure leaves as one line of JSON on stderr so an
// agent can parse it without heuristics.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/api"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/config"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/model"
)

type app struct {
	model  *model.Model
	client *api.Client
	res    config.Resolved
	// configErr is why the configuration could not be resolved. Only
	// version, help and the config commands run without it, so a person can
	// find and repair the file; every other command reports it.
	configErr      error
	stdout, stderr io.Writer
	stdin          io.Reader
	// cacheDir locates the user cache directory; nil means os.UserCacheDir.
	cacheDir func() (string, error)
	// now is the clock for cache ages and key expiry; nil means time.Now.
	now func() time.Time
}

// Execute is the process entry point: it resolves the configuration, runs
// the command tree and returns the exit code (0 success, 1 API or network
// failure, 2 usage error).
func Execute(args []string) int {
	// Ctrl-C and SIGTERM cancel the request in flight and any retry wait.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	a := &app{stdout: os.Stdout, stderr: os.Stderr, stdin: os.Stdin}
	a.configure(os.Getenv, contextSleeper(ctx))
	return a.runContext(ctx, args)
}

// configure resolves the config file and the environment and builds the
// client. A failure is kept in configErr rather than returned.
func (a *app) configure(getenv func(string) string, sleep func(time.Duration)) {
	a.res, a.configErr = config.Resolve(getenv)
	a.client = &api.Client{BaseURL: a.res.BaseURL, APIKey: a.res.APIKey, ReadOnly: a.res.ReadOnly, Sleep: sleep}
}

func (a *app) clock() time.Time {
	if a.now != nil {
		return a.now()
	}
	return time.Now()
}

// cacheRoot is the user cache directory, or "" when there is none; the model
// cache is then skipped.
func (a *app) cacheRoot() string {
	dir := a.cacheDir
	if dir == nil {
		dir = os.UserCacheDir
	}
	root, err := dir()
	if err != nil {
		return ""
	}
	return root
}

// callable is the check every API call passes first: a key from the config
// file only goes to the base URL stored with it. Missing values are left to
// the client, which names the fix.
func (a *app) callable() error {
	if err := a.res.BindingError(); err != nil {
		return api.Usagef("%v", err)
	}
	return nil
}

// requireConfig is the root's persistent pre-run: with an unresolvable
// configuration only version, help and the config commands run.
func (a *app) requireConfig(cmd *cobra.Command, args []string) error {
	if a.configErr == nil {
		return nil
	}
	for c := cmd; c.HasParent(); c = c.Parent() {
		if !c.Parent().HasParent() {
			switch c.Name() {
			case "version", "config", "help":
				return nil
			}
		}
	}
	return api.Usagef("cannot load the configuration: %v", a.configErr)
}

// contextSleeper returns a sleep that gives up as soon as ctx is done.
func contextSleeper(ctx context.Context) func(time.Duration) {
	return func(d time.Duration) {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
		case <-t.C:
		}
	}
}

func (a *app) run(args []string) int { return a.runContext(context.Background(), args) }

func (a *app) runContext(ctx context.Context, args []string) int {
	if w := firstWord(args); a.configErr != nil && w != "" && !isBuiltin(w) {
		// Without a configuration there are no object commands; saying
		// "unknown command" would hide the real problem.
		return a.renderError(api.Usagef("cannot load the configuration: %v", a.configErr))
	}
	if a.configErr == nil {
		if err := a.prepareModel(ctx, args); err != nil {
			return a.renderError(err)
		}
	}
	root := a.newRoot()
	root.SetArgs(args)
	root.SetOut(a.stdout)
	root.SetErr(a.stderr)
	if err := root.ExecuteContext(ctx); err != nil {
		return a.renderError(err)
	}
	return 0
}

// renderError writes err as one line of JSON on stderr and returns the exit
// code: 2 for usage errors, 1 for everything the API or the network produced.
func (a *app) renderError(err error) int {
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		apiErr = api.Usagef("%v", err) // anything else is a cobra parse or usage failure
	}
	raw, mErr := jsonCompact(apiErr)
	if mErr != nil {
		raw, mErr = jsonCompact(&api.Error{Kind: apiErr.Kind, Message: apiErr.Message, Status: apiErr.Status})
		if mErr != nil {
			raw = []byte(`{"kind":"usage","error":"internal: cannot render error","details":null}`)
		}
	}
	fmt.Fprintln(a.stderr, string(raw))
	if apiErr.Kind == api.KindUsage {
		return 2
	}
	return 1
}
