// Package main provides the CLI for Knowhow v2.
// All commands communicate with the server via GraphQL API.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	apiURL   string
	apiToken string
)

var rootCmd = &cobra.Command{
	Use:          "knowhow-v2",
	Short:        "Knowhow v2 CLI — document ingestion and management",
	SilenceUsage: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&apiURL, "api-url", envOrDefault("KNOWHOW_V2_URL", "http://localhost:8485/query"), "GraphQL API URL")
	rootCmd.PersistentFlags().StringVar(&apiToken, "token", os.Getenv("KNOWHOW_V2_TOKEN"), "API bearer token")
}

func main() {
	rootCmd.AddCommand(scrapeCmd)

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
		return fmt.Errorf("API token required: set KNOWHOW_V2_TOKEN or use --token")
	}
	return nil
}
