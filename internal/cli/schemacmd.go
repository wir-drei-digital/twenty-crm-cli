package cli

import (
	"github.com/spf13/cobra"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/api"
)

type objectSummary struct {
	Command      string `json:"command"`
	NamePlural   string `json:"name_plural"`
	NameSingular string `json:"name_singular"`
	Description  string `json:"description,omitempty"`
	Fields       int    `json:"fields"`
}

func (a *app) schemaCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "schema [object]",
		Short: "List the workspace's objects, or print one object's fields (read live)",
		Long: "Without an argument, list the workspace's objects. With one, print that object's fields as JSON:\n" +
			"type, format, the allowed values of select fields (enum), subfields of composite fields, required\n" +
			"(needed on create), read_only (set by Twenty) and relations. The object is its command name or its\n" +
			"API name (note-targets or noteTargets). Always reads the workspace live and refreshes the cache;\n" +
			"works with any valid key, whatever its role.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a.applyGlobalFlags(cmd)
			m, err := a.fetchModel(cmd.Context())
			if err != nil {
				return err
			}
			a.model = m
			if len(args) == 0 {
				list := make([]objectSummary, 0, len(m.Objects))
				for _, o := range m.Objects {
					list = append(list, objectSummary{Command: o.Command, NamePlural: o.NamePlural, NameSingular: o.NameSingular,
						Description: o.Description, Fields: len(o.Fields)})
				}
				return a.emit(cmd, list)
			}
			obj := m.Find(args[0])
			if obj == nil {
				return api.Usagef("this workspace has no object %q; `twentycrm schema` lists them", args[0])
			}
			return a.emit(cmd, obj)
		},
	}
}
