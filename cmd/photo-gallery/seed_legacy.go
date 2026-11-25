package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/mononendev/photo-gallery/internal/albumsync"
	"github.com/mononendev/photo-gallery/internal/db"
	"github.com/mononendev/photo-gallery/internal/gphotos"
	"github.com/mononendev/photo-gallery/internal/legacy"
	"github.com/mononendev/photo-gallery/internal/store"
)

func seedLegacyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "seed-legacy",
		Short: "Create the old gallery's events (published) and sync their albums one after another",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			events, err := legacy.Events()
			if err != nil {
				return err
			}
			dsn := os.Getenv("GALLERY_DB")
			if dsn == "" {
				return errors.New("GALLERY_DB is not set")
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGTERM, os.Interrupt)
			defer stop()
			pool, err := db.Open(ctx, dsn)
			if err != nil {
				return fmt.Errorf("opening the database: %w", err)
			}
			defer pool.Close()
			if err := db.Migrate(ctx, pool); err != nil {
				return fmt.Errorf("migrating the database: %w", err)
			}
			st := store.New(pool)
			for _, in := range events {
				if _, err := st.UpsertEvent(ctx, in); err != nil {
					return fmt.Errorf("event %s: %w", in.Slug, err)
				}
			}
			gp := &gphotos.Client{HTTP: &http.Client{Timeout: 60 * time.Second}}
			out := cmd.OutOrStdout()
			failed := 0
			for _, in := range events {
				res, err := seedSync(ctx, st, gp, in.Slug)
				if err != nil {
					failed++
					fmt.Fprintf(out, "%s: sync failed: %v\n", in.Slug, err)
					continue
				}
				fmt.Fprintf(out, "%s: %d albums, %d album photos, %d unmatched\n", in.Slug, res.Albums, res.Items, res.Unmatched)
			}
			if failed > 0 {
				return fmt.Errorf("%d of %d event syncs failed", failed, len(events))
			}
			return nil
		},
	}
}

func seedSync(ctx context.Context, st *store.Store, gp *gphotos.Client, slug string) (albumsync.Result, error) {
	id, err := st.StartSync(ctx, slug)
	if err != nil {
		return albumsync.Result{}, err
	}
	return albumsync.RunStarted(ctx, st, gp, id, slug)
}
