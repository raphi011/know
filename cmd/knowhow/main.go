// Package main provides the CLI for Knowhow.
// Most commands communicate with the server via GraphQL API;
// dev commands connect directly to SurrealDB.
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
	Use:          "knowhow",
	Short:        "Knowhow CLI — document ingestion and management",
	SilenceUsage: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&apiURL, "api-url", envOrDefault("KNOWHOW_SERVER_URL", "http://localhost:4001/query"), "GraphQL API URL")
	rootCmd.PersistentFlags().StringVar(&apiToken, "token", os.Getenv("KNOWHOW_TOKEN"), "API bearer token")
}

func main() {
	rootCmd.AddCommand(scrapeCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(devCmd)

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
