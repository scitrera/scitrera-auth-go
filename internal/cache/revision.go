// SPDX-License-Identifier: AGPL-3.0-only
package cache

import (
	"context"
	"sync"
	"time"
)

// RevisionGate reconciles using the database on demand. A missed notification
// or a reconnect needs no special recovery: the next expired probe rechecks it.
// Errors never extend the last successful freshness window.
type RevisionGate struct {
	mu         sync.Mutex
	revision   int64
	checked    time.Time
	Interval   time.Duration
	Load       func(context.Context) (int64, error)
	Invalidate func()
}

func (g *RevisionGate) Check(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.checked.IsZero() && time.Since(g.checked) < g.Interval {
		return nil
	}
	started := time.Now()
	probe, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	revision, err := g.Load(probe)
	if err != nil {
		g.checked = time.Time{}
		return err
	}
	if g.checked.IsZero() || revision != g.revision {
		g.Invalidate()
	}
	g.revision = revision
	g.checked = started
	return nil
}
