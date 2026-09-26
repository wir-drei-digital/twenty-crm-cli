package routes

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"unicode"
)

var rawMethods = map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}

// CleanRawPath validates a path for the api escape hatch: relative to the
// base URL, under rest/, no query string, and every segment plain, so no
// spelling of a path can dodge ClassifyRaw.
func CleanRawPath(p string) (string, error) {
	p = strings.TrimPrefix(p, "/")
	if strings.ContainsAny(p, "?#") {
		return "", errors.New("put query parameters in --query k=v, not in the path")
	}
	segs := strings.Split(p, "/")
	for _, s := range segs {
		if s == "" || s == "." || s == ".." || strings.ContainsAny(s, `%\`) || strings.IndexFunc(s, unicode.IsSpace) >= 0 {
			return "", fmt.Errorf("path segment %q is not allowed: segments must be non-empty, not . or .., and free of %%, whitespace and backslashes", s)
		}
	}
	if segs[0] != "rest" {
		if segs[0] == "graphql" || segs[0] == "metadata" {
			return "", errors.New("GraphQL is not supported in v1; use the REST paths under rest/")
		}
		return "", errors.New("only paths under rest/ are allowed, for example rest/companies")
	}
	return p, nil
}

// Raw is how the api escape hatch treats one request.
type Raw struct {
	Class          string
	Blocked        string
	FilterRequired bool
}

// ClassifyRaw gives a raw request its class from the route grammar alone,
// independent of the workspace model (spec: The api escape hatch). The fixed
// prefixes come first; every other path under rest/ is a record path and is
// classified the way Twenty 2.27 routes it.
func ClassifyRaw(method, path string, q url.Values) (Raw, error) {
	method = strings.ToUpper(method)
	if !rawMethods[method] {
		return Raw{}, fmt.Errorf("method %q is not allowed (GET, POST, PUT, PATCH, DELETE)", method)
	}
	p, err := CleanRawPath(path)
	if err != nil {
		return Raw{}, err
	}
	if err := checkQueryKeys(q); err != nil {
		return Raw{}, err
	}
	segs := strings.Split(p, "/")[1:]
	get := method == "GET"
	readOr := func(other Raw) (Raw, error) {
		if get {
			return Raw{Class: ClassRead}, nil
		}
		return other, nil
	}
	n := len(segs)
	switch {
	case n == 0:
		return readOr(Raw{Class: ClassAdmin})
	case segs[0] == "apiKeys" || (segs[0] == "metadata" && n > 1 && segs[1] == "apiKeys"):
		return readOr(Raw{Class: ClassAdmin, Blocked: APIKeysBlocked})
	case segs[0] == "webhooks" || segs[0] == "metadata" || segs[0] == "open-api":
		return readOr(Raw{Class: ClassAdmin})
	}
	// Twenty compares soft_delete with the string "true"; anything else,
	// including TRUE, deletes permanently.
	return classifyRecord(method, segs, q.Get("soft_delete") == "true"), nil
}

// checkQueryKeys refuses the query spellings that could hide a filter or a
// soft_delete from the classification: a repeated one, a key with brackets
// (which Express parses into an object that Twenty then ignores), and a
// differently cased soft_delete or filter (which Twenty does not read).
func checkQueryKeys(q url.Values) error {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if strings.ContainsAny(k, "[]") {
			return fmt.Errorf("--query key %q is not allowed: keys must not contain [ or ]", k)
		}
		for _, name := range []string{"soft_delete", "filter"} {
			if strings.EqualFold(k, name) && k != name {
				return fmt.Errorf("--query key %q is not allowed: Twenty reads only %s, spelled exactly so", k, name)
			}
		}
	}
	for _, k := range []string{"soft_delete", "filter"} {
		if len(q[k]) > 1 {
			return fmt.Errorf("--query %s may appear only once", k)
		}
	}
	return nil
}

// classifyRecord mirrors RestApiCoreController and RestApiCoreService of
// Twenty 2.27: the routes are tried in the controller's order, and a route
// whose handler parses the path answers 400 where parseCorePath does, which
// the CLI classes admin. A wildcard route segment (*path) matches one or
// more segments, so the batch, restore, duplicates and merge routes need at
// least two.
func classifyRecord(method string, segs []string, soft bool) Raw {
	n := len(segs)
	hasID, valid := parseCorePath(segs)
	// update is RestApiCoreService.update, and restore has the same shape:
	// one record with an ID, every record the filter matches without one.
	update := func() Raw {
		switch {
		case !valid:
			return Raw{Class: ClassAdmin}
		case hasID:
			return Raw{Class: ClassWrite}
		}
		return Raw{Class: ClassBulk, FilterRequired: true}
	}
	switch method {
	case "GET": // groupBy, find one, find many
		return Raw{Class: ClassRead}
	case "POST":
		switch {
		case n >= 2 && segs[0] == "batch": // create many
			return Raw{Class: ClassWrite}
		case n >= 2 && segs[n-1] == "duplicates": // find duplicates
			return Raw{Class: ClassRead}
		case !valid:
			return Raw{Class: ClassAdmin}
		}
		return Raw{Class: ClassWrite} // create one
	case "DELETE":
		switch {
		case !valid:
			return Raw{Class: ClassAdmin}
		case hasID && soft:
			return Raw{Class: ClassWrite}
		case hasID:
			return Raw{Class: ClassDestroy}
		case soft:
			return Raw{Class: ClassBulk, FilterRequired: true}
		}
		return Raw{Class: ClassDestroy, FilterRequired: true}
	case "PATCH":
		switch {
		case n >= 2 && segs[0] == "restore": // restore one or many
			return update()
		case n >= 2 && segs[n-1] == "merge": // merge many
			return Raw{Class: ClassBulk}
		}
		return update()
	case "PUT":
		return update()
	}
	return Raw{Class: ClassAdmin}
}

// parseCorePath mirrors Twenty 2.27's parseCorePath on the segments after
// rest/: hasID reports whether the path names one record, and valid is false
// where Twenty answers 400. More than two segments are always invalid, so
// rest/restore/<o>/<id> is too.
func parseCorePath(segs []string) (hasID, valid bool) {
	switch {
	case len(segs) == 0 || len(segs) > 2:
		return false, false
	case len(segs) == 1:
		return false, true
	case segs[0] == "batch":
		return false, true
	case segs[1] == "duplicates" || segs[1] == "groupBy" || segs[1] == "merge":
		return false, true
	case segs[0] == "restore":
		return false, true // the ID would be a third segment
	case ValidateID(segs[1]) != nil:
		return false, false
	}
	return true, true
}
