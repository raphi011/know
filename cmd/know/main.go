// Package main provides the CLI for Know.
// Most commands communicate with the server via REST API;
// db commands connect directly to SurrealDB.
package main

import (
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/raphi011/know/internal/apiclient"
	"github.com/raphi011/know/internal/keychain"
	"github.com/spf13/cobra"
)

// Version information — set by GoReleaser ldflags at build time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var rootCmd = &cobra.Command{
	Use:          "know",
	Short:        "Know CLI — document ingestion and management",
	SilenceUsage: true,
}

type apiFlags struct {
	URL   string
	Token string
}

func addAPIFlags(cmd *cobra.Command) *apiFlags {
	f := &apiFlags{}

	defaultURL := envOrDefault("KNOW_SERVER_URL", "http://localhost:4001")
	defaultToken := os.Getenv("KNOW_TOKEN")

	// Fall back to system keychain when no env var is set.
	if defaultToken == "" {
		if token, err := keychain.GetToken(); err == nil {
			defaultToken = token
		} else if !keychain.IsNotFound(err) {
			fmt.Fprintf(os.Stderr, "Warning: could not read keychain: %v\n", err)
		}
		if defaultURL == "http://localhost:4001" {
			if url, err := keychain.GetAPIURL(); err == nil && url != "" {
				defaultURL = url
			} else if err != nil && !keychain.IsNotFound(err) {
				fmt.Fprintf(os.Stderr, "Warning: could not read server URL from keychain: %v\n", err)
			}
		}
	}

	cmd.Flags().StringVar(&f.URL, "api-url", defaultURL, "REST API base URL")
	cmd.Flags().StringVar(&f.Token, "token", defaultToken, "API bearer token")
	if err := cmd.RegisterFlagCompletionFunc("api-url", noFileCompletions); err != nil {
		panic(fmt.Sprintf("register api-url completion: %v", err))
	}
	if err := cmd.RegisterFlagCompletionFunc("token", noFileCompletions); err != nil {
		panic(fmt.Sprintf("register token completion: %v", err))
	}
	return f
}

func (f *apiFlags) newClient() *apiclient.Client {
	return apiclient.New(f.URL, f.Token)
}

func addVaultFlag(cmd *cobra.Command, af *apiFlags) *string {
	v := new(string)
	cmd.Flags().StringVar(v, "vault", envOrDefault("KNOW_VAULT", "default"), "vault name (env: KNOW_VAULT)")
	if err := cmd.RegisterFlagCompletionFunc("vault", completeVaultNames(af)); err != nil {
		panic(fmt.Sprintf("register vault completion: %v", err))
	}
	return v
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("know %s (%s, %s, %s)\n", version, commit[:min(7, len(commit))], date, runtime.Version())
	},
}

func main() {
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(mvCmd)
	rootCmd.AddCommand(infoCmd)
	rootCmd.AddCommand(dbCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(agentCmd)
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(labelsCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(epubCmd)
	rootCmd.AddCommand(lsCmd)
	rootCmd.AddCommand(noteCmd)
	rootCmd.AddCommand(rmCmd)
	rootCmd.AddCommand(vaultCmd)
	rootCmd.AddCommand(remoteCmd)
	rootCmd.AddCommand(taskCmd)
	rootCmd.AddCommand(browseCmd)
	rootCmd.AddCommand(jobCmd)
	rootCmd.AddCommand(fetchCmd)
	rootCmd.AddCommand(backupCmd)
	rootCmd.AddCommand(restoreCmd)
	rootCmd.AddCommand(completionCmd)
	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(adminCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion script",
	Long: `Generate a shell completion script for the specified shell.

Examples:
  # Fish
  know completion fish > ~/.config/fish/completions/know.fish

  # Zsh
  know completion zsh > "${fpath[1]}/_know"

  # Bash
  know completion bash > /etc/bash_completion.d/know

  # PowerShell
  know completion powershell | Out-String | Invoke-Expression`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return rootCmd.GenBashCompletionV2(os.Stdout, true)
		case "zsh":
			return rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			return rootCmd.GenFishCompletion(os.Stdout, true)
		case "powershell":
			return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
		default:
			return fmt.Errorf("unsupported shell: %s", args[0])
		}
	},
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
