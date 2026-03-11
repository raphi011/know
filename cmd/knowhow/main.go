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
	"strings"

	"github.com/raphi011/knowhow/internal/apiclient"
	"github.com/spf13/cobra"
)

// Version information — set by GoReleaser ldflags at build time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var rootCmd = &cobra.Command{
	Use:          "knowhow",
	Short:        "Knowhow CLI — document ingestion and management",
	SilenceUsage: true,
}

type apiFlags struct {
	URL   string
	Token string
}

func addAPIFlags(cmd *cobra.Command) *apiFlags {
	f := &apiFlags{}
	cmd.Flags().StringVar(&f.URL, "api-url", envOrDefault("KNOWHOW_SERVER_URL", "http://localhost:4001"), "REST API base URL")
	cmd.Flags().StringVar(&f.Token, "token", os.Getenv("KNOWHOW_TOKEN"), "API bearer token")
	return f
}

func (f *apiFlags) newClient() *apiclient.Client {
	return apiclient.New(f.URL, f.Token)
}

func addVaultFlag(cmd *cobra.Command) *string {
	v := new(string)
	cmd.Flags().StringVar(v, "vault", envOrDefault("KNOWHOW_VAULT", "default"), "vault name (env: KNOWHOW_VAULT)")
	return v
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("knowhow %s (%s, %s, %s)\n", version, commit[:min(7, len(commit))], date, runtime.Version())
	},
}

func main() {
	rootCmd.AddCommand(cpCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(devCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(agentCmd)
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(labelsCmd)
	rootCmd.AddCommand(backupCmd)
	rootCmd.AddCommand(lsCmd)
	rootCmd.AddCommand(catCmd)
	rootCmd.AddCommand(rmCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func envOrDefault(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
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
