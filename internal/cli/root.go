package cli

import (
	"time"

	"github.com/spf13/cobra"
)

func (a *app) newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "twentycrm",
		Short: "CLI for the Twenty CRM API, built for agents: JSON in, JSON out",
		Long: "CLI for the Twenty CRM API, built for agents: JSON in, JSON out.\n\n" +
			"Every object of the connected workspace is a command: twentycrm companies list,\n" +
			"twentycrm people get <id>, twentycrm note-targets create --data @link.json.\n\n" +
			"stdout carries the API response and nothing else; errors are one line of JSON on stderr.\n" +
			"Exit codes: 0 success, 1 API or network error, 2 usage error.\n\n" +
			"Run `twentycrm schema` for the workspace's objects, `twentycrm schema <object>` for the\n" +
			"fields of one, and `twentycrm commands --json` for the machine-readable catalog.",
		// The root names no call. Printing help and exiting 0 would read as
		// success and break the stdout contract, so it is a usage error.
		RunE:              groupRunE,
		PersistentPreRunE: a.requireConfig,
		SilenceErrors:     true,
		SilenceUsage:      true,
	}
	pf := root.PersistentFlags()
	pf.Bool("verbose", false, "log requests to stderr (the API key is never logged)")
	pf.Duration("timeout", 30*time.Second, "per-attempt HTTP timeout")
	pf.Bool("force", false, "confirm a bulk-, destroy- or admin-class call")
	pf.String("output", "", "write the response body to a file instead of stdout")
	root.AddCommand(a.versionCommand(), a.metadataCommand())
	a.addObjectCommands(root)
	return root
}
