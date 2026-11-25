// Package dbtest gives tests an isolated, migrated Postgres schema.
package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mononendev/photo-gallery/internal/db"
)

// New creates schema t_<random> on GALLERY_TEST_PG, migrates it, and drops it at cleanup.
// Without GALLERY_TEST_PG the test is skipped, or fails when GALLERY_REQUIRE_PG=1.
func New(t testing.TB) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("GALLERY_TEST_PG")
	if url == "" {
		if os.Getenv("GALLERY_REQUIRE_PG") == "1" {
			t.Fatal("GALLERY_TEST_PG is required")
		}
		t.Skip("GALLERY_TEST_PG not set")
	}
	ctx := context.Background()
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	schema := "t_" + hex.EncodeToString(b[:])

	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE") //nolint:errcheck
		admin.Close()
	})
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	return pool
}
