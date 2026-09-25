package routes

import (
	"errors"
	"fmt"
	"net/url"
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
// independent of the workspace model (spec: The api escape hatch). Rows are
// tried in the spec's order; the first match wins.
func ClassifyRaw(method, path string, q url.Values) (Raw, error) {
	method = strings.ToUpper(method)
	if !rawMethods[method] {
		return Raw{}, fmt.Errorf("method %q is not allowed (GET, POST, PUT, PATCH, DELETE)", method)
	}
	p, err := CleanRawPath(path)
	if err != nil {
		return Raw{}, err
	}
	for _, k := range []string{"soft_delete", "filter"} {
		if len(q[k]) > 1 {
			return Raw{}, fmt.Errorf("--query %s may appear only once", k)
		}
	}
	segs := strings.Split(p, "/")[1:]
	get := method == "GET"
	// Twenty compares soft_delete with the string "true"; anything else,
	// including TRUE, deletes permanently.
	soft := q.Get("soft_delete") == "true"
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
	case segs[0] == "batch" && n == 2 && method == "POST":
		return Raw{Class: ClassWrite}, nil
	case segs[0] == "restore" && n == 3 && method == "PATCH":
		return Raw{Class: ClassWrite}, nil
	case segs[0] == "restore" && n == 2 && method == "PATCH":
		return Raw{Class: ClassBulk, FilterRequired: true}, nil
	case n == 2 && segs[1] == "duplicates" && method == "POST":
		return Raw{Class: ClassRead}, nil
	case n == 2 && segs[1] == "merge" && method == "PATCH":
		return Raw{Class: ClassBulk}, nil
	case n == 2 && segs[1] == "groupBy" && get:
		return Raw{Class: ClassRead}, nil
	case n == 2:
		switch method {
		case "GET":
			return Raw{Class: ClassRead}, nil
		case "PATCH", "PUT":
			return Raw{Class: ClassWrite}, nil
		case "DELETE":
			if soft {
				return Raw{Class: ClassWrite}, nil
			}
			return Raw{Class: ClassDestroy}, nil
		}
	case n == 1:
		switch method {
		case "GET":
			return Raw{Class: ClassRead}, nil
		case "POST":
			return Raw{Class: ClassWrite}, nil
		case "PATCH", "PUT":
			return Raw{Class: ClassBulk, FilterRequired: true}, nil
		case "DELETE":
			if soft {
				return Raw{Class: ClassBulk, FilterRequired: true}, nil
			}
			return Raw{Class: ClassDestroy, FilterRequired: true}, nil
		}
	}
	return readOr(Raw{Class: ClassAdmin})
}
