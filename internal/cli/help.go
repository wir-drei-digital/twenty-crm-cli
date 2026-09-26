package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/model"
	"github.com/wir-drei-digital/twenty-crm-cli/internal/routes"
)

const filterHelp = `
Filter syntax (--filter): field[comparator]:value, joined by commas (all must match), or
wrapped in or(...) and not(...). Composite fields take a dot: emails.primaryEmail[eq]:ana@example.com.
Quote values that hold commas or spaces.
Comparators: eq, neq, in, containsAny, is, gt, gte, lt, lte, startsWith, endsWith, like, ilike.
  --filter 'name[ilike]:"%acme%"'
  --filter 'or(stage[eq]:LEAD,employees[gt]:50)'
  --filter 'deletedAt[is]:NOT_NULL'
Order (--order-by): field[AscNullsFirst|AscNullsLast|DescNullsFirst|DescNullsLast], comma-separated.
`

// objectHelp is the long help of one object verb: the request it sends, its
// class and gates, and the object's fields from the cached model.
func objectHelp(obj *model.Object, v routes.Verb) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s.\n\nRequest: %s /%s", v.Summary, v.Method, v.Path(obj.NamePlural, "<id>"))
	if v.SoftDelete != "" {
		fmt.Fprintf(&b, "?soft_delete=%s", v.SoftDelete)
	}
	fmt.Fprintf(&b, "\nClass: %s", v.Class)
	if routes.NeedsForce(v.Class) {
		b.WriteString(" (needs --force)")
	}
	if v.FilterRequired {
		b.WriteString("; --filter is required")
	}
	if v.Name == "merge" {
		b.WriteString("; read-class with --dry-run")
	}
	b.WriteString("\n")
	if contains(v.Flags, "filter") {
		b.WriteString(filterHelp)
	}
	switch v.Body {
	case routes.BodyArray:
		fmt.Fprintf(&b, "\n--data takes a JSON array of 1 to %d records; the CLI never splits a batch.\n", routes.MaxBatch)
	case routes.BodyMerge:
		b.WriteString("\n--data: {\"ids\":[\"<id>\",\"<id>\"],\"conflictPriorityIndex\":0}; the record at that index wins conflicts.\n")
	case routes.BodyDuplicates:
		b.WriteString("\n--data: {\"ids\":[\"<id>\"]} or {\"data\":[{\"name\":\"Acme\"}]}.\n")
	}
	fmt.Fprintf(&b, "\nFields of %s (from the cached workspace model; `twentycrm schema %s` reads them live):\n", obj.NamePlural, obj.Command)
	tw := tabwriter.NewWriter(&b, 2, 4, 2, ' ', 0)
	for _, f := range obj.Fields {
		typ := f.Type
		if f.Format != "" {
			typ += "/" + f.Format
		}
		var notes []string
		if f.Required {
			notes = append(notes, "required")
		}
		if f.ReadOnly {
			notes = append(notes, "read-only")
		}
		if len(f.Enum) > 0 {
			notes = append(notes, "one of: "+strings.Join(f.Enum, ", "))
		}
		if len(f.Subfields) > 0 {
			notes = append(notes, "subfields: "+strings.Join(f.Subfields, ", "))
		}
		if f.Relation != nil {
			rel := f.Relation.Kind + " -> " + f.Relation.Target
			if f.Relation.Kind == "many_to_one" {
				rel += " (set " + f.Name + "Id)"
			}
			notes = append(notes, rel)
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\n", f.Name, typ, strings.Join(notes, "; "))
	}
	tw.Flush()
	return b.String()
}
