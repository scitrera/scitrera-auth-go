// SPDX-License-Identifier: AGPL-3.0-only
package loginproviders

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	jose "github.com/go-jose/go-jose/v4"
	"github.com/google/uuid"
	"github.com/scitrera/aether/server/pkg/authproxy/login"
	"golang.org/x/oauth2"
)

const microsoftOrigin = "https://login.microsoftonline.com"
const microsoftIssuerTemplate = microsoftOrigin + "/{tenantid}/v2.0"
const personalTenant = "9188040d-6c67-4c5b-b112-36a304b66dad"

// New supplies Microsoft tenant-independent OIDC discovery and verification.
// Other authorities retain Aether's strict standard OIDC behavior, including
// tenant-specific Microsoft issuers. Nothing here grants platform membership.
func New(ctx context.Context, cfg login.ProviderConfig) (*login.Provider, error) {
	issuer := strings.TrimSuffix(cfg.IssuerURL, "/")
	authority := ""
	switch issuer {
	case microsoftOrigin + "/organizations/v2.0":
		authority = "organizations"
	case microsoftOrigin + "/common/v2.0":
		authority = "common"
	default:
		return login.NewProvider(ctx, cfg)
	}
	if cfg.Name == "" || cfg.ClientID == "" || cfg.RedirectURL == "" {
		return nil, fmt.Errorf("Microsoft OIDC provider requires name, client ID and redirect URL")
	}
	client := microsoftClient(ctx)
	var metadata struct {
		Issuer        string `json:"issuer"`
		Authorization string `json:"authorization_endpoint"`
		Token         string `json:"token_endpoint"`
		Keys          string `json:"jwks_uri"`
	}
	if err := fetchJSON(ctx, client, issuer+"/.well-known/openid-configuration", &metadata); err != nil {
		return nil, fmt.Errorf("Microsoft OIDC discovery: %w", err)
	}
	// This exception is only for the two fixed Microsoft authorities above. Do
	// not make arbitrary templated issuers or metadata-selected hosts trusted.
	if metadata.Issuer != microsoftIssuerTemplate ||
		metadata.Authorization != microsoftOrigin+"/"+authority+"/oauth2/v2.0/authorize" ||
		metadata.Token != microsoftOrigin+"/"+authority+"/oauth2/v2.0/token" ||
		(metadata.Keys != microsoftOrigin+"/"+authority+"/discovery/v2.0/keys" && metadata.Keys != microsoftOrigin+"/common/discovery/v2.0/keys") {
		return nil, fmt.Errorf("Microsoft OIDC discovery returned unexpected issuer or endpoints")
	}
	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{"email", "profile"}
	}
	scopes = append([]string{oidc.ScopeOpenID}, scopes...)
	return &login.Provider{
		Config:   cfg,
		OAuth:    &oauth2.Config{ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, RedirectURL: cfg.RedirectURL, Scopes: scopes, Endpoint: oauth2.Endpoint{AuthURL: metadata.Authorization, TokenURL: metadata.Token}},
		Verifier: &microsoftVerifier{clientID: cfg.ClientID, organizationsOnly: authority == "organizations", keys: &microsoftKeys{client: client, url: metadata.Keys, now: time.Now}},
	}, nil
}

func microsoftClient(ctx context.Context) *http.Client {
	original, _ := ctx.Value(oauth2.HTTPClient).(*http.Client)
	if original == nil {
		original = http.DefaultClient
	}
	client := *original
	if client.Timeout == 0 || client.Timeout > 10*time.Second {
		client.Timeout = 10 * time.Second
	}
	// Metadata and keys must stay at the configured authority, including when a
	// network intermediary responds with a redirect. Do not follow other hosts.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &client
}

func fetchJSON(ctx context.Context, client *http.Client, address string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("metadata HTTP status %d", response.StatusCode)
	}
	const maxBytes = 1 << 20
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return err
	}
	if len(raw) > maxBytes {
		return fmt.Errorf("metadata exceeds size limit")
	}
	return json.Unmarshal(raw, out)
}

type microsoftVerifier struct {
	clientID          string
	organizationsOnly bool
	keys              *microsoftKeys
}

func (v *microsoftVerifier) Verify(ctx context.Context, raw string) (*oidc.IDToken, error) {
	// Read only enough untrusted input to derive an expected issuer at a fixed
	// host. No token claim selects a discovery/JWKS URL or creates a cache entry.
	if len(raw) > 64<<10 {
		return nil, fmt.Errorf("Microsoft ID token exceeds size limit")
	}
	parsed, err := jose.ParseSigned(raw, []jose.SignatureAlgorithm{jose.RS256})
	if err != nil {
		return nil, fmt.Errorf("invalid Microsoft ID token: %w", err)
	}
	var claims struct {
		Issuer string `json:"iss"`
		Tenant string `json:"tid"`
	}
	if err = json.Unmarshal(parsed.UnsafePayloadWithoutVerification(), &claims); err != nil {
		return nil, fmt.Errorf("invalid Microsoft token claims: %w", err)
	}
	id, err := uuid.Parse(claims.Tenant)
	if err != nil || id == uuid.Nil || len(claims.Tenant) != 36 || !strings.EqualFold(id.String(), claims.Tenant) {
		return nil, fmt.Errorf("Microsoft token requires a GUID tid")
	}
	if v.organizationsOnly && id.String() == personalTenant {
		return nil, fmt.Errorf("personal Microsoft accounts are not allowed by organizations authority")
	}
	expected := microsoftOrigin + "/" + claims.Tenant + "/v2.0"
	if claims.Issuer != expected {
		return nil, fmt.Errorf("Microsoft token issuer does not match tid")
	}
	// Standard verifier still checks exact issuer, audience, expiry and signature.
	// The key set additionally limits the signing key to this exact issuer.
	verifier := oidc.NewVerifier(expected, scopedMicrosoftKeys{keys: v.keys, issuer: expected}, &oidc.Config{ClientID: v.clientID, SupportedSigningAlgs: []string{oidc.RS256}})
	verified, err := verifier.Verify(ctx, raw)
	if err != nil {
		return nil, err
	}
	if verified.Subject == "" {
		return nil, fmt.Errorf("Microsoft ID token requires a subject")
	}
	return verified, nil
}
