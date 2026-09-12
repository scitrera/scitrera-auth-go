// SPDX-License-Identifier: AGPL-3.0-only
package admin

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/scitrera/aether/server/pkg/authproxy/login"
)

// Browser-session operations are independent of registry revisions. ServeHTTP
// has already enforced the operator session, Host, Origin and write CSRF token.
func (s *Server) userSessions(w http.ResponseWriter, r *http.Request, actor string, parts []string) {
	if len(parts) > 4 {
		s.fail(w, missing())
		return
	}
	if err := userID(parts[1]); err != nil {
		s.fail(w, err)
		return
	}
	if (r.Method != "GET" && r.Method != "DELETE") || (r.Method == "GET" && len(parts) != 3) {
		s.fail(w, &problem{405, "method_not_allowed", "Unsupported session operation"})
		return
	}
	ctx := r.Context()
	var email string
	err := s.store.DB.QueryRowContext(ctx, `SELECT email FROM public.users WHERE id=$1`, parts[1]).Scan(&email)
	if errors.Is(err, sql.ErrNoRows) {
		s.fail(w, missing())
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	manager, supported := s.browserSessions.(login.SessionManager)
	store := "disabled"
	if s.browserSessions != nil {
		store = s.browserSessions.Name()
	}
	if r.Method == "GET" {
		limit, offset := 50, 0
		for key, dest := range map[string]*int{"limit": &limit, "offset": &offset} {
			if raw := r.URL.Query().Get(key); raw != "" {
				n, err := strconv.Atoi(raw)
				if err != nil {
					s.fail(w, invalid("Invalid session pagination"))
					return
				}
				*dest = n
			}
		}
		if limit < 1 || limit > 200 || offset < 0 || offset > 1000000 {
			s.fail(w, invalid("Invalid session pagination"))
			return
		}
		page := &login.SessionPage{Sessions: []login.SessionInfo{}}
		if supported {
			page, err = manager.ListSessions(ctx, strings.ToLower(strings.TrimSpace(email)), limit, offset)
			if err != nil {
				s.fail(w, sessionStoreUnavailable())
				return
			}
		}
		s.json(w, 200, map[string]any{"supported": supported, "store": store, "sessions": page.Sessions, "has_more": page.HasMore})
		return
	}
	if !supported {
		s.fail(w, &problem{409, "session_management_unavailable", "Browser session management requires Redis/Valkey sessions"})
		return
	}
	if len(parts) == 4 {
		id := parts[3]
		_, err := hex.DecodeString(id)
		if len(id) != 64 || err != nil || id != strings.ToLower(id) {
			s.fail(w, invalid("Invalid session management ID"))
			return
		}
	}
	action := "session.revoke_all"
	if len(parts) == 4 {
		action = "session.revoke"
	}
	// PostgreSQL and Redis cannot share a transaction. Record the request durably
	// before touching Redis, then record completion. A failure response may mean
	// revocation already happened; callers should refresh before retrying. Never audit cookie-like
	// strings, OAuth claims, or tokens, even if supplied as a management ID.
	resource := "users/" + parts[1] + "/sessions"
	if err := s.sessionAudit(ctx, actor, action+".requested", resource); err != nil {
		s.fail(w, err)
		return
	}
	if len(parts) == 4 {
		err = manager.RevokeSession(ctx, strings.ToLower(strings.TrimSpace(email)), parts[3])
	} else {
		err = manager.RevokeAllSessions(ctx, strings.ToLower(strings.TrimSpace(email)))
	}
	if err != nil {
		s.fail(w, sessionStoreUnavailable())
		return
	}
	if err := s.sessionAudit(ctx, actor, action+".completed", resource); err != nil {
		s.fail(w, err)
		return
	}
	s.json(w, 200, map[string]bool{"revoked": true})
}

func sessionStoreUnavailable() error {
	return &problem{503, "session_store_unavailable", "Session store unavailable. Refresh to check the result before retrying."}
}

func (s *Server) sessionAudit(ctx context.Context, actor, action, resource string) error {
	result, err := s.store.DB.ExecContext(ctx, `INSERT INTO public.auth_admin_audit(operator,action,resource,fields,revision) SELECT $1,$2,$3,ARRAY['sessions'],revision FROM public.auth_admin_state WHERE singleton`, actor, action, resource)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("session audit did not persist a record")
	}
	return nil
}
