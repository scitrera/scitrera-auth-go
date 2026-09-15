// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Scitrera LLC.
package external

import (
	"context"
	_ "embed"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

//go:embed select.html.tmpl
var landingTmplRaw string

var landingTmpl = template.Must(template.New("landing").Parse(landingTmplRaw))

var tenantHintRE = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

const (
	defaultBrandName    = "scitrera.ai"
	defaultBrandLogoURL = "https://scitrera.com/logo2.png"
	defaultBrandTagline = "Choose your organization's sign-in method."
)

// Branding customises the landing page chrome. Zero values fall back to the
// scitrera defaults (see BrandingFromEnv).
type Branding struct {
	ProductName string
	LogoURL     string
	Tagline     string
}

// BrandingFromEnv reads SCITRERA_AUTH_BRAND_* overrides, defaulting to the
// scitrera.ai branding used by the legacy Python login page.
func BrandingFromEnv() Branding {
	return Branding{
		ProductName: getenv("SCITRERA_AUTH_BRAND_NAME", defaultBrandName),
		LogoURL:     getenv("SCITRERA_AUTH_BRAND_LOGO_URL", defaultBrandLogoURL),
		Tagline:     getenv("SCITRERA_AUTH_BRAND_TAGLINE", defaultBrandTagline),
	}
}

// providerButton is the view model for one provider's sign-in button.
type providerButton struct {
	Name     string
	Label    string
	Class    string
	IconSVG  template.HTML
	LoginURL string
}

type landingView struct {
	Branding  Branding
	Providers []providerButton
}

// buildProviderButtons maps configured provider names to display metadata.
// Names are sorted for a stable button order. Known providers get branded
// labels/colours/icons; unknown providers fall back to a generic button.
func buildProviderButtons(names []string) []providerButton {
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)

	buttons := make([]providerButton, 0, len(sorted))
	for _, name := range sorted {
		b := providerButton{
			Name:     name,
			LoginURL: "/auth/login/" + name,
		}
		switch strings.ToLower(name) {
		case "google":
			b.Label = "Sign in with Google"
			b.Class = "google-btn"
			b.IconSVG = googleIcon
		case "azure", "microsoft", "entra", "azuread", "azure-ad":
			b.Label = "Sign in with Microsoft"
			b.Class = "microsoft-btn"
			b.IconSVG = microsoftIcon
		default:
			b.Label = "Sign in with " + titleCase(name)
			b.Class = "generic-btn"
		}
		buttons = append(buttons, b)
	}
	return buttons
}

// renderLanding writes the provider-selection page.
func (s *Server) renderLanding(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self' https: data:; style-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'")
	view := landingView{
		Branding:  s.brandingForRequest(r),
		Providers: s.providers,
	}
	if err := landingTmpl.Execute(w, view); err != nil {
		log.Printf("external: landing render error: %v", err)
	}
}

// brandingForRequest treats tenant as a presentation hint only. It never
// selects an admission tenant, changes providers, or affects the return URL.
func (s *Server) brandingForRequest(r *http.Request) Branding {
	b := s.brandingWithDefaults()
	if s.repo == nil {
		return b
	}
	hint := s.opts.DefaultTenant
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return b
	}
	if hints, present := query["tenant"]; present {
		if len(hints) != 1 {
			return b
		}
		hint = hints[0]
	}
	slug := strings.ToLower(strings.TrimSpace(hint))
	if !tenantHintRE.MatchString(slug) {
		return b
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	tenant, err := s.repo.GetTenantBySlug(ctx, slug)
	if err != nil {
		log.Printf("external: login branding lookup error: %v", err)
		return b
	}
	if tenant == nil || !tenant.Enabled {
		return b
	}
	if name := strings.TrimSpace(tenant.Name); name != "" {
		b.ProductName = name
	}
	// The admin API accepts HTTPS logo URLs. Recheck legacy/direct database
	// values here before putting them on an unauthenticated page.
	logo := strings.TrimSpace(tenant.Logo)
	if u, err := url.Parse(logo); err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil {
		b.LogoURL = logo
	}
	return b
}

// brandingWithDefaults fills any empty branding fields with the scitrera
// defaults (so a partially-populated Options.Branding still renders cleanly).
func (s *Server) brandingWithDefaults() Branding {
	b := s.opts.Branding
	if b.ProductName == "" {
		b.ProductName = defaultBrandName
	}
	if b.LogoURL == "" {
		b.LogoURL = defaultBrandLogoURL
	}
	if b.Tagline == "" {
		b.Tagline = defaultBrandTagline
	}
	return b
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// titleCase upper-cases the first rune of s, leaving the rest unchanged. Used
// for the generic provider button label (e.g. "okta" -> "Okta").
func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// Provider icons, reused from the legacy Python select.html. They are coloured
// white to sit on the branded button backgrounds.
const (
	googleIcon template.HTML = `<svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
            <path d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z" fill="#FFFFFF"></path>
            <path d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.3v2.84C4.02 20.44 7.7 23 12 23z" fill="#FFFFFF"></path>
            <path d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.3C1.42 8.84 1 10.4 1 12s.42 3.16 1.2 4.93l3.64-2.84z" fill="#FFFFFF"></path>
            <path d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 4.02 3.56 2.3 6.84l3.54 2.84c.87-2.6 3.3-4.53 6.16-4.53z" fill="#FFFFFF"></path>
        </svg>`
	microsoftIcon template.HTML = `<svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
            <path d="M11.25 2H2v9.25h9.25V2zm0 10.75H2v9.25h9.25V12.75zm1.5-10.75h9.25v9.25h-9.25V2zm0 10.75h9.25v9.25h-9.25V12.75z" fill="#FFFFFF"></path>
        </svg>`
)
