// Package main provides the CLI for Knowhow.
// Most commands communicate with the server via REST API;
// dev commands connect directly to SurrealDB.
package main

import (
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strconv"

	"github.com/spf13/cobra"
)

// Version information — set by GoReleaser ldflags at build time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var (
	apiURL   string
	apiToken string
)

var rootCmd = &cobra.Command{
	Use:          "knowhow",
	Short:        "Knowhow CLI — document ingestion and management",
	SilenceUsage: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&apiURL, "api-url", envOrDefault("KNOWHOW_SERVER_URL", "http://localhost:4001"), "REST API base URL")
	rootCmd.PersistentFlags().StringVar(&apiToken, "token", os.Getenv("KNOWHOW_TOKEN"), "API bearer token")
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("knowhow %s (%s, %s, %s)\n", version, commit[:min(7, len(commit))], date, runtime.Version())
	},
}

func main() {
	rootCmd.AddCommand(scrapeCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(devCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(uiCmd)
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(labelsCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envOrDefaultInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		i, err := strconv.Atoi(v)
		if err != nil {
			slog.Warn("invalid integer env var, using default", "key", key, "value", v, "default", def, "error", err)
			return def
		}
		return i
	}
	return def
}

func envOrDefaultBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			slog.Warn("invalid boolean env var, using default", "key", key, "value", v, "default", def, "error", err)
			return def
		}
		return b
	}
	return def
}

func requireToken() error {
	if apiToken == "" {
		return fmt.Errorf("api token required: set KNOWHOW_TOKEN or use --token")
	}
	return nil
}
