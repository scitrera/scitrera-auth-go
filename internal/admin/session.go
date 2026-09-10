// SPDX-License-Identifier: AGPL-3.0-only
package admin

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

type Credentials struct {
	Operators map[string]string `json:"operators"`
}

var operatorRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}$`)

// Bootstrap writes an exclusive, private file; it never overwrites credentials
// or prints their values. Each named operator receives an independent token.
func Bootstrap(path, operator string) error {
	if path == "" || !operatorRE.MatchString(operator) {
		return fmt.Errorf("token-file and valid operator name are required")
	}
	raw, err := json.MarshalIndent(Credentials{map[string]string{operator: randomToken()}}, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.Write(append(raw, '\n')); err != nil {
		return err
	}
	return f.Sync()
}
func loadCredentials(path string) (map[string][32]byte, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("operator token file required: %w", err)
	}
	if !before.Mode().IsRegular() || before.Mode().Perm()&0077 != 0 || before.Size() > 65536 {
		return nil, fmt.Errorf("operator token file must be regular, <=64KiB, and private (mode 0600 or 0400)")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) {
		return nil, fmt.Errorf("operator token file changed during read")
	}
	decoder := json.NewDecoder(io.LimitReader(f, 65537))
	decoder.DisallowUnknownFields()
	var config Credentials
	if err = decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("invalid operator credentials file")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, fmt.Errorf("invalid operator credentials file")
	}
	if len(config.Operators) == 0 || len(config.Operators) > 100 {
		return nil, fmt.Errorf("configure 1–100 named operators")
	}
	out := map[string][32]byte{}
	for name, token := range config.Operators {
		decoded, err := base64.RawURLEncoding.DecodeString(token)
		if !operatorRE.MatchString(name) || err != nil || len(decoded) < 32 {
			return nil, fmt.Errorf("operator tokens must contain at least 32 random bytes, base64url encoded")
		}
		out[name] = sha256.Sum256([]byte(token))
	}
	return out, nil
}
func randomToken() string {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:])
}
func hash(s string) []byte { h := sha256.Sum256([]byte(s)); return h[:] }

type session struct {
	Operator string    `json:"operator"`
	Expires  time.Time `json:"expires_at"`
	CSRF     string    `json:"csrf_token,omitempty"`
	csrfHash []byte
}

func (s *Server) cookieName() string {
	if s.secure {
		return "__Host-auth_admin"
	}
	return "auth_admin_local"
}
func (s *Server) current(r *http.Request) (*session, error) {
	cookie, err := r.Cookie(s.cookieName())
	if err != nil || len(cookie.Value) != 43 {
		return nil, &problem{401, "unauthorized", "Sign in with an operator token"}
	}
	var out session
	var credential []byte
	err = s.store.DB.QueryRowContext(r.Context(), `SELECT operator,expires_at,csrf_hash,credential_hash FROM public.auth_admin_sessions WHERE token_hash=$1 AND expires_at>CURRENT_TIMESTAMP`, hash(cookie.Value)).Scan(&out.Operator, &out.Expires, &out.csrfHash, &credential)
	if err == sql.ErrNoRows {
		return nil, &problem{401, "unauthorized", "Session expired. Sign in again."}
	}
	if err != nil {
		return nil, err
	}
	expected, ok := s.operators[out.Operator]
	if !ok || subtle.ConstantTimeCompare(expected[:], credential) != 1 {
		return nil, &problem{401, "unauthorized", "Operator credentials changed. Sign in again."}
	}
	return &out, nil
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.loginLimit.Allow() {
		w.Header().Set("Retry-After", "5")
		s.fail(w, &problem{429, "rate_limited", "Too many sign-in attempts. Wait a few seconds."})
		return
	}
	var body struct {
		Operator string `json:"operator"`
		Token    string `json:"token"`
	}
	if err := decode(w, r, &body); err != nil {
		s.fail(w, err)
		return
	}
	expected, ok := s.operators[body.Operator]
	supplied := sha256.Sum256([]byte(body.Token))
	if subtle.ConstantTimeCompare(expected[:], supplied[:]) != 1 || !ok {
		s.fail(w, &problem{401, "unauthorized", "Invalid operator credentials"})
		return
	}
	token := randomToken()
	csrf := base64.RawURLEncoding.EncodeToString(hash("csrf:" + token))
	expires := time.Now().UTC().Add(s.ttl)
	tx, err := s.store.DB.BeginTx(r.Context(), nil)
	if err != nil {
		s.fail(w, err)
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(r.Context(), `DELETE FROM public.auth_admin_sessions WHERE expires_at<=CURRENT_TIMESTAMP`); err != nil {
		s.fail(w, err)
		return
	}
	// Reauthentication replaces the previous cookie's session, preventing fixation.
	if old, e := r.Cookie(s.cookieName()); e == nil {
		if _, err = tx.ExecContext(r.Context(), `DELETE FROM public.auth_admin_sessions WHERE token_hash=$1`, hash(old.Value)); err != nil {
			s.fail(w, err)
			return
		}
	}
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO public.auth_admin_sessions(token_hash,operator,credential_hash,csrf_hash,expires_at) VALUES($1,$2,$3,$4,$5)`, hash(token), body.Operator, expected[:], hash(csrf), expires); err != nil {
		s.fail(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		s.fail(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: s.cookieName(), Value: token, Path: "/", HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode, MaxAge: int(s.ttl.Seconds()), Expires: expires})
	s.json(w, 200, session{Operator: body.Operator, Expires: expires, CSRF: csrf})
}
func (s *Server) me(w http.ResponseWriter, r *http.Request, current *session) {
	cookie, _ := r.Cookie(s.cookieName())
	csrf := base64.RawURLEncoding.EncodeToString(hash("csrf:" + cookie.Value))
	current.CSRF = csrf
	s.json(w, 200, current)
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	cookie, _ := r.Cookie(s.cookieName())
	if _, err := s.store.DB.ExecContext(r.Context(), `DELETE FROM public.auth_admin_sessions WHERE token_hash=$1`, hash(cookie.Value)); err != nil {
		s.fail(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: s.cookieName(), Value: "", Path: "/", HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	s.json(w, 200, map[string]bool{"signed_out": true})
}
func validOrigin(origin string) bool { return origin != "" && !strings.HasSuffix(origin, "/") }
