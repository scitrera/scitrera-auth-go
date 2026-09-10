// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Scitrera LLC.
package external

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLogout_RedirectsToTargetAfterDelegateClearsSession(t *testing.T) {
	s := newTestServer(&fakeStore{}, Options{TargetURL: "https://app2.example.net"})

	loginMux := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name:   "scitrera_session",
			Value:  "",
			Path:   "/",
			MaxAge: -1,
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux := s.routes(loginMux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/auth/logout", nil))

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("POST /auth/logout status = %d, want 303", rr.Code)
	}
	if got := rr.Header().Get("Location"); got != "https://app2.example.net" {
		t.Errorf("Location = %q, want post-login target", got)
	}
	cookies := rr.Header().Values("Set-Cookie")
	if len(cookies) != 1 {
		t.Fatalf("Set-Cookie count = %d, want 1", len(cookies))
	}
	if !strings.Contains(cookies[0], "scitrera_session=") || !strings.Contains(cookies[0], "Max-Age=0") {
		t.Errorf("Set-Cookie = %q, want expired scitrera_session cookie", cookies[0])
	}
}
