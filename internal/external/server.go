// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Scitrera LLC.
// Package external implements the auth-proxy's public-facing front door: a
// second HTTP listener, distinct from the cluster-internal forward-auth
// (ext_auth) plane served by github.com/scitrera/aether/pkg/authproxy.Run.
//
// The internal plane answers /auth/verify + /healthz for envoy/nginx and must
// never be exposed to the load balancer. This external plane is the only
// surface the LB routes to: a provider-selection landing page at "/", the
// browser OAuth login/callback/logout endpoints, and the session-status
// endpoint the frontend polls.
//
// It is built entirely on the OSS pkg/authproxy *public* API — auth-go is a
// separate Go module and cannot import aether's internal/ packages. The OSS
// login handlers, session store, and provider registry are reused verbatim;
// this package only adds the landing page, the absolute post-login redirect,
// and the route/port separation. Because the session store is the same Redis
// (addr + key prefix) and the cookie config is identical, a session minted on
// this external plane is validated by the internal plane's /auth/verify with
// no shared in-process state.
package external

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	pkgauthproxy "github.com/scitrera/aether/server/pkg/authproxy"
	"github.com/scitrera/aether/server/pkg/authproxy/login"

	"github.com/scitrera/scitrera-auth-go/internal/observability"
)

// returnCookieName is the short-lived cookie used to carry an allowlisted
// post-login return URL across the OAuth round-trip. The OSS callback only
// honours same-origin relative "next" values, so an absolute return target
// is stashed here on the landing render and consumed when the browser comes
// back to "/".
const returnCookieName = "scitrera_post_login_rd"

// Options configures the external front-door server.
type Options struct {
	Source http.Handler // Corresponding source archive for this build, when bundled.
	// ListenAddr is the TCP address for the external listener (e.g. ":8081").
	ListenAddr string
	// TargetURL is the absolute URL an already-authenticated browser is sent
	// to from "/" (e.g. "https://app.example.net"). Required.
	TargetURL string
	// AllowedRedirectHosts is the set of hostnames an inbound ?rd=/?next=
	// return URL may target. Same-host relative paths are always allowed.
	AllowedRedirectHosts []string
	// AllowedRedirectOrigins, when configured, replaces the host-only allowlist.
	// It matches scheme and host:port, preventing downgrade or port changes.
	AllowedRedirectOrigins []string
	// ReturnCookieHMACKey, when non-empty, is the HMAC-SHA256 key used to sign
	// the short-lived post-login return cookie so its value cannot be forged by
	// anything that can merely set a cookie on the parent domain. Sourced from
	// AUTH_PROXY_TOKEN_HMAC_KEY. When empty, the cookie is written unsigned
	// (graceful degradation — sanitizeRedirect is still re-checked on read).
	ReturnCookieHMACKey []byte
	// Branding customises the landing page.
	Branding Branding
	// DefaultTenant is an optional tenant slug used only for login branding
	// when the request has no tenant query parameter. Empty means no hint.
	DefaultTenant string
	// Repo, when set, supplies public login branding for ?tenant=<slug> and lets
	// /checkz enrich the session-status response with the user's tenant list.
	// The concrete *mtdb.Repo from the internal plane is passed through here.
	// When nil, or when a lookup errors, /checkz gracefully degrades to the basic
	// {auth,user_id,email} response.
	// Login branding falls back to Branding, with Scitrera defaults.
	Repo tenantLookup
	// CheckzAllowedOrigins is the server-side Origin allowlist enforced on the
	// session-status endpoints (/checkz, /auth/checkz) BEFORE any session/PII
	// processing — defense-in-depth on top of the edge Envoy CORS policy. Each
	// entry is a full origin (scheme+host, e.g. "https://app.example.net"), NOT
	// a bare host, because browsers send the Origin header as scheme+host and we
	// compare with an exact, case-sensitive match.
	//
	// A request whose Origin header is present but not in this list is rejected
	// with 403 before lookupSession runs. A request with NO Origin header is
	// allowed (same-origin / top-level navigation / non-browser callers; the
	// session cookie is still required and a cross-origin browser fetch always
	// carries an Origin). When this list is EMPTY the check is disabled entirely
	// (fail-open on config absence — the edge CORS policy still applies), so
	// deployments that have not set SCITRERA_AUTH_CHECKZ_ALLOWED_ORIGINS are not
	// broken. Sourced from SCITRERA_AUTH_CHECKZ_ALLOWED_ORIGINS.
	CheckzAllowedOrigins []string
}

