// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (c) 2026 Scitrera LLC.
package migrations

import "embed"

// Files are embedded so the migration command works from any directory.
//
//go:embed *.sql
var Files embed.FS
