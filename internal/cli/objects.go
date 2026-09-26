package cli

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/api"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/model"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/routes"
)

// flagSpec is one verb flag: its type, the query parameter it sets ("" for
// flags the CLI consumes itself) and its help.
type flagSpec struct {
	kind  string // "string", "int" or "bool"
	query string
	usage string
}

var verbFlags = map[string]flagSpec{
	"filter":                 {"string", "filter", `Twenty filter, e.g. name[ilike]:"%acme%" (see --help)`},
	"order-by":               {"string", "order_by", "sort order, e.g. createdAt[DescNullsLast],name"},
	"limit":                  {"int", "limit", "records per page"},
	"depth":                  {"int", "depth", "0: the record only (Twenty's default); 1: with its direct relations"},
	"starting-after":         {"string", "starting_after", "cursor: start after this pageInfo.endCursor"},
	"ending-before":          {"string", "ending_before", "cursor: end before this pageInfo.startCursor"},
	"group-by":               {"string", "group_by", `fields to group by, as JSON: [{"city":true}]`},
	"aggregate":              {"string", "aggregate", `aggregates per group, as JSON: ["countNotEmptyId"]`},
	"view-id":                {"string", "view_id", "apply the filters of this view (a UUID)"},
	"include-records-sample": {"bool", "include_records_sample", "include sample records in each group"},
	"order-by-for-records":   {"string", "order_by_for_records", "order of the sample records in each group"},
	"upsert":                 {"bool", "upsert", "update the record instead when it already exists"},
	"all":                    {"bool", "", "follow every page and print one JSON array of the records"},
	"max-pages":              {"int", "", "page cap for --all"},
	"dry-run":                {"bool", "", "preview the merge without changing anything (no --force needed)"},
}

