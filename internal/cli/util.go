package cli

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/api"
)

func flagBool(cmd *cobra.Command, name string) bool {
	v, _ := cmd.Flags().GetBool(name)
	return v
}

func flagString(cmd *cobra.Command, name string) string {
	v, _ := cmd.Flags().GetString(name)
	return v
}

// groupRunE answers a namespace invoked without a subcommand: that is a
// usage error, not a success that prints help.
func groupRunE(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return api.Usagef("unknown command %q for %q", args[0], cmd.CommandPath())
	}
	var names []string
	for _, sub := range cmd.Commands() {
		if !sub.Hidden && sub.Name() != "help" && sub.Name() != "completion" {
			names = append(names, sub.Name())
		}
	}
	if len(names) > 12 {
		names = append(names[:12], "...")
	}
	return api.Usagef("%s needs a subcommand: %s", cmd.CommandPath(), strings.Join(names, ", "))
}

// jsonCompact encodes v without HTML escaping and without a trailing newline.
func jsonCompact(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// setJSONField sets one top-level field of a JSON object body and keeps every
// other field's bytes as they were.
func setJSONField(body []byte, key string, value any) ([]byte, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, api.Usagef("--data must be a JSON object: %v", err)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, api.Usagef("%v", err)
	}
	m[key] = raw
	return jsonCompact(m)
}

func cloneValues(v url.Values) url.Values {
	out := url.Values{}
	for k, vs := range v {
		out[k] = append([]string(nil), vs...)
	}
	return out
}
