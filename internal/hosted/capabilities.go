package hosted

import (
	"os"
	"strings"
)

// Capabilities describes operator-enabled features for this instance.
type Capabilities struct {
	WorkspaceID      string            `json:"workspace_id"`
	AuthMode         string            `json:"auth_mode"`
	MCPConfigured    bool              `json:"mcp_configured"`
	MCPEndpoint      string            `json:"mcp_endpoint,omitempty"`
	PublicBaseURL    string            `json:"public_base_url,omitempty"`
	Sending          map[string]bool   `json:"sending"`
	Integrations     map[string]bool   `json:"integrations"`
	FeatureFlags     map[string]string `json:"feature_flags,omitempty"`
	EncryptionReady  bool              `json:"encryption_ready"`
	GoogleOAuthReady bool              `json:"google_oauth_ready"`
	MicrosoftReady   bool              `json:"microsoft_oauth_ready"`
}

func envTruthy(key string, defaultTrue bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if v == "" {
		return defaultTrue
	}
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return defaultTrue
	}
}

func envPresent(keys ...string) bool {
	for _, k := range keys {
		if strings.TrimSpace(os.Getenv(k)) == "" {
			return false
		}
	}
	return true
}

// BuildCapabilities derives the settings catalog from environment + server opts.
func BuildCapabilities(workspaceID, publicBaseURL string, encryptionReady, googleReady bool) Capabilities {
	authMode := strings.TrimSpace(os.Getenv("AUTH_MODE"))
	if authMode == "" {
		authMode = "cloudflare_access"
	}
	microsoftReady := envPresent("MICROSOFT_CLIENT_ID", "MICROSOFT_CLIENT_SECRET")
	mcpToken := strings.TrimSpace(os.Getenv("MCP_BEARER_TOKEN")) != ""
	base := strings.TrimRight(publicBaseURL, "/")

	gmailOn := envTruthy("FEATURE_GMAIL", googleReady || strings.TrimSpace(os.Getenv("OPENOUTREACH_MOCK_GMAIL")) == "1")
	smtpOn := envTruthy("FEATURE_SMTP_IMAP", true)
	msOn := envTruthy("FEATURE_MICROSOFT", microsoftReady)
	resendOn := envTruthy("FEATURE_RESEND", false)
	sesOn := envTruthy("FEATURE_SES", false)
	cfEmailOn := envTruthy("FEATURE_CF_EMAIL", false)

	apolloOn := envTruthy("FEATURE_APOLLO", true)
	clayOn := envTruthy("FEATURE_CLAY", true)
	webhookOn := envTruthy("FEATURE_WEBHOOK", true)
	sheetsOn := envTruthy("FEATURE_SHEETS", true)

	c := Capabilities{
		WorkspaceID:   workspaceID,
		AuthMode:      authMode,
		MCPConfigured: mcpToken,
		PublicBaseURL: base,
		Sending: map[string]bool{
			"gmail":     gmailOn,
			"smtp_imap": smtpOn,
			"microsoft": msOn,
			"resend":    resendOn,
			"ses":       sesOn,
			"cf_email":  cfEmailOn,
		},
		Integrations: map[string]bool{
			"apollo":  apolloOn,
			"clay":    clayOn,
			"webhook": webhookOn,
			"sheets":  sheetsOn,
			"hunter":  envTruthy("FEATURE_HUNTER", false),
			"warmup":  envTruthy("FEATURE_WARMUP", false),
		},
		EncryptionReady:  encryptionReady,
		GoogleOAuthReady: googleReady,
		MicrosoftReady:   microsoftReady && msOn,
	}
	if base != "" {
		c.MCPEndpoint = base + "/mcp"
	}
	c.FeatureFlags = map[string]string{
		"FEATURE_GMAIL":     boolStr(gmailOn),
		"FEATURE_SMTP_IMAP": boolStr(smtpOn),
		"FEATURE_MICROSOFT": boolStr(msOn),
		"FEATURE_APOLLO":    boolStr(apolloOn),
		"FEATURE_CLAY":      boolStr(clayOn),
		"FEATURE_WEBHOOK":   boolStr(webhookOn),
		"FEATURE_SHEETS":    boolStr(sheetsOn),
		"FEATURE_RESEND":    boolStr(resendOn),
		"FEATURE_SES":       boolStr(sesOn),
		"FEATURE_CF_EMAIL":  boolStr(cfEmailOn),
		"FEATURE_WARMUP":    boolStr(envTruthy("FEATURE_WARMUP", false)),
	}
	return c
}

func boolStr(v bool) string {
	if v {
		return "1"
	}
	return "0"
}