// objectAliases are the other names an object answers to: its API plural
// and its kebab-case singular, when they differ from the command.
func objectAliases(o *model.Object) []string {
	var out []string
	for _, s := range []string{o.NamePlural, model.CommandName(o.NameSingular)} {
		if s != o.Command && !contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// addObjectCommands hangs one group per object off root, each with the full
// verb table. A built-in wins over an object of the same name, and an alias
// never takes a name another object or a built-in already has.
func (a *app) addObjectCommands(root *cobra.Command) {
	if a.model == nil {
		return
	}
	taken := map[string]bool{}
	for _, n := range builtinNames {
		taken[n] = true
	}
	for _, o := range a.model.Objects {
		taken[o.Command] = true
	}
	for i := range a.model.Objects {
		obj := &a.model.Objects[i]
		if isBuiltin(obj.Command) {
			continue // shadowed: reachable through api; the catalog says so
		}
		short := obj.Description
		if short == "" {
			short = "Records of " + obj.NamePlural
		}
		group := &cobra.Command{Use: obj.Command, Short: short, RunE: groupRunE}
		for _, alias := range objectAliases(obj) {
			if !taken[alias] {
				group.Aliases = append(group.Aliases, alias)
				taken[alias] = true
			}
		}
		for _, v := range routes.ObjectVerbs {
			group.AddCommand(a.objectVerbCommand(obj, v))
		}
		root.AddCommand(group)
	}
}

func (a *app) objectVerbCommand(obj *model.Object, v routes.Verb) *cobra.Command {
	use, args := v.Name, cobra.NoArgs
	if v.TakesID {
		use, args = v.Name+" <id>", cobra.ExactArgs(1)
	}
	cmd := &cobra.Command{
		Use: use, Short: v.Summary, Args: args,
		RunE: func(cmd *cobra.Command, args []string) error { return a.runObjectVerb(cmd, obj, v, args) },
	}
	// Help is built on demand: rendering field tables for every command on
	// every run would cost more than the call itself.
	cmd.SetHelpFunc(func(c *cobra.Command, s []string) {
		c.Long = objectHelp(obj, v)
		c.Parent().HelpFunc()(c, s)
	})
	registerVerbFlags(cmd, v)
	return cmd
}

func registerVerbFlags(cmd *cobra.Command, v routes.Verb) {
	for _, name := range v.Flags {
		spec := verbFlags[name]
		switch spec.kind {
		case "int":
			def := 0
			if name == "max-pages" {
				def = 100
			}
			cmd.Flags().Int(name, def, spec.usage)
		case "bool":
			cmd.Flags().Bool(name, false, spec.usage)
		default:
			cmd.Flags().String(name, "", spec.usage)
		}
	}
	if v.Body != routes.BodyNone {
		cmd.Flags().String("data", "", "JSON body: literal, @file, or - for stdin")
	}
}

// verbQuery turns the verb's flags into query parameters, checks their
// ranges and adds the verb's fixed soft_delete value.
func verbQuery(cmd *cobra.Command, v routes.Verb) (url.Values, error) {
	q := url.Values{}
	for _, name := range v.Flags {
		spec := verbFlags[name]
		if spec.query == "" || !cmd.Flags().Changed(name) {
			continue
		}
		switch spec.kind {
		case "int":
			n, _ := cmd.Flags().GetInt(name)
			if name == "depth" && n != 0 && n != 1 {
				return nil, api.Usagef("--depth takes 0 or 1, got %d", n)
			}
			if name == "limit" && (n < 1 || n > v.MaxLimit) {
				return nil, api.Usagef("--limit takes 1 to %d, got %d", v.MaxLimit, n)
			}
			q.Set(spec.query, strconv.Itoa(n))
		case "bool":
			if flagBool(cmd, name) {
				q.Set(spec.query, "true")
			}
		default:
			// A blank value is left out: Twenty rejects an empty filter,
			// and a required filter is caught by the gates.
			if s := flagString(cmd, name); strings.TrimSpace(s) != "" {
				q.Set(spec.query, s)
			}
		}
	}
	if v.RequiredFlag != "" && q.Get(verbFlags[v.RequiredFlag].query) == "" {
		return nil, api.Usagef("--%s is required", v.RequiredFlag)
	}
	if v.SoftDelete != "" {
		q.Set("soft_delete", v.SoftDelete)
	}
	return q, nil
}

// runObjectVerb turns one parsed command into one API call. Every check
// runs before any network I/O, so a refused call has no side effects.
func (a *app) runObjectVerb(cmd *cobra.Command, obj *model.Object, v routes.Verb, args []string) error {
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
	command := obj.Command + " " + v.Name
	if err := routes.CheckBody(v.Body, body); err != nil {
		return api.Usagef("%s: %v", command, err)
	}
	class := v.Class
	if v.Name == "merge" && flagBool(cmd, "dry-run") {
		class = routes.ClassRead
		if body, err = setJSONField(body, "dryRun", true); err != nil {
			return err
		}
	}
	d := routes.Decision{Command: command, Class: class, FilterRequired: v.FilterRequired,
		Filter: q.Get("filter"), ReadOnly: a.res.ReadOnly, Force: flagBool(cmd, "force")}
	if err := d.Check(); err != nil {
		return api.Usagef("%v", err)
	}
	req := api.Request{Method: v.Method, Path: v.Path(obj.NamePlural, id), Query: q, Body: body, Risk: class, Object: obj.Command}
	return a.send(cmd, req, pager{rowsKey: obj.NamePlural, pageSize: 200})
}

// send runs one checked request, or a page walk with --all, and writes the
// result.
func (a *app) send(cmd *cobra.Command, req api.Request, p pager) error {
	if err := a.callable(); err != nil {
		return err
	}
	a.applyGlobalFlags(cmd)
	out, err := openOutput(flagString(cmd, "output"))
	if err != nil {
		return err
	}
	defer out.discard()
	if cmd.Flags().Lookup("all") != nil && flagBool(cmd, "all") {
		return a.runAll(cmd, req, p, out)
	}
	resp, err := a.client.Do(cmd.Context(), req)
	if err != nil {
		return err
	}
	return a.writeResponse(resp, out)
}

func (a *app) applyGlobalFlags(cmd *cobra.Command) {
	if flagBool(cmd, "verbose") {
		a.client.Verbose = a.stderr
	}
	if d, err := cmd.Flags().GetDuration("timeout"); err == nil {
		a.client.Timeout = d
	}
}
