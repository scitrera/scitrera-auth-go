// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Scitrera LLC.
package external

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/scitrera/aether/server/pkg/authproxy/login"
)

// handleLanding serves "/" (and the "/login" alias):
//
//   - if the request already carries a valid session, 302 to the post-login
//     destination (the configured absolute target, or an allowlisted return
//     URL captured earlier);
//   - otherwise render the provider-selection landing page, stashing any
//     allowlisted ?rd=/?next= return URL in a short-lived cookie so it can be
//     honoured once the OAuth round-trip returns the browser to "/".
func (s *Server) handleLanding(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	if s.isAuthenticated(r) {
		dest := s.opts.TargetURL
		if rc := s.readReturnCookie(r); rc != "" {
			if d, ok := s.sanitizeRedirect(rc); ok {
				dest = d
			}
		}
		s.clearReturnCookie(w)
		http.Redirect(w, r, dest, http.StatusFound)
		return
	}

	// Capture an allowlisted return URL so it survives the OAuth round-trip.
	rd := firstNonEmpty(r.URL.Query().Get("rd"), r.URL.Query().Get("next"))
	if dest, ok := s.sanitizeRedirect(rd); ok {
		s.setReturnCookie(w, dest)
	}

	s.renderLanding(w)
}

// isAuthenticated reports whether the request carries a live session.
func (s *Server) isAuthenticated(r *http.Request) bool {
	id := login.ReadSession(r, s.cookies)
	if id == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	data, err := s.store.Get(ctx, id)
	if err != nil {
		log.Printf("external: session lookup error: %v", err)
		return false
	}
	return data != nil
}

// sanitizeRedirect validates an inbound return URL. Same-origin relative paths
// are always allowed (but not protocol-relative "//host" or "/\host"). Absolute
// URLs are allowed only when http(s) and their host is in AllowedRedirectHosts.
// Returns ("", false) for an empty or disallowed value.
func (s *Server) sanitizeRedirect(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	if strings.HasPrefix(raw, "/") {
		if strings.HasPrefix(raw, "//") || strings.HasPrefix(raw, "/\\") {
			return "", false
		}
		return raw, true
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", false
	}
	for _, h := range s.opts.AllowedRedirectHosts {
		if strings.ToLower(strings.TrimSpace(h)) == host {
			return raw, true
		}
	}
	return "", false
}

// setReturnCookie writes the short-lived return-URL cookie. It mirrors the
// session cookie's Secure/SameSite but is deliberately host-only (no Domain) so
// it binds to this front-door host alone — combined with the HMAC signature
// (when a key is configured), a sibling host under the session domain cannot
// forge or plant a return target. The session cookie's domain is unchanged.
func (s *Server) setReturnCookie(w http.ResponseWriter, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     returnCookieName,
		Value:    s.signReturnValue(value),
		Path:     "/",
		MaxAge:   int((10 * time.Minute).Seconds()),
		Secure:   s.cookies.Secure,
		HttpOnly: true,
		SameSite: s.cookieSameSite(),
	})
}

// clearReturnCookie expires the return-URL cookie. It must match setReturnCookie's
// attributes (host-only, same Path) for the browser to overwrite it.
func (s *Server) clearReturnCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     returnCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   s.cookies.Secure,
		HttpOnly: true,
		SameSite: s.cookieSameSite(),
	})
}

// signReturnValue appends an HMAC-SHA256 tag to value as
// "value.base64url(mac)" when a key is configured; otherwise it returns value
// unchanged (unsigned graceful degradation).
func (s *Server) signReturnValue(value string) string {
	if len(s.opts.ReturnCookieHMACKey) == 0 {
		return value
	}
	mac := hmac.New(sha256.New, s.opts.ReturnCookieHMACKey)
	mac.Write([]byte(value))
	return value + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// verifyReturnValue validates the HMAC tag produced by signReturnValue and
// returns the bare value. When no key is configured it accepts the value as-is.
// On a missing or invalid signature it returns ("", false).
func (s *Server) verifyReturnValue(raw string) (string, bool) {
	if len(s.opts.ReturnCookieHMACKey) == 0 {
		return raw, true
	}
	i := strings.LastIndexByte(raw, '.')
	if i < 0 {
		return "", false
	}
	value, sig := raw[:i], raw[i+1:]
	wantMAC, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		return "", false
	}
	mac := hmac.New(sha256.New, s.opts.ReturnCookieHMACKey)
	mac.Write([]byte(value))
	if !hmac.Equal(mac.Sum(nil), wantMAC) {
		return "", false
	}
	return value, true
}

// cookieSameSite returns the configured SameSite mode, defaulting to Lax when
// unset (the OSS loader normally fills this in, but guard the zero value).
func (s *Server) cookieSameSite() http.SameSite {
	if s.cookies.SameSite == 0 {
		return http.SameSiteLaxMode
	}
	return s.cookies.SameSite
}

// readReturnCookie returns the verified return-URL cookie value, or "". The
// HMAC signature (when a key is configured) is checked here; an unsigned or
// tampered value is dropped. Callers re-check the value through sanitizeRedirect
// regardless, so an empty/invalid cookie simply falls back to the target.
func (s *Server) readReturnCookie(r *http.Request) string {
	c, err := r.Cookie(returnCookieName)
	if err != nil || c == nil {
		return ""
	}
	value, ok := s.verifyReturnValue(c.Value)
	if !ok {
		return ""
	}
	return value
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
