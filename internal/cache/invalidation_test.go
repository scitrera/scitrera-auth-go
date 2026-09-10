// SPDX-License-Identifier: AGPL-3.0-only
package cache

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestInvalidateDuringLoadRetriesStaleResult(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	c := New(8, time.Hour, func(context.Context, string) (string, error) {
		if calls.Add(1) == 1 {
			close(started)
			<-release
			return "stale", nil
		}
		return "fresh", nil
	})
	result := make(chan string, 1)
	go func() { v, _ := c.Get(context.Background(), "user"); result <- v }()
	<-started
	c.Invalidate("user")
	fresh, err := c.Get(context.Background(), "user")
	if err != nil || fresh != "fresh" {
		t.Fatal("new request joined stale flight", fresh, err)
	}
	close(release)
	if got := <-result; got != "fresh" {
		t.Fatal("in-flight caller returned stale data", got)
	}
	if got, _ := c.Get(context.Background(), "user"); got != "fresh" {
		t.Fatal("stale repopulation")
	}
}
func TestPurgeInvalidatesCachedAbsence(t *testing.T) {
	var value any
	c := New[any](8, time.Hour, func(context.Context, string) (any, error) { return value, nil })
	c.Get(context.Background(), "policy")
	value = "new rule"
	c.Purge()
	if got, _ := c.Get(context.Background(), "policy"); got != value {
		t.Fatal("absence remained cached")
	}
}
func TestRevisionGateFailureAndReconnect(t *testing.T) {
	revision := int64(1)
	failed := false
	invalidations := 0
	gate := &RevisionGate{Load: func(context.Context) (int64, error) {
		if failed {
			return 0, errors.New("disconnected")
		}
		return revision, nil
	}, Invalidate: func() { invalidations++ }}
	if err := gate.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	failed = true
	revision = 3
	if gate.Check(context.Background()) == nil {
		t.Fatal("disconnect passed")
	}
	failed = false
	if err := gate.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if invalidations != 2 {
		t.Fatal("missed revision not reconciled", invalidations)
	}
}
