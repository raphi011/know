// Package cli provides the command-line interface for knowhow.
package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/raphaelgruber/memcp-go/internal/config"
	"github.com/raphaelgruber/memcp-go/internal/db"
	"github.com/raphaelgruber/memcp-go/internal/llm"
	"github.com/raphaelgruber/memcp-go/internal/service"
	"github.com/spf13/cobra"
)

var (
	// Version is set at build time.
	Version = "0.1.0"

	// Global flags
	verbose bool

	// Global config and db client
	cfg      config.Config
	dbClient *db.Client

	// Lazy-initialized LLM components
	embedder *llm.Embedder
	model    *llm.Model
)

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "knowhow",
	Short: "Personal knowledge RAG database",
	Long: `Knowhow is a personal knowledge RAG database - like Obsidian / second brain
but searchable, indexable, and AI-augmented.

Store any type of knowledge (people, services, concepts, documents) with
flexible schemas, Markdown templates, and semantic search.`,
	Version: Version,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip DB connection for version and help commands
		if cmd.Name() == "version" || cmd.Name() == "help" {
			return nil
		}

		// Load config
		cfg = config.Load()

		// Connect to database
		ctx := context.Background()
		dbCfg := db.Config{
			URL:       cfg.SurrealDBURL,
			Namespace: cfg.SurrealDBNamespace,
			Database:  cfg.SurrealDBDatabase,
			Username:  cfg.SurrealDBUser,
			Password:  cfg.SurrealDBPass,
			AuthLevel: cfg.SurrealDBAuthLevel,
		}

		var err error
		dbClient, err = db.NewClient(ctx, dbCfg, nil)
		if err != nil {
			return fmt.Errorf("connect to database: %w", err)
		}

		// Initialize schema
		if err := dbClient.InitSchema(ctx); err != nil {
			return fmt.Errorf("initialize schema: %w", err)
		}

		return nil
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		// Close database connection
		if dbClient != nil {
			if err := dbClient.Close(context.Background()); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to close database: %v\n", err)
			}
		}
	},
}

// getServices creates services with lazy LLM initialization.
// Commands that need embeddings pass requireLLM=true.
func getServices(ctx context.Context, requireLLM bool) (*service.EntityService, *service.SearchService, *service.IngestService, error) {
	if requireLLM && embedder == nil {
		var err error
		embedder, err = llm.NewEmbedder(cfg)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("init embedder: %w", err)
		}
		model, err = llm.NewModel(cfg)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("init model: %w", err)
		}
	}

	return service.NewEntityService(dbClient, embedder, model),
		service.NewSearchService(dbClient, embedder, model),
		service.NewIngestService(dbClient, embedder, model), nil
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
