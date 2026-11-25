package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"github.com/mononendev/photo-gallery/internal/api"
)

func serveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Run the public API (GALLERY_PORT, 8080) and the admin API (GALLERY_ADMIN_PORT, 8081)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dsn := os.Getenv("GALLERY_DB")
			if dsn == "" {
				return errors.New("GALLERY_DB is not set")
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGTERM, os.Interrupt)
			defer stop()

			pool, err := pgxpool.New(ctx, dsn)
			if err != nil {
				return fmt.Errorf("opening the database: %w", err)
			}
			defer pool.Close()

			srv := &api.Server{Pool: pool, Version: version, Log: slog.Default()}
			public := &http.Server{Addr: ":" + envOr("GALLERY_PORT", "8080"), Handler: srv.Public()}
			admin := &http.Server{Addr: ":" + envOr("GALLERY_ADMIN_PORT", "8081"), Handler: srv.Admin()}

			errc := make(chan error, 2)
			for _, s := range []*http.Server{public, admin} {
				go func() {
					slog.Info("listening", "addr", s.Addr)
					if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
						errc <- fmt.Errorf("%s: %w", s.Addr, err)
					}
				}()
			}

			select {
			case err := <-errc:
				return err
			case <-ctx.Done():
			}
			slog.Info("shutting down")
			sctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			return errors.Join(public.Shutdown(sctx), admin.Shutdown(sctx))
		},
	}
}
