// SPDX-License-Identifier: AGPL-3.0-only
// Package testbrowser serves a disposable, real admin UI for Playwright. It is
// compiled as a test executable only; no fixture routes enter the product.
package testbrowser

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/scitrera/aether/server/pkg/authproxy/login"
	"github.com/scitrera/scitrera-auth-go/internal/admin"
	"github.com/scitrera/scitrera-auth-go/internal/adminui"
	"github.com/scitrera/scitrera-auth-go/internal/testdb"
)

func TestServeBrowserSessions(t *testing.T) {
	fixtureFile := os.Getenv("AUTH_BROWSER_SESSIONS_FIXTURE")
	if fixtureFile == "" {
		t.Skip("only run when explicitly launching Playwright fixtures")
	}
	if os.Getenv("AUTH_TEST_REDIS_ADDR") == "" {
		t.Fatal("AUTH_TEST_REDIS_ADDR is required")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	repo, _ := testdb.New(t)
	if err := repo.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	userID := uuid.NewString()
	const email = "browser-sessions@example.com"
	if _, err := repo.DB().ExecContext(ctx, `INSERT INTO public.users(id,email,name) VALUES($1,$2,'Session Browser Test')`, userID, email); err != nil {
		t.Fatal(err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: os.Getenv("AUTH_TEST_REDIS_ADDR")})
	prefix := "browser-session-test:" + uuid.NewString() + ":"
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
	store := login.NewRedisOpaqueSessionStore(rdb, prefix)
	for i := range 51 {
		if _, err := store.New(ctx, &login.SessionData{UserID: email, Email: email, Provider: "google", IssuedAt: time.Now().Add(-time.Duration(i) * time.Minute), ExpiresAt: time.Now().Add(time.Hour + time.Duration(i)*time.Minute)}); err != nil {
			t.Fatal(err)
		}
	}
	tokenFile := filepath.Join(t.TempDir(), "operators.json")
	if err := admin.Bootstrap(tokenFile, "operator"); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	baseURL := "http://" + listener.Addr().String()
	server, err := admin.New(admin.Options{Addr: listener.Addr().String(), Origin: baseURL, TokenFile: tokenFile, DB: repo.DB(), UI: adminui.Handler(), BrowserSessions: store})
	if err != nil {
		listener.Close()
		t.Fatal(err)
	}
	httpServer := &http.Server{Handler: server, ReadHeaderTimeout: 5 * time.Second}
	done := make(chan error, 1)
	go func() { done <- httpServer.Serve(listener) }()
	t.Cleanup(func() { httpServer.Close(); <-done })
	payload, err := json.Marshal(map[string]string{"base_url": baseURL, "user_id": userID, "token_file": tokenFile})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixtureFile, payload, 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(fixtureFile) })
	<-ctx.Done()
}
