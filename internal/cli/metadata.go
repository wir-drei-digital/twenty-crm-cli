package cli

import (
	"github.com/spf13/cobra"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/api"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/routes"
)

func (a *app) metadataCommand() *cobra.Command {
	md := &cobra.Command{
		Use:   "metadata",
		Short: "Data model, views, page layouts and webhooks (Twenty's metadata API)",
		Long: "Data model, views, page layouts and webhooks (Twenty's metadata API).\n\n" +
			"Reading is read-class. Creating, updating and deleting is admin-class and needs --force;\n" +
			"deleting an object or a field removes its data for good. API keys can be read but never\n" +
			"created, changed or revoked here. objects and fields need the Data Model permission on\n" +
			"the key's role.",
		RunE: groupRunE,
	}
	for _, k := range routes.MetadataKinds {
		kind := k
		g := &cobra.Command{Use: kind.Command, Short: "Metadata: " + kind.Command, RunE: groupRunE}
		for _, v := range routes.MetadataVerbs {
			g.AddCommand(a.metadataVerbCommand(kind, v))
		}
		md.AddCommand(g)
	}
	return md
}

func (a *app) metadataVerbCommand(kind routes.MetaKind, v routes.Verb) *cobra.Command {
	blocked := routes.MetadataBlocked(kind.Command, v.Name)
	short := v.Summary
	if blocked != "" {
		short = "[blocked] " + short
	}
	use, args := v.Name, cobra.NoArgs
	if v.TakesID {
		use, args = v.Name+" <id>", cobra.ExactArgs(1)
	}
	cmd := &cobra.Command{
		Use: use, Short: short, Args: args,
		RunE: func(cmd *cobra.Command, args []string) error {
			id := ""
			if v.TakesID {
				id = args[0]
				if err := routes.ValidateID(id); err != nil {
					return api.Usagef("%v", err)
				}
			}
			q, err := verbQuery(cmd, v)
			if err != nil {
				return err
			}
			var body []byte
			if v.Body != routes.BodyNone {
				if body, err = a.readJSONBody(cmd); err != nil {
					return err
				}
			}
			command := "metadata " + kind.Command + " " + v.Name
			if err := routes.CheckBody(v.Body, body); err != nil {
				return api.Usagef("%s: %v", command, err)
			}
			d := routes.Decision{Command: command, Class: v.Class, Blocked: blocked, ReadOnly: a.res.ReadOnly, Force: flagBool(cmd, "force")}
			if err := d.Check(); err != nil {
				return api.Usagef("%v", err)
			}
			req := api.Request{Method: v.Method, Path: v.Path(kind.Segment, id), Query: q, Body: body, Risk: v.Class}
			return a.send(cmd, req, pager{rowsKey: kind.Segment, pageSize: 1000})
		},
	}
	registerVerbFlags(cmd, v)
	return cmd
}