// Server is the external front-door HTTP server. It owns its own session
// store handle (shared with the internal plane via Redis) so the landing
// handler can check auth without a subrequest.
type Server struct {
	httpServer *http.Server
	store      login.SessionStore
	redis      interface{ Close() error } // *redis.Client or nil (jwt store)
	cookies    login.CookieConfig
	providers  []providerButton
	repo       tenantLookup
	opts       Options
}

// New builds the external front-door server from the AUTH_PROXY_LOGIN_* /
// AUTH_PROXY_SESSION_* environment (the same env that configures the internal
// plane's login subsystem). It performs OIDC discovery for each provider and
// probes the session store, so a returned error means the server cannot serve
// login traffic.
func New(opts Options) (*Server, error) {
	if opts.ListenAddr == "" {
		return nil, fmt.Errorf("external: ListenAddr is required")
	}
	if opts.TargetURL == "" {
		return nil, fmt.Errorf("external: TargetURL (SCITRERA_AUTH_POST_LOGIN_TARGET) is required")
	}

	loginCfg, err := pkgauthproxy.LoadLoginConfigFromEnv()
	if err != nil {
		return nil, fmt.Errorf("external: load login config: %w", err)
	}
	if !loginCfg.Enabled {
		return nil, fmt.Errorf("external: no login providers configured (set AUTH_PROXY_LOGIN_PROVIDERS)")
	}

	store, redisClient, err := loginCfg.BuildSessionStore()
	if err != nil {
		return nil, fmt.Errorf("external: build session store: %w", err)
	}

	// Close the session store's Redis client on any early return below; cleared
	// once the server is fully built and ownership transfers to the Server.
	ok := false
	defer func() {
		if !ok && redisClient != nil {
			_ = redisClient.Close()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	reg, err := loginCfg.BuildRegistry(ctx)
	if err != nil {
		return nil, fmt.Errorf("external: build provider registry: %w", err)
	}

	handlers, err := login.NewHandlers(login.Options{
		Registry: reg,
		Store:    store,
		Cookies:  loginCfg.Cookies,
	})
	if err != nil {
		return nil, fmt.Errorf("external: build login handlers: %w", err)
	}

	s := &Server{
		store:     store,
		cookies:   loginCfg.Cookies,
		providers: buildProviderButtons(reg.Names()),
		repo:      opts.Repo,
		opts:      opts,
	}
	if redisClient != nil {
		s.redis = redisClient
	}

	// OSS login handlers live on a private sub-mux; the outer mux forwards
	// only the browser-facing prefixes so the external surface stays minimal.
	loginMux := http.NewServeMux()
	handlers.Mount(loginMux)

	s.httpServer = &http.Server{
		Addr:              opts.ListenAddr,
		Handler:           otelHandler(withRecover(s.routes(loginMux))),
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	ok = true
	return s, nil
}

// routes builds the outer mux. loginMux is the OSS login sub-mux (already
// carrying /auth/login/, /auth/callback/, /auth/logout, /auth/me,
// /auth/checkz). We expose only the subset the browser needs; everything else
// under /auth/ (notably /auth/me and /auth/verify) returns 404 here.
//
// A few routes are wrapped before forwarding to the OSS handlers: /auth/logout
// is restricted to POST (the OSS mux accepts GET, which a top-level SameSite=Lax
// GET could trigger as a CSRF logout); /auth/login/ has its ?next= re-validated
// against sanitizeRedirect (the OSS login handler only checks a "/" prefix, so a
// protocol-relative "//evil.com" would survive as a post-login open redirect);
// and the session-status endpoints (/auth/checkz, /checkz) carry Cache-Control:
// no-store because their bodies include the session email / user_id.
func (s *Server) routes(loginMux http.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	if s.opts.Source != nil {
		mux.Handle("/source.tar.gz", s.opts.Source)
	}

	// Browser OAuth surface. Logout is wrapped after the OSS handler clears the
	// session so browsers land back on the configured default target.
	mux.Handle("/auth/login/", s.sanitizeLoginNext(loginMux))
	mux.Handle("/auth/callback/", loginMux)
	mux.Handle("/auth/logout", methodGuard(http.MethodPost, s.redirectLogout(loginMux)))

	// Session-status: served by our own handler (not the OSS one) so the
	// response carries the user's tenant list for the SPA. The handler sets
	// Cache-Control: no-store itself (its body includes the session email /
	// user_id), so no noStore wrapper is needed here.
	mux.HandleFunc("/auth/checkz", s.handleCheckz)

	// Any other /auth/* path is deliberately not part of the external surface.
	mux.HandleFunc("/auth/", handleNotFound)

	// Frontend compatibility: it polls "/checkz" (no /auth prefix). Serve it
	// with the same tenant-enriched handler.
	mux.HandleFunc("/checkz", s.handleCheckz)

	mux.HandleFunc("/healthz", s.handleHealthz)

	// Landing page (and the /login alias so existing login links keep working).
	mux.HandleFunc("/login", s.handleLanding)
	mux.HandleFunc("/", s.handleLanding)

	return mux
}

// Handler supports composing explicitly shared listeners without duplicating routes.
func (s *Server) Handler() http.Handler { return s.httpServer.Handler }

// Start begins serving and blocks until the server is shut down. TLS is
// terminated upstream (LB/ingress), so this listens plaintext like the
// internal plane.
func (s *Server) Start() error {
	log.Printf("external: front-door listening on %s (post-login target %q)", s.opts.ListenAddr, s.opts.TargetURL)
	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Stop gracefully shuts the server down and releases the session store's
// Redis client (if any).
func (s *Server) Stop(ctx context.Context) error {
	err := s.httpServer.Shutdown(ctx)
	if s.redis != nil {
		_ = s.redis.Close()
	}
	return err
}

// handleHealthz answers LB / k8s probes.
func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// handleNotFound returns a JSON 404, matching the OSS proxy's not-found shape.
func handleNotFound(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
}

// methodGuard rejects any request whose method is not the allowed one with a
// JSON 405, before forwarding to next. Used to keep /auth/logout POST-only so a
// top-level SameSite=Lax GET cannot drive a CSRF logout.
func methodGuard(method string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusMethodNotAllowed)
			_, _ = w.Write([]byte(`{"error":"method not allowed"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// sanitizeLoginNext re-validates the ?next= parameter on /auth/login/<provider>
// before forwarding to the OSS login handler. The OSS handler only checks that
// next has a "/" prefix, so a protocol-relative "//evil.com" (or "/\evil.com")
// would survive and become the post-login redirect. We drop next when it fails
// s.sanitizeRedirect (the OSS handler then falls back to "/"); a valid relative
// or allowlisted-absolute next is preserved.
func (s *Server) sanitizeLoginNext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if nx := r.URL.Query().Get("next"); nx != "" {
			if _, ok := s.sanitizeRedirect(nx); !ok {
				q := r.URL.Query()
				q.Del("next")
				r2 := r.Clone(r.Context())
				r2.URL.RawQuery = q.Encode()
				next.ServeHTTP(w, r2)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// otelHandler wraps next with OpenTelemetry HTTP server instrumentation
// (server spans + http.server.* metrics) under the operation name "auth-go".
//
// Only /healthz is excluded: it is a k8s liveness/readiness probe, polled on a
// fixed interval, and carries no signal.
//
// /checkz and /auth/checkz are NOT excluded. They were, which left this plane
// emitting almost nothing — they are the SPA's session probe, i.e. real user
// traffic on the login/tenant-selection path, and exactly the thing worth
// tracing when someone reports being stuck logged out. The earlier grouping of
// them with /healthz looks like it was by name rather than by nature.
//
// When OTel is disabled (no global providers installed) this is effectively a
// passthrough.
func otelHandler(next http.Handler) http.Handler {
	return observability.HTTP(next, "external")
}

// withRecover wraps next so a panic in a handler is logged and turned into a
// JSON 500 instead of crashing the listener.
func withRecover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("external: panic serving %s: %v", r.URL.Path, rec)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"internal server error"}`))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
