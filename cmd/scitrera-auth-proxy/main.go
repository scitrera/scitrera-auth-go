// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Scitrera LLC.
// scitrera-auth-proxy is the multi-tenant variant of Aether's auth-proxy.
//
// It thinly wraps github.com/scitrera/aether/pkg/authproxy.Run by injecting
// a ScitreraIdentityResolver that talks to the Scitrera MT Postgres schema
// (users / tenants / user_tenants / tenant_domains / tenant_config). Every
// other piece of the auth-proxy — composite authenticator chain, ACL, OBO
// authority resolution, browser OAuth login flow — comes from the OSS
// library unchanged.
//
// Configuration: same env vars as the OSS auth-proxy, plus:
//
//   - SCITRERA_MT_DB_URL (required) — MT Postgres connection string
//   - SCITRERA_AUTH_CACHE_TTL (optional, default 5m)
//   - SCITRERA_AUTH_CACHE_CAPACITY (optional, default 16384)
//   - SCITRERA_AUTH_CHECKZ_ALLOWED_ORIGINS (optional) — comma-separated full
//     origins (scheme+host) allowed to call /checkz; empty disables the
//     server-side Origin reject (relies on edge CORS)
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"errors"

	pkgauthproxy "github.com/scitrera/aether/server/pkg/authproxy"
	"github.com/scitrera/aether/server/pkg/authproxy/login"

	"github.com/scitrera/scitrera-auth-go/internal/admin"
	"github.com/scitrera/scitrera-auth-go/internal/adminui"
	"github.com/scitrera/scitrera-auth-go/internal/external"
	"github.com/scitrera/scitrera-auth-go/internal/mtdb"
	"github.com/scitrera/scitrera-auth-go/internal/observability"
	"github.com/scitrera/scitrera-auth-go/internal/resolver"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	if handled, err := command(); handled {
		if err != nil {
			return err
		}
		return nil
	}
	log.Println("Scitrera auth-proxy starting...")

	// Root context cancelled on SIGINT/SIGTERM so both planes drain gracefully.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if os.Getenv("AUTH_PROXY_DB_URL") == "" && os.Getenv("SCITRERA_MT_DB_URL") != "" {
		if err := os.Setenv("AUTH_PROXY_DB_URL", proxyDSN(os.Getenv("SCITRERA_MT_DB_URL"))); err != nil {
			return err
		}
	}
	cfg, err := pkgauthproxy.LoadConfigFromEnv()
	if err != nil {
		return fmt.Errorf("auth-proxy: failed to load config: %v", err)
	}

	plan, err := planListeners(cfg.ListenAddr, os.Getenv("SCITRERA_AUTH_EXTERNAL_ADDR"), os.Getenv("SCITRERA_AUTH_ADMIN_ADDR"), os.Getenv("SCITRERA_AUTH_METRICS_ADDR"))
	if err != nil {
		return err
	}
	telemetry, err := observability.Init(ctx, plan.metrics != "", version)
	if err != nil {
		return fmt.Errorf("observability: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := telemetry.Shutdown(shutdownCtx); err != nil {
			log.Printf("observability shutdown: %v", err)
		}
	}()

	mtDSN := os.Getenv("SCITRERA_MT_DB_URL")
	if mtDSN == "" {
		return fmt.Errorf("SCITRERA_MT_DB_URL is required")
	}
	repo, err := mtdb.New(mtDSN)
	if err != nil {
		return fmt.Errorf("scitrera_mt: failed to connect to MT Postgres: %v", err)
	}
	defer repo.Close()
	if err := repo.SchemaSmokeTest(context.Background()); err != nil {
		return fmt.Errorf("scitrera_mt: schema smoke test failed (column rename or missing table?): %v", err)
	}

	mtResolver, err := resolver.New(resolver.Options{
		Repo:          repo,
		CacheTTL:      parseDur(os.Getenv("SCITRERA_AUTH_CACHE_TTL"), 5*time.Minute),
		CacheCapacity: parseInt(os.Getenv("SCITRERA_AUTH_CACHE_CAPACITY"), 16384),
	})
	if err != nil {
		return fmt.Errorf("scitrera_mt: failed to construct resolver: %v", err)
	}

	// Admin can bind privately or explicitly share a configured listener.
	var adminServer *admin.Server
	if addr := plan.admin; addr != "" {
		loginConfig, e := pkgauthproxy.LoadLoginConfigFromEnv()
		if e != nil {
			return fmt.Errorf("admin provider configuration: %v", e)
		}
		providers := []admin.Provider{}
		var browserSessions login.SessionStore
		if loginConfig.Enabled {
			store, redisClient, err := loginConfig.BuildSessionStore()
			if err != nil {
				return fmt.Errorf("admin browser sessions: %w", err)
			}
			browserSessions = store
			if redisClient != nil {
				defer redisClient.Close()
			}
		}
		for _, p := range loginConfig.Providers {
			examples := []string{"email", "sub"}
			switch p.Name {
			case "azure", "entra", "microsoft":
				examples = append(examples, "tid")
			case "google":
				examples = append(examples, "hd")
			}
			providers = append(providers, admin.Provider{Name: p.Name, Configured: p.IssuerURL != "" && p.ClientID != "" && p.RedirectURL != "", Checks: examples})
		}
		adminServer, err = admin.New(admin.Options{Addr: addr, Origin: os.Getenv("SCITRERA_AUTH_ADMIN_ORIGIN"), TokenFile: os.Getenv("SCITRERA_AUTH_ADMIN_TOKEN_FILE"), SessionTTL: parseDur(os.Getenv("SCITRERA_AUTH_ADMIN_SESSION_TTL"), 8*time.Hour), DB: repo.DB(), UI: adminui.Handler(), Providers: providers, BrowserSessions: browserSessions})
		if err != nil {
			return fmt.Errorf("admin configuration: %v", err)
		}
		log.Printf("operator dashboard enabled on %s", addr)
	}

	// Construct all surfaces before binding. Identical configured addresses
	// explicitly share the same router; distinct addresses remain isolated.
	libraryInitialized := make(chan struct{})
	var ext *external.Server
	if extAddr := plan.external; extAddr != "" {
		ext, err = external.New(external.Options{
			ListenAddr:           extAddr,
			Source:               adminui.SourceHandler(),
			TargetURL:            os.Getenv("SCITRERA_AUTH_POST_LOGIN_TARGET"),
			AllowedRedirectHosts: splitCSV(os.Getenv("SCITRERA_AUTH_ALLOWED_REDIRECT_HOSTS")),
			// Server-side Origin allowlist for /checkz + /auth/checkz (defense-in-depth
			// on top of the edge Envoy CORS). Full origins (scheme+host), comma-separated;
			// populated from the same CORS allow-origins list via the Helm chart. Empty =
			// no origin enforcement (rely on CORS).
			CheckzAllowedOrigins: splitCSV(os.Getenv("SCITRERA_AUTH_CHECKZ_ALLOWED_ORIGINS")),
			ReturnCookieHMACKey:  []byte(os.Getenv("AUTH_PROXY_TOKEN_HMAC_KEY")),
			Branding:             external.BrandingFromEnv(),
			DefaultTenant:        os.Getenv("SCITRERA_AUTH_LOGIN_DEFAULT_TENANT"),
			// Share tenant presentation metadata for login branding and the
			// user's tenant list for /checkz (same source as /auth/verify).
			Repo: repo,
		})
		if err != nil {
			return fmt.Errorf("external front-door: %v", err)
		}
		if os.Getenv("AUTH_PROXY_TOKEN_HMAC_KEY") == "" {
			log.Printf("external front-door: WARNING AUTH_PROXY_TOKEN_HMAC_KEY unset — " +
				"post-login return cookie is unsigned (sanitizeRedirect still re-checked on read)")
		}
		log.Printf("external front-door enabled on %s", extAddr)
	}

	var adminHandler, publicHandler http.Handler
	if adminServer != nil {
		adminHandler = observability.HTTP(adminServer, "admin")
	}
	if ext != nil {
		publicHandler = ext.Handler()
	}
	serveErrCh := make(chan error, len(plan.extraAddresses()))
	var extraServers []*http.Server
	for _, addr := range plan.extraAddresses() {
		srv := &http.Server{Addr: addr, Handler: plan.handler(addr, nil, publicHandler, adminHandler, telemetry.Metrics), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
		extraServers = append(extraServers, srv)
		go func() {
			select {
			case <-libraryInitialized:
			case <-ctx.Done():
				return
			}
			err := srv.ListenAndServe()
			if errors.Is(err, http.ErrServerClosed) {
				err = nil
			}
			if err != nil {
				err = fmt.Errorf("listener %s: %w", srv.Addr, err)
			}
			serveErrCh <- err
		}()
	}
	if plan.metrics != "" {
		log.Printf("metrics enabled on %s/metrics", plan.metrics)
	}

	// Run the internal plane in a goroutine so we can select between it, the
	// external plane, and signal-driven cancellation.
	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- pkgauthproxy.Run(
			ctx, cfg,
			pkgauthproxy.WithIdentityResolver(mtResolver),
			pkgauthproxy.WithHandlerMiddleware(func(next http.Handler) http.Handler {
				close(libraryInitialized)
				return plan.handler(plan.internal, internalOtelHandler(next), publicHandler, adminHandler, telemetry.Metrics)
			}),
		)
	}()

	var runFailure error
	internalDone := false
	select {
	case <-ctx.Done():
		log.Printf("auth-proxy: shutdown signal received, draining...")
	case runErr := <-runErrCh:
		internalDone = true
		if runErr != nil {
			runFailure = fmt.Errorf("auth-proxy: internal plane error: %w", runErr)
		}
	case listenerErr := <-serveErrCh:
		runFailure = listenerErr
	}

	// Cancel ctx so the internal plane (still blocked in Run when an external
	// error or signal woke us) drains, then stop the external plane.
	stop()
	for _, srv := range extraServers {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = srv.Shutdown(shutdownCtx)
		cancel()
	}
	if ext != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = ext.Stop(shutdownCtx)
		cancel()
	}
	// Wait for the internal plane to finish draining (unless it was already the
	// path that woke us) before returning, so its deferred cleanup and the
	// deferred repo.Close() above run un-truncated.
	if !internalDone {
		if err := <-runErrCh; err != nil {
			log.Printf("auth-proxy: internal plane drain error: %v", err)
		}
	}
	return runFailure
}

// splitCSV splits a comma-separated env value into trimmed, non-empty parts.
func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func parseDur(s string, def time.Duration) time.Duration {
	if s == "" {
		return def
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	return def
}

func parseInt(s string, def int) int {
	if s == "" {
		return def
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}

// internalOtelHandler instruments trusted verification/proxy traffic.
func internalOtelHandler(next http.Handler) http.Handler {
	return observability.HTTP(next, "internal")
}

// The dedicated auth installation isolates Aether tables from public MT tables.
// Explicit AUTH_PROXY_DB_URL keeps the existing platform integration unchanged.
func proxyDSN(dsn string) string {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err == nil {
			q := u.Query()
			q.Set("search_path", "auth_proxy,public")
			u.RawQuery = q.Encode()
			return u.String()
		}
	}
	return dsn + " search_path='auth_proxy,public'"
}
