// Package main provides the CLI for Knowhow.
// Most commands communicate with the server via GraphQL API;
// dev commands connect directly to SurrealDB.
package main

import (
	"fmt"
	"os"
	"runtime"

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
	rootCmd.PersistentFlags().StringVar(&apiURL, "api-url", envOrDefault("KNOWHOW_SERVER_URL", "http://localhost:4001/query"), "GraphQL API URL")
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

func requireToken() error {
	if apiToken == "" {
		return fmt.Errorf("api token required: set KNOWHOW_TOKEN or use --token")
	}
	return nil
}
