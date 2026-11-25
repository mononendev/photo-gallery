package main

import (
	"encoding/json"
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
	"github.com/mononendev/photo-gallery/internal/store"
)

func syncCmd() *cobra.Command {
	var event string
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Fetch an event's Google Photos albums and match them to photosort rows",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
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
			id, err := st.StartSync(ctx, event)
			switch {
			case errors.Is(err, store.ErrNotFound):
				return fmt.Errorf("event %s not found", event)
			case err != nil:
				return err
			}
			gp := &gphotos.Client{HTTP: &http.Client{Timeout: 60 * time.Second}}
			res, err := albumsync.RunStarted(ctx, st, gp, id, event)
			if err != nil {
				return err
			}
			return writeResult(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().StringVar(&event, "event", "", "event slug")
	cmd.MarkFlagRequired("event")
	return cmd
}

func writeResult(w interface{ Write([]byte) (int, error) }, res albumsync.Result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(res)
}
