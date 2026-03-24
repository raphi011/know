// Package main provides the CLI for Know.
// Most commands communicate with the server via REST API.
package main

import (
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/raphi011/know/internal/apiclient"
	"github.com/raphi011/know/internal/config"
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

// globalAPI holds the root-level --api-url and --token flags used by all REST commands.
var globalAPI apiFlags

// Logging flags and state shared across all commands.
var (
	globalLogLevel  string
	globalLogFormat string
	globalLogFile   string
	globalLogClean  func() error  // set by PersistentPreRunE, called in main()
	globalLevelVar  slog.LevelVar // shared so commands can read/change level dynamically
)

func init() {
	pf := rootCmd.PersistentFlags()

	// API flags
	defaultURL := envOrDefault("KNOW_SERVER_URL", "http://localhost:4001")
	defaultToken := os.Getenv("KNOW_TOKEN")

	pf.StringVar(&globalAPI.URL, "api-url", defaultURL, "REST API base URL")
	pf.StringVar(&globalAPI.Token, "token", defaultToken, "API bearer token")
	if err := rootCmd.RegisterFlagCompletionFunc("api-url", noFileCompletions); err != nil {
		panic(fmt.Sprintf("register api-url completion: %v", err))
	}
	if err := rootCmd.RegisterFlagCompletionFunc("token", noFileCompletions); err != nil {
		panic(fmt.Sprintf("register token completion: %v", err))
	}

	// Logging flags
	pf.StringVar(&globalLogLevel, "log-level", envOrDefault("KNOW_LOG_LEVEL", "info"), "log level (debug, info, warn, error)")
	pf.StringVar(&globalLogFormat, "log-format", envOrDefault("KNOW_LOG_FORMAT", "text"), "log format (text, json)")
	pf.StringVar(&globalLogFile, "log-file", os.Getenv("KNOW_LOG_FILE"), "log file path (logs to stderr when empty)")

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		var level slog.Level
		if err := level.UnmarshalText([]byte(globalLogLevel)); err != nil {
			return fmt.Errorf("invalid log level %q: %w", globalLogLevel, err)
		}
		globalLevelVar.Set(level)

		format, err := config.ParseLogFormat(globalLogFormat)
		if err != nil {
			return err
		}

		logger, cleanup, err := config.SetupLogger(globalLogFile, &globalLevelVar, format)
		if err != nil {
			return fmt.Errorf("setup logger: %w", err)
		}
		globalLogClean = cleanup
		slog.SetDefault(logger)
		return nil
	}
}

type apiFlags struct {
	URL   string
	Token string
}

// newClient creates an API client, falling back to the system keychain
// for token and URL when not provided via flags or environment variables.
// Reads from the struct but does not mutate it — keychain values are used
// as local overrides only.
func (f *apiFlags) newClient() *apiclient.Client {
	token := f.Token
	url := f.URL

	if token == "" {
		if t, err := keychain.GetToken(); err == nil {
			token = t
		} else if !keychain.IsNotFound(err) {
			fmt.Fprintf(os.Stderr, "Warning: could not read keychain: %v\n", err)
		}
	}
	if url == "http://localhost:4001" {
		if u, err := keychain.GetAPIURL(); err == nil && u != "" {
			url = u
		} else if err != nil && !keychain.IsNotFound(err) {
			fmt.Fprintf(os.Stderr, "Warning: could not read server URL from keychain: %v\n", err)
		}
	}
	return apiclient.New(url, token)
}

func addVaultFlag(cmd *cobra.Command) *string {
	v := new(string)
	cmd.Flags().StringVar(v, "vault", envOrDefault("KNOW_VAULT", "default"), "vault name (env: KNOW_VAULT)")
	if err := cmd.RegisterFlagCompletionFunc("vault", completeVaultNames()); err != nil {
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
	defer func() {
		if globalLogClean == nil {
			return
		}
		if err := globalLogClean(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to close log file: %v\n", err)
		}
	}()

	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(mvCmd)
	rootCmd.AddCommand(infoCmd)
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
	jobCmd.AddCommand(reprocessCmd)
	rootCmd.AddCommand(jobCmd)
	rootCmd.AddCommand(fetchCmd)
	rootCmd.AddCommand(backupCmd)
	rootCmd.AddCommand(restoreCmd)
	rootCmd.AddCommand(completionCmd)
	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(adminCmd)
	rootCmd.AddCommand(devCmd)

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
