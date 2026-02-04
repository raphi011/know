// Package cli provides the command-line interface for knowhow.
package cli

import (
	"fmt"
	"os"

	"github.com/raphaelgruber/memcp-go/internal/client"
	"github.com/spf13/cobra"
)

var (
	// Version is set at build time.
	Version = "0.1.0"

	// Global flags
	verbose bool

	// GraphQL client (initialized in PersistentPreRunE)
	gqlClient *client.Client
)

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "knowhow",
	Short: "Personal knowledge RAG database",
	Long: `Knowhow is a personal knowledge RAG database - like Obsidian / second brain
but searchable, indexable, and AI-augmented.

Store any type of knowledge (people, services, concepts, documents) with
flexible schemas, Markdown templates, and semantic search.

Note: The knowhow-server must be running for this CLI to work.
Start it with: make dev`,
	Version:      Version,
	SilenceUsage: true, // Don't print usage on errors
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip client initialization for version and help commands
		if cmd.Name() == "version" || cmd.Name() == "help" {
			return nil
		}

		// Initialize GraphQL client
		gqlClient = client.New("")

		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	// Add subcommands
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(askCmd)
	rootCmd.AddCommand(scrapeCmd)
	rootCmd.AddCommand(linkCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(usageCmd)
	rootCmd.AddCommand(templateCmd)
}

// exitWithError prints an error message and exits with code 1.
func exitWithError(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}
