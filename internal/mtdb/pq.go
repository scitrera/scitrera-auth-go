// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Scitrera LLC.
package mtdb

import (
	"github.com/lib/pq"
)

// pqArray adapts a *[]string to lib/pq's array scanner. Nil-arrayed columns
// (a tenant with no associated slugs) come back as an empty slice.
func pqArray(p *[]string) interface{} { return pq.Array(p) }
