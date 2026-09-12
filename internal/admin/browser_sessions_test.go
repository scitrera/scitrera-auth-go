// SPDX-License-Identifier: AGPL-3.0-only
package admin_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/scitrera/aether/server/pkg/authproxy/login"
	"github.com/scitrera/scitrera-auth-go/internal/admin"
)

func browserStore(t *testing.T) (*login.RedisOpaqueSessionStore, *redis.Client, string) {
	t.Helper()
	addr := os.Getenv("AUTH_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("set AUTH_TEST_REDIS_ADDR to a disposable Redis/Valkey service")
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	prefix := "auth-session-test:" + uuid.NewString() + ":"
	t.Cleanup(func() {
		var cursor uint64
		for {
			keys, next, err := rdb.Scan(context.Background(), cursor, prefix+"*", 100).Result()
			if err != nil {
				break
			}
			if len(keys) > 0 {
				rdb.Del(context.Background(), keys...)
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
		rdb.Close()
	})
	return login.NewRedisOpaqueSessionStore(rdb, prefix), rdb, prefix
}

func seedBrowserSession(t *testing.T, store login.SessionStore, email string) string {
	t.Helper()
	id, err := store.New(context.Background(), &login.SessionData{UserID: email, Email: email, Name: "Person", Provider: "google", Claims: map[string]any{"private": "do-not-expose"}, IssuedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func browserUser(c *client) string {
	c.call("POST", "/users", userBody(true), 200)
	return c.call("GET", "/users", nil, 200)["data"].([]any)[0].(map[string]any)["id"].(string)
}

func TestBrowserSessionAdministration(t *testing.T) {
	store, rdb, prefix := browserStore(t)
	c, repo, _ := setup(t, store)
	user := browserUser(c)
	path := "/users/" + user + "/sessions"
	token := seedBrowserSession(t, store, "Person@Example.COM")
	second := seedBrowserSession(t, store, "person@example.com")
	other := seedBrowserSession(t, store, "other@example.com")
	// Separate store instances model the public/internal and private listeners.
	replica := login.NewRedisOpaqueSessionStore(rdb, prefix)
	result := c.call("GET", path+"?limit=1", nil, 200)
	if result["supported"] != true || result["has_more"] != true {
		t.Fatal(result)
	}
	sessions := result["sessions"].([]any)
	id := sessions[0].(map[string]any)["id"].(string)
	raw, _ := json.Marshal(result)
	if strings.Contains(string(raw), token) || strings.Contains(string(raw), "do-not-expose") {
		t.Fatal("sensitive session data exposed")
	}
	for _, tc := range []struct {
		cookie       bool
		origin, csrf string
		status       int
	}{
		{false, "https://admin.example.test", c.csrf, 401},
		{true, "https://admin.example.test", "", 403},
		{true, "https://foreign.example.test", c.csrf, 403},
	} {
		r := httptest.NewRequest("DELETE", "https://admin.example.test"+admin.Prefix+path, nil)
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("X-CSRF-Token", tc.csrf)
		if tc.cookie {
			r.AddCookie(c.cookie)
		}
		w := httptest.NewRecorder()
		c.s.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("boundary: got %d want %d", w.Code, tc.status)
		}
	}
	ctx := context.Background()
	otherPage, err := store.ListSessions(ctx, "other@example.com", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	c.call("DELETE", path+"/"+otherPage.Sessions[0].ID, nil, 200)
	if data, err := replica.Get(ctx, other); err != nil || data == nil {
		t.Fatal("revoked another user's session")
	}
	revision := c.revision
	// Session writes require CSRF but deliberately no configuration If-Match.
	r := httptest.NewRequest("DELETE", "https://admin.example.test"+admin.Prefix+path+"/"+id, nil)
	r.Header.Set("Origin", "https://admin.example.test")
	r.Header.Set("X-CSRF-Token", c.csrf)
	r.AddCookie(c.cookie)
	w := httptest.NewRecorder()
	c.s.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	remaining := c.call("GET", path, nil, 200)["sessions"].([]any)
	if len(remaining) != 1 {
		t.Fatal("single revocation", remaining)
	}
	c.call("DELETE", path, nil, 200)
	for _, token := range []string{token, second} {
		if data, err := replica.Get(ctx, token); err != nil || data != nil {
			t.Fatal("revocation not visible to replica", err)
		}
	}
	if len(c.call("GET", path, nil, 200)["sessions"].([]any)) != 0 {
		t.Fatal("sessions remain listed")
	}
	c.call("GET", "/status", nil, 200)
	if c.revision != revision {
		t.Fatal("session revocation changed configuration revision")
	}
	var audit string
	err = repo.DB().QueryRow(`SELECT COALESCE(json_agg(row_to_json(a))::text,'[]') FROM public.auth_admin_audit a WHERE action LIKE 'session.%'`).Scan(&audit)
	if err != nil {
		t.Fatal(err)
	}
	for _, sensitive := range []string{token, second, other, id, "do-not-expose"} {
		if strings.Contains(audit, sensitive) {
			t.Fatal("sensitive data in audit")
		}
	}
	if !strings.Contains(audit, "session.revoke_all.completed") || !strings.Contains(audit, "session.revoke.requested") {
		t.Fatal("missing audit", audit)
	}
	for _, suffix := range []string{"?limit=0", "?offset=-1", "?limit=201", "?offset=oops"} {
		c.call("GET", path+suffix, nil, 400)
	}
	c.call("DELETE", path+"/bad-id", nil, 400)
	c.call("GET", "/users/"+uuid.NewString()+"/sessions", nil, 404)
	c.call("POST", path, nil, 405)
	rdb.Close()
	c.call("GET", path, nil, 503)
	c.call("DELETE", path, nil, 503)
}

func TestBrowserSessionUnsupportedModes(t *testing.T) {
	jwt, err := login.NewSignedJWTSessionStore([]byte(strings.Repeat("k", 32)), "test")
	if err != nil {
		t.Fatal(err)
	}
	for _, store := range []login.SessionStore{nil, jwt} {
		c, _, _ := setup(t, store)
		path := "/users/" + browserUser(c) + "/sessions"
		result := c.call("GET", path, nil, 200)
		if result["supported"] != false {
			t.Fatal("reported unsupported store as manageable")
		}
		c.call("DELETE", path, nil, 409)
	}
}

func TestSessionRevocationRequiresDurableAudit(t *testing.T) {
	for _, failure := range []string{"DROP TABLE public.auth_admin_audit", "DELETE FROM public.auth_admin_state"} {
		t.Run(failure, func(t *testing.T) {
			store, _, _ := browserStore(t)
			c, repo, _ := setup(t, store)
			path := "/users/" + browserUser(c) + "/sessions"
			token := seedBrowserSession(t, store, "person@example.com")
			if _, err := repo.DB().Exec(failure); err != nil {
				t.Fatal(err)
			}
			c.call("DELETE", path, nil, 500)
			if data, err := store.Get(context.Background(), token); err != nil || data == nil {
				t.Fatal("revoked without an audit record", err)
			}
		})
	}
}
