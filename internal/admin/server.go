// SPDX-License-Identifier: AGPL-3.0-only
package admin

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/scitrera/aether/server/pkg/authproxy/login"
	"golang.org/x/time/rate"
)

const Prefix = "/api/auth-admin/v1"

type Provider struct {
	Name       string   `json:"name"`
	Configured bool     `json:"configured"`
	Checks     []string `json:"supported_claim_examples"`
}
type Options struct {
	Addr, Origin, TokenFile string
	SessionTTL              time.Duration
	DB                      *sql.DB
	UI                      http.Handler
	Providers               []Provider
	BrowserSessions         login.SessionStore
}
type Server struct {
	store           *Store
	operators       map[string][32]byte
	origin, host    string
	secure          bool
	ttl             time.Duration
	http            *http.Server
	ui              http.Handler
	providers       []Provider
	loginLimit      *rate.Limiter
	browserSessions login.SessionStore
}

func New(opts Options) (*Server, error) {
	if opts.DB == nil || opts.Addr == "" {
		return nil, fmt.Errorf("admin requires a database and listen address")
	}
	u, err := url.Parse(opts.Origin)
	if err != nil || !validOrigin(opts.Origin) || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("SCITRERA_AUTH_ADMIN_ORIGIN must be an exact http(s) origin without a path")
	}
	secure := u.Scheme == "https"
	if !secure {
		ip := net.ParseIP(u.Hostname())
		if u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return nil, fmt.Errorf("HTTP admin origin is only supported on localhost/loopback; use HTTPS for remote administration")
		}
	}
	operators, err := loadCredentials(opts.TokenFile)
	if err != nil {
		return nil, err
	}
	ttl := opts.SessionTTL
	if ttl == 0 {
		ttl = 8 * time.Hour
	}
	if ttl < time.Minute || ttl > 24*time.Hour {
		return nil, fmt.Errorf("admin session TTL must be between 1m and 24h")
	}
	if opts.Providers == nil {
		opts.Providers = []Provider{}
	}
	s := &Server{store: &Store{opts.DB}, operators: operators, origin: opts.Origin, host: u.Host, secure: secure, ttl: ttl, ui: opts.UI, providers: opts.Providers, loginLimit: rate.NewLimiter(rate.Every(time.Second), 10)}
	s.browserSessions = opts.BrowserSessions
	s.http = &http.Server{Addr: opts.Addr, Handler: s, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	return s, nil
}
func (s *Server) Start() error {
	err := s.http.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
func (s *Server) Stop(ctx context.Context) error { return s.http.Shutdown(ctx) }

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
	if s.secure {
		w.Header().Set("Strict-Transport-Security", "max-age=31536000")
	}
	if r.Host != s.host {
		s.fail(w, &problem{403, "invalid_host", "Unexpected admin host"})
		return
	}
	origin := r.Header.Get("Origin")
	write := r.Method != "GET" && r.Method != "HEAD"
	publicPage := r.URL.Path == "/" || strings.HasPrefix(r.URL.Path, "/admin/")
	if (origin != "" && origin != s.origin) || (write && origin != s.origin) || (r.Header.Get("Sec-Fetch-Site") == "cross-site" && (write || !publicPage)) {
		s.fail(w, &problem{403, "origin_rejected", "Admin requests require the configured same origin"})
		return
	}
	if r.URL.Path == "/" {
		http.Redirect(w, r, "/admin/", http.StatusSeeOther)
		return
	}
	// Only the sign-in shell/assets are public. All live data stays in the API.
	if strings.HasPrefix(r.URL.Path, "/admin/") && s.ui != nil {
		if r.Method != "GET" && r.Method != "HEAD" {
			s.fail(w, &problem{405, "method_not_allowed", "Method not allowed"})
			return
		}
		s.ui.ServeHTTP(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	if r.URL.Path == Prefix+"/session" && r.Method == "POST" {
		s.login(w, r)
		return
	}
	current, err := s.current(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	if write && subtle.ConstantTimeCompare(hash(r.Header.Get("X-CSRF-Token")), current.csrfHash) != 1 {
		s.fail(w, &problem{403, "csrf_rejected", "CSRF token missing or invalid. Reload and try again."})
		return
	}
	switch {
	case r.URL.Path == Prefix+"/session/me" && r.Method == "GET":
		s.me(w, r, current)
	case r.URL.Path == Prefix+"/session" && r.Method == "DELETE":
		s.logout(w, r)
	default:
		s.api(w, r, current.Operator)
	}
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	typ, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || typ != "application/json" {
		return &problem{415, "content_type", "Content-Type must be application/json"}
	}
	r.Body = http.MaxBytesReader(w, r.Body, 65536)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err = d.Decode(v); err != nil {
		var max *http.MaxBytesError
		if errors.As(err, &max) {
			return &problem{413, "too_large", "Request body exceeds 64KiB"}
		}
		return invalid("Invalid JSON request")
	}
	if err = d.Decode(new(any)); err != io.EOF {
		return invalid("Expected one JSON object")
	}
	return nil
}
func (s *Server) json(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func (s *Server) fail(w http.ResponseWriter, err error) {
	var p *problem
	err = mapDBError(err)
	if !errors.As(err, &p) {
		log.Printf("auth-admin: request failed (%T)", err)
		p = &problem{500, "internal_error", "Unable to complete the request. Check database availability and server configuration."}
	}
	s.json(w, p.Status, map[string]string{"code": p.Code, "error": p.Message})
}
func expectedRevision(r *http.Request) (int64, error) {
	v := r.Header.Get("If-Match")
	if len(v) < 3 || v[0] != '"' || v[len(v)-1] != '"' {
		return 0, &problem{428, "revision_required", "Send the quoted revision in If-Match"}
	}
	n, err := strconv.ParseInt(v[1:len(v)-1], 10, 64)
	if err != nil || n < 0 {
		return 0, invalid("Invalid revision")
	}
	return n, nil
}
func (s *Server) respond(w http.ResponseWriter, e *Envelope, err error) {
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("ETag", fmt.Sprintf(`"%d"`, e.Revision))
	s.json(w, 200, e)
}
func (s *Server) api(w http.ResponseWriter, r *http.Request, actor string) {
	ctx := r.Context()
	path := strings.TrimPrefix(r.URL.Path, Prefix+"/")
	parts := strings.Split(path, "/")
	if !strings.HasPrefix(r.URL.Path, Prefix+"/") {
		s.fail(w, missing())
		return
	}
	if len(parts) >= 3 && parts[0] == "users" && parts[2] == "sessions" {
		s.userSessions(w, r, actor, parts)
		return
	}
	if r.Method == "GET" {
		limit, offset := 100, 0
		for key, p := range map[string]*int{"limit": &limit, "offset": &offset} {
			if raw := r.URL.Query().Get(key); raw != "" {
				n, err := strconv.Atoi(raw)
				if err != nil || n < 0 {
					s.fail(w, invalid("Invalid pagination"))
					return
				}
				*p = n
			}
		}
		if limit < 1 || limit > 200 || offset > 1000000 {
			s.fail(w, invalid("Limit must be 1–200 and offset at most 1000000"))
			return
		}
		filters := listFilters{}
		if path == "users" || path == "tenants" {
			filters.Query = strings.TrimSpace(r.URL.Query().Get("q"))
			filters.Enabled = r.URL.Query().Get("enabled")
			if (filters.Query != "" && !textValue(filters.Query, 200)) || (filters.Enabled != "" && filters.Enabled != "true" && filters.Enabled != "false") {
				s.fail(w, invalid("Invalid search or enabled filter"))
				return
			}
			if raw := r.URL.Query().Get("tenant"); raw != "" {
				if path != "users" {
					s.fail(w, invalid("Tenant filter applies to users only"))
					return
				}
				var err error
				filters.Tenant, err = slug(raw)
				if err != nil {
					s.fail(w, err)
					return
				}
			}
		}
		e, err := s.store.read(ctx, func(tx *sql.Tx) (any, error) {
			switch {
			case path == "status":
				return map[string]any{"schema_version": 1, "propagation_bound_ms": 1000, "revision_probe_timeout_ms": 2000, "admission": "membership-or-explicit-auto-add", "providers": s.providers}, nil
			case path == "tenants":
				return tenants(ctx, tx, "", limit, offset, filters)
			case len(parts) == 2 && parts[0] == "tenants":
				return tenants(ctx, tx, parts[1], 1, 0, listFilters{})
			case len(parts) == 3 && parts[0] == "tenants" && parts[2] == "domains":
				return domains(ctx, tx, parts[1])
			case len(parts) == 3 && parts[0] == "tenants" && parts[2] == "auth":
				return policy(ctx, tx, parts[1])
			case path == "users":
				return users(ctx, tx, "", limit, offset, filters)
			case len(parts) == 2 && parts[0] == "users":
				if err := userID(parts[1]); err != nil {
					return nil, err
				}
				return users(ctx, tx, parts[1], 1, 0, listFilters{})
			case len(parts) == 3 && parts[0] == "users" && parts[2] == "memberships":
				if err := userID(parts[1]); err != nil {
					return nil, err
				}
				u, err := users(ctx, tx, parts[1], 1, 0, listFilters{})
				if err != nil {
					return nil, err
				}
				return u.(User).Memberships, nil
			default:
				return nil, missing()
			}
		})
		s.respond(w, e, err)
		return
	}
	revision, err := expectedRevision(r)
	if err != nil {
		s.fail(w, err)
		return
	}
	var mutate func(*sql.Tx) error
	var fields []string
	switch {
	case path == "tenants" && r.Method == "POST", len(parts) == 2 && parts[0] == "tenants" && r.Method == "PUT":
		create := path == "tenants"
		var body TenantInput
		if err = decode(w, r, &body); err == nil {
			err = body.validate(create)
		}
		if err != nil {
			s.fail(w, err)
			return
		}
		target := body.Slug
		if !create {
			target = parts[1]
		}
		fields = []string{"name", "enabled"}
		for key := range body.Metadata {
			fields = append(fields, "metadata:"+key)
		}
		if create {
			path = "tenants/" + target
		}
		mutate = func(tx *sql.Tx) error { return saveTenant(ctx, tx, target, body, create) }
	case len(parts) == 3 && parts[0] == "tenants" && parts[2] == "domains" && r.Method == "POST":
		var body struct {
			Domain string `json:"domain"`
		}
		if err = decode(w, r, &body); err == nil {
			body.Domain, err = domain(body.Domain)
		}
		if err != nil {
			s.fail(w, err)
			return
		}
		fields = []string{"domains"}
		mutate = func(tx *sql.Tx) error {
			if err := exists(ctx, tx, "tenants", "slug", parts[1]); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, `INSERT INTO public.tenant_domains(tenant_slug,domain) VALUES($1,$2)`, parts[1], body.Domain)
			return err
		}
	case len(parts) == 4 && parts[0] == "tenants" && parts[2] == "domains" && r.Method == "DELETE":
		d, e := domain(parts[3])
		if e != nil {
			s.fail(w, e)
			return
		}
		fields = []string{"domains"}
		mutate = func(tx *sql.Tx) error {
			result, err := tx.ExecContext(ctx, `DELETE FROM public.tenant_domains WHERE tenant_slug=$1 AND lower(rtrim(btrim(domain),'.'))=$2`, parts[1], d)
			if err != nil {
				return err
			}
			n, _ := result.RowsAffected()
			if n == 0 {
				return missing()
			}
			return nil
		}
	case len(parts) == 3 && parts[0] == "tenants" && parts[2] == "auth" && r.Method == "PUT":
		var body PolicyInput
		if err = decode(w, r, &body); err == nil {
			err = body.validate()
		}
		if err != nil {
			s.fail(w, err)
			return
		}
		fields = []string{}
		if body.AutoAdd != nil {
			fields = append(fields, "auth:auto_add")
		}
		if body.Providers != nil {
			fields = append(fields, "auth:providers")
		}
		for provider := range body.Checks {
			fields = append(fields, "auth:checks:"+provider)
		}
		mutate = func(tx *sql.Tx) error { return savePolicy(ctx, tx, parts[1], body) }
	case path == "users" && r.Method == "POST", len(parts) == 2 && parts[0] == "users" && r.Method == "PUT":
		create := path == "users"
		var body UserInput
		if err = decode(w, r, &body); err == nil {
			err = body.validate()
		}
		if err != nil {
			s.fail(w, err)
			return
		}
		id := uuid.NewString()
		if !create {
			id = parts[1]
			if err = userID(id); err != nil {
				s.fail(w, err)
				return
			}
		}
		fields = []string{"email", "name", "enabled"}
		if body.DefaultTenant != nil {
			fields = append(fields, "default_tenant_slug")
		}
		if create {
			path = "users/" + id
		}
		mutate = func(tx *sql.Tx) error { return saveUser(ctx, tx, id, body, create) }
	case len(parts) == 3 && parts[0] == "users" && parts[2] == "memberships" && r.Method == "POST":
		var body struct {
			Tenant string `json:"tenant_slug"`
		}
		if err = decode(w, r, &body); err == nil {
			body.Tenant, err = slug(body.Tenant)
		}
		if err == nil {
			err = userID(parts[1])
		}
		if err != nil {
			s.fail(w, err)
			return
		}
		fields = []string{"memberships"}
		mutate = func(tx *sql.Tx) error { return membership(ctx, tx, parts[1], body.Tenant, false) }
	case len(parts) == 4 && parts[0] == "users" && parts[2] == "memberships" && r.Method == "DELETE":
		if err = userID(parts[1]); err != nil {
			s.fail(w, err)
			return
		}
		fields = []string{"memberships", "default_tenant_slug"}
		mutate = func(tx *sql.Tx) error { return membership(ctx, tx, parts[1], parts[3], true) }
	default:
		s.fail(w, &problem{405, "method_not_allowed", "Unsupported configuration operation"})
		return
	}
	sort.Strings(fields)
	// Resource paths can include email domains but never tokens or request blobs.
	e, err := s.store.write(ctx, revision, actor, r.Method, path, fields, mutate)
	s.respond(w, e, err)
}
