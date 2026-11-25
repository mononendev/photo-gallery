package main

import (
	"errors"
	"os"

	"github.com/spf13/cobra"

	"github.com/mononendev/photo-gallery/internal/db"
)

func migrateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "migrate",
		Short: "Apply database migrations and exit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dsn := os.Getenv("GALLERY_DB")
			if dsn == "" {
				return errors.New("GALLERY_DB is not set")
			}
			pool, err := db.Open(cmd.Context(), dsn)
			if err != nil {
				return err
			}
			defer pool.Close()
			return db.Migrate(cmd.Context(), pool)
		},
	}
}
