package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/api"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/routes"
)

// The catalog is `twentycrm commands --json`: schema_version is the contract
// a consumer pins to; new optional keys may appear under the same version.
type catalogObject struct {
	Command      string `json:"command"`
	NamePlural   string `json:"name_plural"`
	NameSingular string `json:"name_singular"`
	ShadowedBy   string `json:"shadowed_by,omitempty"`
}

type catalogVerb struct {
	Verb           string   `json:"verb"`
	Method         string   `json:"method"`
	Path           string   `json:"path"`
	Class          string   `json:"class"`
	TakesID        bool     `json:"takes_id"`
	FilterRequired bool     `json:"filter_required"`
	SoftDelete     string   `json:"soft_delete,omitempty"`
	Filter         string   `json:"filter,omitempty"` // a fixed filter, with {id} for the record ID
	Body           string   `json:"body,omitempty"`
	MaxRecords     int      `json:"max_records,omitempty"`
	Flags          []string `json:"flags"`
	Summary        string   `json:"summary"`
}

type catalogMeta struct {
	Command       string `json:"command"`
	Method        string `json:"method"`
	Path          string `json:"path"`
	Class         string `json:"class"`
	TakesID       bool   `json:"takes_id"`
	Blocked       bool   `json:"blocked,omitempty"`
	BlockedReason string `json:"blocked_reason,omitempty"`
}

type catalogCommand struct {
	Command string `json:"command"`
	Summary string `json:"summary"`
}

type catalog struct {
	SchemaVersion  int              `json:"schema_version"`
	ModelFetchedAt *time.Time       `json:"model_fetched_at,omitempty"`
	ModelMissing   bool             `json:"model_missing,omitempty"`
	Objects        []catalogObject  `json:"objects"`
	ObjectVerbs    []catalogVerb    `json:"object_verbs"`
	Metadata       []catalogMeta    `json:"metadata"`
	Commands       []catalogCommand `json:"commands"`
}

func verbCatalog(v routes.Verb) catalogVerb {
	flags := append([]string{}, v.Flags...)
	if v.Body != routes.BodyNone {
		flags = append(flags, "data")
	}
	e := catalogVerb{Verb: v.Name, Method: v.Method, Path: v.PathTemplate, Class: v.Class, TakesID: v.TakesID,
		FilterRequired: v.FilterRequired, SoftDelete: v.SoftDelete, Body: string(v.Body), Flags: flags, Summary: v.Summary}
	if v.Body == routes.BodyArray {
		e.MaxRecords = routes.MaxBatch
	}
	if v.IDFilter {
		e.Filter = routes.FilterByID("{id}")
	}
	return e
}

// buildCatalog describes the tree from the cached model; it never fetches.
func (a *app) buildCatalog(root *cobra.Command) catalog {
	c := catalog{SchemaVersion: 1, Objects: []catalogObject{}, ObjectVerbs: []catalogVerb{}, Metadata: []catalogMeta{}, Commands: []catalogCommand{}}
	if a.model == nil {
		c.ModelMissing = true
	} else {
		t := a.model.FetchedAt
		c.ModelFetchedAt = &t
		for _, o := range a.model.Objects {
			e := catalogObject{Command: o.Command, NamePlural: o.NamePlural, NameSingular: o.NameSingular}
			if isBuiltin(o.Command) {
				e.ShadowedBy = o.Command
			}
			c.Objects = append(c.Objects, e)
		}
	}
	for _, v := range routes.ObjectVerbs {
		c.ObjectVerbs = append(c.ObjectVerbs, verbCatalog(v))
	}
	for _, k := range routes.MetadataKinds {
		for _, v := range routes.MetadataVerbs {
			reason := routes.MetadataBlocked(k.Command, v.Name)
			c.Metadata = append(c.Metadata, catalogMeta{Command: "metadata " + k.Command + " " + v.Name, Method: v.Method,
				Path: v.Path(k.Segment, "{id}"), Class: v.Class, TakesID: v.TakesID, Blocked: reason != "", BlockedReason: reason})
		}
	}
	for _, cmd := range root.Commands() {
		if isBuiltin(cmd.Name()) && cmd.Name() != "help" && cmd.Name() != "completion" {
			c.Commands = append(c.Commands, catalogCommand{Command: cmd.Name(), Summary: cmd.Short})
		}
	}
	return c
}

func (a *app) commandsCommand() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "commands",
		Short: "List the object verbs, metadata commands and objects (--json for the machine-readable catalog)",
		Long: "List the command grammar. The verb table is the same for every object, so the catalog lists it once,\n" +
			"next to the workspace's objects from the cached model. When no model is cached yet, model_missing is\n" +
			"true; `twentycrm schema` loads it.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c := a.buildCatalog(cmd.Root())
			if asJSON {
				return a.emit(cmd, c)
			}
			for _, o := range c.Objects {
				fmt.Fprintf(a.stdout, "object  %s\n", o.Command)
			}
			for _, v := range c.ObjectVerbs {
				path := v.Path
				if v.Filter != "" {
					path += "?filter=" + v.Filter
				}
				fmt.Fprintf(a.stdout, "verb    %-16s %-6s %-30s [%s]\n", v.Verb, v.Method, path, v.Class)
			}
			for _, m := range c.Metadata {
				tag := ""
				if m.Blocked {
					tag = " blocked"
				}
				fmt.Fprintf(a.stdout, "%-40s %-6s [%s]%s\n", m.Command, m.Method, m.Class, tag)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit the machine-readable catalog")
	return cmd
}

// emit writes v as one line of JSON to stdout, or to --output.
func (a *app) emit(cmd *cobra.Command, v any) error {
	raw, err := jsonCompact(v)
	if err != nil {
		return err
	}
	out, err := openOutput(flagString(cmd, "output"))
	if err != nil {
		return err
	}
	defer out.discard()
	return a.writeResponse(&api.Response{Body: append(raw, '\n')}, out)
}
