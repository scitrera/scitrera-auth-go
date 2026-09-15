// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/scitrera/scitrera-auth-go/internal/admin"
	"github.com/scitrera/scitrera-auth-go/internal/mtdb"
)

var version = "0.1.3"
var revision = "development"

func command() (bool, error) {
	if len(os.Args) == 1 {
		return false, nil
	}
	switch os.Args[1] {
	case "version":
		fmt.Printf("scitrera-auth-proxy %s (%s)\n", version, revision)
		return true, nil
	case "bootstrap":
		f := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
		path := f.String("token-file", "", "new private operator credentials file")
		operator := f.String("operator", "operator", "operator audit identity")
		if err := f.Parse(os.Args[2:]); err != nil {
			return true, err
		}
		if f.NArg() != 0 {
			return true, fmt.Errorf("unexpected bootstrap arguments")
		}
		if err := admin.Bootstrap(*path, *operator); err != nil {
			return true, err
		}
		fmt.Printf("Operator credentials created in %s (0600). Keep this file private.\n", *path)
		return true, nil
	case "migrate":
		if len(os.Args) != 2 {
			return true, fmt.Errorf("migrate takes no arguments; set SCITRERA_MT_DB_URL")
		}
		dsn := os.Getenv("SCITRERA_MT_DB_URL")
		if dsn == "" {
			return true, fmt.Errorf("SCITRERA_MT_DB_URL is required")
		}
		repo, err := mtdb.New(dsn)
		if err != nil {
			return true, err
		}
		defer repo.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err = repo.Migrate(ctx); err != nil {
			return true, err
		}
		fmt.Println("Auth schema version 1 ready.")
		return true, nil
	case "help", "-h", "--help":
		fmt.Println("Usage: scitrera-auth-proxy [migrate | bootstrap --token-file PATH --operator NAME | version]\nNo arguments starts the configured auth listeners. See README.md for environment configuration.")
		return true, nil
	default:
		return true, fmt.Errorf("unknown command %q", os.Args[1])
	}
}
