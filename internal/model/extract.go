package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ErrNoObjects means the document describes no object. Twenty serves an
// empty document, with status 200, to a caller whose token it does not
// accept, so this is almost always a key problem.
var ErrNoObjects = errors.New("the workspace's OpenAPI document lists no objects; Twenty sends an empty document " +
	"when it does not accept the API key, so check `twentycrm auth status` and the key in Twenty (Settings, APIs & Webhooks)")

type schema struct {
	Type        any               `json:"type"` // a string, or an array of strings in OpenAPI 3.1
	Format      string            `json:"format"`
	Enum        []any             `json:"enum"`
	Items       *schema           `json:"items"`
	Ref         string            `json:"$ref"`
	OneOf       []schema          `json:"oneOf"`
	Properties  map[string]schema `json:"properties"`
	Required    []string          `json:"required"`
	Description string            `json:"description"`
}

type document struct {
	Paths map[string]map[string]json.RawMessage `json:"paths"`
	Tags  []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"tags"`
	Components struct {
		Schemas map[string]schema `json:"schemas"`
	} `json:"components"`
}

// Extract reads the objects and fields out of GET /rest/open-api/core
// (spec: Workspace model). The result is sorted by command, each object's
// fields by name.
func Extract(raw []byte) ([]Object, error) {
	var d document
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("the workspace's OpenAPI document is not valid JSON: %v", err)
	}
	if d.Paths == nil {
		return nil, errors.New("the workspace's OpenAPI document has no paths")
	}
	tags := map[string]string{}
	for _, t := range d.Tags {
		tags[t.Name] = t.Description
	}
	type found struct{ plural, singular string }
	var objs []found
	for path, item := range d.Paths {
		plural, ok := strings.CutPrefix(path, "/")
		if !ok || plural == "" || strings.Contains(plural, "/") {
			continue
		}
		if operationID(item, "get") != "findMany"+upperFirst(plural) {
			continue
		}
		singular, ok := strings.CutPrefix(operationID(item, "post"), "createOne")
		if !ok || singular == "" {
			continue
		}
		objs = append(objs, found{plural, lowerFirst(singular)})
	}
	if len(objs) == 0 {
		return nil, ErrNoObjects
	}
	pluralOf := map[string]string{} // "Company" -> "companies"
	for _, o := range objs {
		pluralOf[upperFirst(o.singular)] = o.plural
	}
	out := make([]Object, 0, len(objs))
	for _, o := range objs {
		name := upperFirst(o.singular)
		create, okCreate := d.Components.Schemas[name]
		resp, okResp := d.Components.Schemas[name+"ForResponse"]
		if !okCreate || !okResp {
			return nil, fmt.Errorf("the workspace's OpenAPI document has no %s or %sForResponse schema for %s", name, name, o.plural)
		}
		required := map[string]bool{}
		for _, r := range create.Required {
			required[r] = true
		}
		obj := Object{Command: CommandName(o.plural), NamePlural: o.plural, NameSingular: o.singular, Description: tags[o.plural]}
		for fname, p := range resp.Properties {
			f := Field{Name: fname, Type: typeName(p.Type), Description: p.Description, Required: required[fname]}
			if rel := relationOf(p, pluralOf); rel != nil {
				f.Relation = rel
			} else {
				_, inCreate := create.Properties[fname]
				f.ReadOnly = !inCreate
				f.Format = p.Format
				f.Enum = enumOf(p)
				for sub := range p.Properties {
					f.Subfields = append(f.Subfields, sub)
				}
				sort.Strings(f.Subfields)
			}
			obj.Fields = append(obj.Fields, f)
		}
		sort.Slice(obj.Fields, func(i, j int) bool { return obj.Fields[i].Name < obj.Fields[j].Name })
		out = append(out, obj)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Command < out[j].Command })
	return out, nil
}

func operationID(item map[string]json.RawMessage, method string) string {
	var op struct {
		OperationID string `json:"operationId"`
	}
	if raw, ok := item[method]; ok {
		json.Unmarshal(raw, &op)
	}
	return op.OperationID
}

// relationOf recognises Twenty's two relation shapes: an object whose oneOf
// references <Target>ForResponse (many to one), and an array of them (one to
// many). A target outside the document keeps its singular name.
func relationOf(p schema, pluralOf map[string]string) *Relation {
	target := func(ref string) string {
		name := strings.TrimSuffix(strings.TrimPrefix(ref, "#/components/schemas/"), "ForResponse")
		if plural, ok := pluralOf[name]; ok {
			return plural
		}
		return lowerFirst(name)
	}
	if len(p.OneOf) > 0 && strings.HasSuffix(p.OneOf[0].Ref, "ForResponse") {
		return &Relation{Target: target(p.OneOf[0].Ref), Kind: "many_to_one"}
	}
	if p.Items != nil && strings.HasSuffix(p.Items.Ref, "ForResponse") {
		return &Relation{Target: target(p.Items.Ref), Kind: "one_to_many"}
	}
	return nil
}

func enumOf(p schema) []string {
	values := p.Enum
	if len(values) == 0 && p.Items != nil {
		values = p.Items.Enum
	}
	var out []string
	for _, v := range values {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func typeName(t any) string {
	switch v := t.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, x := range v {
			if s, ok := x.(string); ok && s != "null" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ",")
	}
	return ""
}

func upperFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[n:]
}

func lowerFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToLower(r)) + s[n:]
}
