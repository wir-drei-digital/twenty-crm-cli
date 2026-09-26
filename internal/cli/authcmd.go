package cli

import (
	"math"
	"time"

	"github.com/spf13/cobra"

	"github.com/wir-drei-digital/twenty-crm-cli/internal/auth"
)

// authStatus is `twentycrm auth status`: what is configured, never the key.
type authStatus struct {
	Mode          string   `json:"mode"` // api_key or none
	Source        string   `json:"source,omitempty"`
	BaseURL       string   `json:"base_url,omitempty"`
	BaseURLSource string   `json:"base_url_source,omitempty"`
	WorkspaceID   string   `json:"workspace_id,omitempty"`
	KeyID         string   `json:"key_id,omitempty"`
	ExpiresAt     string   `json:"expires_at,omitempty"`
	ExpiresInDays *int     `json:"expires_in_days,omitempty"`
	ExpiresSoon   bool     `json:"expires_soon"`
	Expired       bool     `json:"expired"`
	ReadOnly      bool     `json:"read_only"`
	KeyError      string   `json:"key_error,omitempty"`
	Missing       []string `json:"missing,omitempty"`
	Hint          string   `json:"hint,omitempty"`
}

const expirySoon = 14 * 24 * time.Hour

func (a *app) authCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "auth", Short: "Inspect the configured API key", RunE: groupRunE}
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show the configured key as one line of JSON: workspace, key ID, expiry (offline; never prints the key)",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, args []string) error { return a.emit(cmd, a.authStatus()) },
	})
	return cmd
}

func (a *app) authStatus() authStatus {
	r := a.res
	s := authStatus{Mode: "none", BaseURL: r.BaseURL, BaseURLSource: r.BaseURLSource, ReadOnly: r.ReadOnly, Missing: r.Missing()}
	if r.APIKey != "" {
		s.Mode, s.Source = "api_key", r.KeySource
		if k, err := auth.ParseAPIKey(r.APIKey); err != nil {
			s.KeyError = err.Error()
		} else {
			s.WorkspaceID, s.KeyID = k.WorkspaceID, k.KeyID
			if !k.ExpiresAt.IsZero() {
				now := a.clock()
				left := k.ExpiresAt.Sub(now)
				days := int(math.Floor(left.Hours() / 24))
				s.ExpiresAt, s.ExpiresInDays = k.ExpiresAt.Format(time.RFC3339), &days
				s.Expired = k.Expired(now)
				s.ExpiresSoon = !s.Expired && left < expirySoon
			}
		}
	}
	switch {
	case len(s.Missing) > 0:
		s.Hint = "run `twentycrm init`, or set TWENTY_BASE_URL and TWENTY_API_KEY"
	case r.BindingError() != nil:
		s.Hint = r.BindingError().Error()
	case s.KeyError != "":
		s.Hint = "the key cannot be read; create an API key in Twenty under Settings, APIs & Webhooks"
	case s.Expired:
		s.Hint = "the key has expired: create a new one in Twenty under Settings, APIs & Webhooks, then run `twentycrm config set api-key`"
	case s.ExpiresSoon:
		s.Hint = "the key expires soon: create a new one in Twenty under Settings, APIs & Webhooks before " + s.ExpiresAt[:10]
	}
	return s
}
