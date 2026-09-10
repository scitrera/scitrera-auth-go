// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Scitrera LLC.
package external

import (
	"bytes"
	"net/http"
)

// redirectLogout lets the OSS logout handler perform the actual session
// deletion and cookie clearing, then turns its successful JSON response into a
// browser redirect to the configured default target.
func (s *Server) redirectLogout(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capture := newLogoutResponseCapture()
		next.ServeHTTP(capture, r)

		status := capture.statusCode()
		if status < http.StatusOK || status >= http.StatusMultipleChoices {
			copyHeaders(w.Header(), capture.header)
			w.WriteHeader(status)
			_, _ = w.Write(capture.body.Bytes())
			return
		}

		for _, cookie := range capture.header.Values("Set-Cookie") {
			w.Header().Add("Set-Cookie", cookie)
		}
		http.Redirect(w, r, s.opts.TargetURL, http.StatusSeeOther)
	})
}

type logoutResponseCapture struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func newLogoutResponseCapture() *logoutResponseCapture {
	return &logoutResponseCapture{header: make(http.Header)}
}

func (c *logoutResponseCapture) Header() http.Header {
	return c.header
}

func (c *logoutResponseCapture) WriteHeader(status int) {
	if c.status == 0 {
		c.status = status
	}
}

func (c *logoutResponseCapture) Write(p []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	return c.body.Write(p)
}

func (c *logoutResponseCapture) statusCode() int {
	if c.status == 0 {
		return http.StatusOK
	}
	return c.status
}

func copyHeaders(dst http.Header, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}
