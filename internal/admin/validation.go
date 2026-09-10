// SPDX-License-Identifier: AGPL-3.0-only
package admin

import (
	"encoding/json"
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"golang.org/x/net/idna"
)

var labelRE = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var providerRE = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
var claimRE = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_.:/-]{0,127}$`)

type problem struct {
	Status        int
	Code, Message string
}

func (e *problem) Error() string   { return e.Message }
func invalid(message string) error { return &problem{400, "invalid_request", message} }
func missing() error               { return &problem{404, "not_found", "Resource not found"} }
func textValue(s string, max int) bool {
	return strings.TrimSpace(s) != "" && len(s) <= max && !strings.ContainsFunc(s, unicode.IsControl)
}
func slug(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if !labelRE.MatchString(s) {
		return "", invalid("Tenant slug must be a DNS label (1–63 lowercase letters, digits or hyphens)")
	}
	return s, nil
}
func domain(s string) (string, error) {
	s = strings.TrimSuffix(strings.TrimSpace(strings.ToLower(s)), ".")
	a, err := idna.Lookup.ToASCII(s)
	if err != nil || len(a) > 253 || !strings.Contains(a, ".") {
		return "", invalid("Enter a domain name, without a scheme, path, port or wildcard")
	}
	for _, part := range strings.Split(a, ".") {
		if !labelRE.MatchString(part) {
			return "", invalid("Invalid domain name")
		}
	}
	return a, nil
}
func email(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	a, err := mail.ParseAddress(s)
	if err != nil || a.Address != s || len(s) > 254 || strings.ContainsAny(s, "\r\n\t ") {
		return "", invalid("Enter a valid email address")
	}
	at := strings.LastIndexByte(s, '@')
	d, err := domain(s[at+1:])
	if err != nil {
		return "", err
	}
	return s[:at+1] + d, nil
}
func userID(s string) error {
	if _, err := uuid.Parse(s); err != nil {
		return invalid("Invalid user ID")
	}
	return nil
}

type TenantInput struct {
	Slug     string                     `json:"slug"`
	Name     string                     `json:"name"`
	Enabled  *bool                      `json:"enabled"`
	Metadata map[string]json.RawMessage `json:"metadata"`
}

func (t *TenantInput) validate(create bool) error {
	if create {
		s, err := slug(t.Slug)
		if err != nil {
			return err
		}
		t.Slug = s
	} else if t.Slug != "" {
		return invalid("Tenant slugs are immutable")
	}
	t.Name = strings.TrimSpace(t.Name)
	if !textValue(t.Name, 200) || t.Enabled == nil {
		return invalid("Name (1–200 characters) and enabled are required")
	}
	// Only presentation metadata is editable here. Unknown existing keys survive.
	for key, raw := range t.Metadata {
		if key != "logo" && key != "default_workspace" {
			return invalid("Only logo and default_workspace metadata are editable")
		}
		if string(raw) == "null" {
			continue
		}
		var val string
		if json.Unmarshal(raw, &val) != nil || len(val) > 2048 || strings.ContainsFunc(val, unicode.IsControl) {
			return invalid("Metadata values must be strings or null")
		}
		if key == "logo" && val != "" {
			u, err := url.Parse(val)
			if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
				return invalid("Logo must be an HTTPS URL or empty")
			}
		}
	}
	return nil
}

type UserInput struct {
	Email         string  `json:"email"`
	Name          string  `json:"name"`
	Enabled       *bool   `json:"enabled"`
	DefaultTenant *string `json:"default_tenant_slug"`
}

func (u *UserInput) validate() error {
	e, err := email(u.Email)
	if err != nil {
		return err
	}
	u.Email = e
	u.Name = strings.TrimSpace(u.Name)
	if !textValue(u.Name, 200) || u.Enabled == nil {
		return invalid("Name (1–200 characters) and enabled are required")
	}
	if u.DefaultTenant != nil && *u.DefaultTenant != "" {
		s, err := slug(*u.DefaultTenant)
		if err != nil {
			return err
		}
		u.DefaultTenant = &s
	}
	return nil
}

type PolicyInput struct {
	AutoAdd   *bool                     `json:"auto_add"`
	Providers *[]string                 `json:"providers"`
	Checks    map[string]map[string]any `json:"checks"`
}

func (p *PolicyInput) validate() error {
	if p.Providers != nil {
		if len(*p.Providers) > 64 {
			return invalid("Too many providers")
		}
		seen := map[string]bool{}
		for _, v := range *p.Providers {
			if !providerRE.MatchString(v) || seen[v] {
				return invalid("Invalid or duplicate provider name")
			}
			seen[v] = true
		}
	}
	if len(p.Checks) > 64 {
		return invalid("Too many provider checks")
	}
	for provider, checks := range p.Checks {
		if !providerRE.MatchString(provider) || len(checks) > 64 {
			return invalid("Invalid provider checks")
		}
		for key, value := range checks {
			if !claimRE.MatchString(key) {
				return invalid("Invalid claim name")
			}
			if provider == "google" && key == "hd" {
				// Only this claim has a blank option: an existing member whose
				// Google account has no hosted domain, never domain-only admission.
				normalize := func(s string) (string, error) {
					if s == "" {
						return "", nil
					}
					return domain(s)
				}
				switch v := value.(type) {
				case string:
					normalized, err := normalize(v)
					if err != nil {
						return err
					}
					checks[key] = normalized
				case []any:
					if len(v) > 128 {
						return invalid("Too many allowed hosted domains")
					}
					for i, option := range v {
						s, ok := option.(string)
						if !ok {
							return invalid("Google hosted domains must be strings")
						}
						normalized, err := normalize(s)
						if err != nil {
							return err
						}
						v[i] = normalized
					}
				default:
					return invalid("Google hd must be a domain string or list; an empty string allows registered members without hd")
				}
				continue
			}
			switch v := value.(type) {
			case string:
				if !textValue(v, 2048) {
					return invalid("Claim values must be nonempty strings")
				}
			case []any:
				if len(v) > 128 {
					return invalid("Too many allowed claim values")
				}
				for _, a := range v {
					s, ok := a.(string)
					if !ok || !textValue(s, 2048) {
						return invalid("Claim lists must contain nonempty strings")
					}
				}
			default:
				return invalid(fmt.Sprintf("Claim %s must be a string or list of strings", key))
			}
		}
	}
	return nil
}
