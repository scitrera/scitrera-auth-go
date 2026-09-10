// SPDX-License-Identifier: AGPL-3.0-only
// Package testdb provisions only uniquely named disposable test databases.
package testdb

import (
	"database/sql"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/scitrera/scitrera-auth-go/internal/mtdb"
)

func New(t *testing.T) (*mtdb.Repo, string) {
	t.Helper()
	dsn := os.Getenv("AUTH_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set AUTH_TEST_POSTGRES_DSN to a disposable PostgreSQL service (requires CREATEDB)")
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	name := "auth_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = db.Exec(`CREATE DATABASE "` + name + `"`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec(`DROP DATABASE "` + name + `" WITH (FORCE)`); err != nil {
			t.Errorf("cleanup owned test database: %v", err)
		}
	})
	u.Path = "/" + name
	repo, err := mtdb.New(u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repo.Close() })
	return repo, u.String()
}
