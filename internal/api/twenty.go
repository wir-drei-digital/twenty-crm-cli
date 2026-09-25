package api

import (
	"encoding/json"
	"fmt"
	"strings"
)

// failure is the error body Twenty's REST API sends,
// {"statusCode":400,"messages":["..."],"error":"BAD_REQUEST"}, or the NestJS
// default {"statusCode":404,"message":"...","error":"Not Found"}.
type failure struct {
	messages []string
	code     string
}

func parseFailure(body []byte) failure {
	var raw struct {
		Messages json.RawMessage `json:"messages"`
		Message  json.RawMessage `json:"message"`
		Error    json.RawMessage `json:"error"`
	}
	var f failure
	if json.Unmarshal(body, &raw) != nil {
		return f
	}
	f.messages = append(stringsOf(raw.Messages), stringsOf(raw.Message)...)
	if s := stringsOf(raw.Error); len(s) == 1 {
		f.code = s[0]
	}
	return f
}

// stringsOf reads a JSON string or an array of strings; anything else is nil.
func stringsOf(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var one string
	if json.Unmarshal(raw, &one) == nil {
		if one == "" {
			return nil
		}
		return []string{one}
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil {
		return many
	}
	return nil
}

// summary leads with Twenty's first message rather than the status text.
func (f failure) summary() string {
	switch {
	case len(f.messages) > 1:
		return fmt.Sprintf(": %s (and %d more)", f.messages[0], len(f.messages)-1)
	case len(f.messages) == 1:
		return ": " + f.messages[0]
	case f.code != "":
		return ": " + f.code
	}
	return ""
}

// hint names the likely fix for the errors people meet while setting up.
func hint(r Request, status int, f failure) string {
	text := strings.ToLower(strings.Join(f.messages, " "))
	switch {
	case status == 401 && strings.Contains(text, "expired"):
		return "; hint: the API key has expired: create a new one in Twenty under Settings, APIs & Webhooks, then run `twentycrm config set api-key`"
	case status == 401:
		return "; hint: the API key is invalid or revoked; `twentycrm auth status` shows which key is in use"
	case status == 403 && (strings.HasPrefix(r.Path, "rest/metadata/objects") || strings.HasPrefix(r.Path, "rest/metadata/fields")):
		return "; hint: objects and fields need the Data Model permission on the key's role; `twentycrm schema` works without it"
	case status == 403:
		return "; hint: the key's role does not allow this on this object (Twenty: Settings, Roles)"
	case status == 400 && r.Object != "" && strings.Contains(text, "field"):
		return fmt.Sprintf("; hint: run `twentycrm schema %s` for the field names", r.Object)
	case status == 429:
		return "; hint: Twenty allows 100 requests per minute"
	}
	return ""
}

func errorDetails(resp *Response) any {
	if len(resp.Body) == 0 {
		return nil
	}
	var v any
	if json.Unmarshal(resp.Body, &v) == nil {
		return v
	}
	return string(resp.Body)
}

// errorFor builds the error for a non-2xx response; extra goes between
// Twenty's message and the hint.
func errorFor(r Request, resp *Response, extra string) *Error {
	f := parseFailure(resp.Body)
	return &Error{
		Kind:    kindForStatus(resp.Status),
		Message: fmt.Sprintf("%s %s: HTTP %d%s%s%s", r.Method, r.Path, resp.Status, f.summary(), extra, hint(r, resp.Status, f)),
		Status:  resp.Status,
		Details: errorDetails(resp),
	}
}
