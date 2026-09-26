package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/api"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/auth"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/config"
)

func (a *app) configCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Manage the twentycrm config file", RunE: groupRunE}
	set := &cobra.Command{Use: "set", Short: "Set a config value", Args: cobra.ArbitraryArgs, RunE: keyGroupRunE("set")}
	set.AddCommand(
		&cobra.Command{
			Use:   "base-url <url>",
			Short: "Save the Twenty base URL, e.g. https://crm.example.com (Twenty Cloud: https://api.twenty.com)",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				n, err := config.NormalizeBaseURL(args[0])
				if err != nil {
					return api.Usagef("base-url: %v", err)
				}
				c, err := config.Load()
				if err != nil {
					return api.Usagef("%v", err)
				}
				if c.APIKey != "" && c.BaseURL != "" && c.BaseURL != n {
					return api.Usagef("the stored API key belongs to %s; run `twentycrm config unset api-key` first, then set the new base URL and its key", c.BaseURL)
				}
				c.BaseURL = n
				if err := config.Save(c); err != nil {
					return api.Usagef("%v", err)
				}
				a.warnEnv("TWENTY_BASE_URL")
				fmt.Fprintf(a.stdout, "base_url = %s\n", n)
				return nil
			},
		},
		&cobra.Command{
			Use:   "api-key",
			Short: "Read an API key from stdin, check it and store it (0600), bound to the stored base URL",
			Long: "Read a Twenty API key from stdin, check that it is an API key, and store it in the config file (0600).\n" +
				"The key never comes from the command line, which every process on the machine can see:\n" +
				"  twentycrm config set api-key < key.txt\n\n" +
				"Set the base URL first: the key is only ever sent to the base URL stored with it.\n" +
				"TWENTY_API_KEY, when set, wins.",
			Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := config.Load()
				if err != nil {
					return api.Usagef("%v", err)
				}
				if c.BaseURL == "" {
					return api.Usagef("set the base URL first: `twentycrm config set base-url <url>`; the key is bound to it")
				}
				raw, err := io.ReadAll(io.LimitReader(a.stdin, 64<<10))
				if err != nil {
					return api.Usagef("reading stdin: %v", err)
				}
				keyText := auth.CleanKey(string(raw))
				if keyText == "" {
					return api.Usagef("no key on stdin; usage: twentycrm config set api-key < key.txt")
				}
				key, err := auth.ParseAPIKey(keyText)
				if err != nil {
					return api.Usagef("%v", err)
				}
				c.APIKey = keyText
				if err := config.Save(c); err != nil {
					return api.Usagef("%v", err)
				}
				a.warnEnv("TWENTY_API_KEY")
				fmt.Fprintf(a.stdout, "api key saved for %s (workspace %s, key id %s, %s)\n", c.BaseURL, key.WorkspaceID, key.KeyID, expiryText(key))
				return nil
			},
		},
		&cobra.Command{
			Use:   "read-only <true|false>",
			Short: "Persist read-only mode",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				v, err := strconv.ParseBool(args[0])
				if err != nil {
					return api.Usagef("read-only wants true or false, got %q", args[0])
				}
				c, err := config.Load()
				if err != nil {
					return api.Usagef("%v", err)
				}
				c.ReadOnly = v
				if err := config.Save(c); err != nil {
					return api.Usagef("%v", err)
				}
				fmt.Fprintf(a.stdout, "read_only = %v\n", v)
				return nil
			},
		},
	)

	unset := &cobra.Command{Use: "unset", Short: "Remove a config value", Args: cobra.ArbitraryArgs, RunE: keyGroupRunE("unset")}
	for _, k := range []struct {
		use, short, done string
		stored           func(config.Config) bool
		refuse           func(config.Config) string
		clear            func(*config.Config)
	}{
		{"base-url", "Remove the stored base URL", "base-url removed",
			func(c config.Config) bool { return c.BaseURL != "" },
			func(c config.Config) string {
				if c.APIKey != "" {
					return "the stored API key is bound to this base URL; run `twentycrm config unset api-key` first"
				}
				return ""
			},
			func(c *config.Config) { c.BaseURL = "" }},
		{"api-key", "Remove the stored API key from this machine",
			"api key removed from this machine; it stays valid until you revoke it in Twenty under Settings, APIs & Webhooks",
			func(c config.Config) bool { return c.APIKey != "" }, func(config.Config) string { return "" },
			func(c *config.Config) { c.APIKey = "" }},
		{"read-only", "Stop persisting read-only mode", "read-only removed",
			func(c config.Config) bool { return c.ReadOnly }, func(config.Config) string { return "" },
			func(c *config.Config) { c.ReadOnly = false }},
	} {
		unset.AddCommand(&cobra.Command{
			Use: k.use, Short: k.short, Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := config.Load()
				if err != nil {
					return api.Usagef("%v", err)
				}
				if !k.stored(c) {
					fmt.Fprintf(a.stdout, "%s was not set\n", k.use)
					return nil
				}
				if why := k.refuse(c); why != "" {
					return api.Usagef("%s", why)
				}
				k.clear(&c)
				if err := config.Save(c); err != nil {
					return api.Usagef("%v", err)
				}
				fmt.Fprintln(a.stdout, k.done)
				return nil
			},
		})
	}

	path := &cobra.Command{
		Use: "path", Short: "Print the config file location", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := config.Path()
			if err != nil {
				return api.Usagef("%v", err)
			}
			fmt.Fprintln(a.stdout, p)
			return nil
		},
	}
	cmd.AddCommand(set, unset, path)
	return cmd
}

// expiryText renders a key's expiry for people.
func expiryText(k *auth.Key) string {
	if k.ExpiresAt.IsZero() {
		return "never expires"
	}
	return "expires " + k.ExpiresAt.Format("2006-01-02")
}

// warnEnv says on stderr when an environment variable overrides what was
// just saved.
func (a *app) warnEnv(name string) {
	if a.res.FromEnv[name] {
		fmt.Fprintf(a.stderr, "note: %s is set in this environment and wins over the config file\n", name)
	}
}

// keyGroupRunE answers `config set` or `config unset` given something that is
// not one of their keys. An environment variable name is the expected
// mistake, so it is answered with the key it plainly means; any other name
// is answered with the valid keys.
func keyGroupRunE(verb string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			if k := suggestConfigKey(args[0]); k != "" {
				return api.Usagef("config keys are base-url, api-key and read-only, not environment variable names; "+
					"did you mean `twentycrm config %s %s`?", verb, k)
			}
			return api.Usagef("unknown config key %q; the keys are base-url, api-key and read-only", args[0])
		}
		return groupRunE(cmd, args)
	}
}

func suggestConfigKey(given string) string {
	switch strings.TrimPrefix(strings.ToLower(given), "twenty_") {
	case "base_url", "baseurl", "url", "base":
		return "base-url"
	case "api_key", "apikey", "key", "token":
		return "api-key"
	case "read_only", "readonly":
		return "read-only"
	}
	return ""
}
