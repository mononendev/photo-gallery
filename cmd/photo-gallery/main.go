// Command photo-gallery serves a public event photo gallery: Google Photos albums, matched to the metadata
// photosort computed for the same shots.
package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

// Set at build time (-ldflags "-X main.version=...").
var version = "dev"

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	root := &cobra.Command{
		Use:           "photo-gallery",
		Short:         "Public event photo gallery",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(serveCmd())
	root.AddCommand(migrateCmd())
	root.AddCommand(syncCmd())
	root.AddCommand(seedLegacyCmd())
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
