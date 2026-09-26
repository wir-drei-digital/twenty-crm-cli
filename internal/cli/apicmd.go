package cli

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/api"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/routes"
)

// apiCommand is the escape hatch for anything the tree lacks. It is not a
// bypass: the path takes its class from the route grammar, and the gates are
// the same as for commands (spec: The api escape hatch).
func (a *app) apiCommand() *cobra.Command {
	var queries, headers []string
	cmd := &cobra.Command{
		Use:   "api <METHOD> <path>",
		Short: "Raw authenticated request (escape hatch); the guardrails still apply",
		Long: "Raw authenticated request to a path under rest/, relative to the base URL, for example\n" +
			"rest/companies or rest/metadata/objects. Query parameters come from --query k=v, never from the path.\n\n" +
			"The path takes its class from Twenty's route grammar: a DELETE without soft_delete=true deletes\n" +
			"permanently and needs --force, a PATCH or DELETE on a whole collection needs --filter and --force,\n" +
			"metadata changes need --force, API keys cannot be changed, and an unknown non-GET path needs --force.\n" +
			"Read-only mode allows read-class calls only. Authorization and the method-override headers cannot\n" +
			"be set; GET and DELETE take no --data; each path segment must be non-empty and must not be . or ..\n" +
			"or contain %, whitespace or a backslash. GraphQL is not supported.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			method := strings.ToUpper(args[0])
			q := url.Values{}
			for _, kv := range queries {
				k, v, ok := strings.Cut(kv, "=")
				if !ok || k == "" {
					return api.Usagef("--query wants k=v, got %q", kv)
				}
				q.Add(k, v)
			}
			hdr := http.Header{}
			for _, kv := range headers {
				k, v, ok := strings.Cut(kv, ":")
				if !ok || strings.TrimSpace(k) == "" {
					return api.Usagef("--header wants k:v, got %q", kv)
				}
				hdr.Add(strings.TrimSpace(k), strings.TrimSpace(v))
			}
			raw, err := routes.ClassifyRaw(method, args[1], q)
			if err != nil {
				return api.Usagef("%v", err)
			}
			path, _ := routes.CleanRawPath(args[1])
			body, err := a.readJSONBody(cmd)
			if err != nil {
				return err
			}
			if body != nil && (method == http.MethodGet || method == http.MethodDelete) {
				return api.Usagef("%s takes no request body; drop --data", method)
			}
			d := routes.Decision{Command: "api " + method + " " + path, Class: raw.Class, Blocked: raw.Blocked,
				FilterRequired: raw.FilterRequired, Filter: q.Get("filter"), ReadOnly: a.res.ReadOnly, Force: flagBool(cmd, "force")}
			if err := d.Check(); err != nil {
				return api.Usagef("%v", err)
			}
			return a.send(cmd, api.Request{Method: method, Path: path, Query: q, Body: body, Risk: raw.Class, Headers: hdr}, pager{})
		},
	}
	cmd.Flags().String("data", "", "JSON body: literal, @file, or - for stdin")
	cmd.Flags().StringArrayVar(&queries, "query", nil, "query parameter k=v (repeatable)")
	cmd.Flags().StringArrayVar(&headers, "header", nil, "extra header k:v (repeatable; Authorization cannot be set)")
	return cmd
}
