// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Scitrera LLC.
package cache

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestGet_NilInterfaceValue is the regression guard for the production panic
// "interface conversion: interface is nil, not interface {}" seen in auth-go's
// resolver. The cfgCache is a TTLCache[any] whose loader legitimately returns a
// nil value (a tenant with no auth:checks:<provider> config). On a COLD-CACHE
// miss the singleflight closure boxes the nil V into a nil `any`; the old
// single-return assertion `v.(V)` panicked on that nil. Get must instead return
// a nil value with no error so the caller can handle "no config".
func TestGet_NilInterfaceValue(t *testing.T) {
	loads := 0
	c := New[any](16, time.Minute, func(_ context.Context, _ string) (any, error) {
		loads++
		return nil, nil // legitimate "no value" result
	})

	// Cold miss: must NOT panic and must surface nil/no-error.
	v, err := c.Get(context.Background(), "tenant:auth:checks:azure")
	if err != nil {
		t.Fatalf("unexpected error on nil-value miss: %v", err)
	}
	if v != nil {
		t.Fatalf("expected nil value, got %#v", v)
	}

	// Warm hit: served from the entry table (line 52 path), still nil/no-error,
	// and the loader is not invoked again.
	v, err = c.Get(context.Background(), "tenant:auth:checks:azure")
	if err != nil {
		t.Fatalf("unexpected error on nil-value hit: %v", err)
	}
	if v != nil {
		t.Fatalf("expected nil value on hit, got %#v", v)
	}
	if loads != 1 {
		t.Fatalf("expected exactly 1 loader call (miss then cached), got %d", loads)
	}
}

// TestGet_NonNilValueRoundTrips ensures a normal (non-nil) value still round-trips
// through the singleflight/any boxing unchanged.
func TestGet_NonNilValueRoundTrips(t *testing.T) {
	want := map[string]any{"require": "groups"}
	c := New[any](16, time.Minute, func(_ context.Context, _ string) (any, error) {
		return want, nil
	})
	got, err := c.Get(context.Background(), "k")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := got.(map[string]any)
	if !ok || m["require"] != "groups" {
		t.Fatalf("value did not round-trip: %#v", got)
	}
}

// TestGet_LoaderErrorReturnsZero ensures a loader error yields the zero value and
// the error (and does not cache anything).
func TestGet_LoaderErrorReturnsZero(t *testing.T) {
	boom := errors.New("db down")
	c := New[any](16, time.Minute, func(_ context.Context, _ string) (any, error) {
		return nil, boom
	})
	v, err := c.Get(context.Background(), "k")
	if !errors.Is(err, boom) {
		t.Fatalf("expected loader error, got %v", err)
	}
	if v != nil {
		t.Fatalf("expected nil zero value on error, got %#v", v)
	}
	if c.Len() != 0 {
		t.Fatalf("error result must not be cached, Len=%d", c.Len())
	}
}

// TestGet_ConcreteTypeValue exercises a non-interface V (no nil-boxing hazard) to
// confirm the comma-ok assertion is transparent for concrete types too.
func TestGet_ConcreteTypeValue(t *testing.T) {
	c := New[string](16, time.Minute, func(_ context.Context, key string) (string, error) {
		return "val:" + key, nil
	})
	got, err := c.Get(context.Background(), "x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "val:x" {
		t.Fatalf("got %q", got)
	}
}
