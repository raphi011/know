package cli

import (
	"context"
	"fmt"

	"github.com/raphaelgruber/memcp-go/internal/service"
	"github.com/spf13/cobra"
)

var (
	searchLabels   []string
	searchTypes    []string
	searchVerified bool
	searchLimit    int
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search the knowledge base without LLM synthesis",
	Long: `Search the knowledge base using hybrid BM25 + vector search.

Returns matching entities ranked by relevance without LLM synthesis.
Use 'ask' command for LLM-augmented responses.

Examples:
  knowhow search "authentication"
  knowhow search "token refresh" --labels "work,auth-service"
  knowhow search "senior engineer" --type person
  knowhow search "kubernetes" --verified`,
	Args: cobra.ExactArgs(1),
	RunE: runSearch,
}

func init() {
	searchCmd.Flags().StringSliceVarP(&searchLabels, "labels", "l", nil, "filter by labels")
	searchCmd.Flags().StringSliceVarP(&searchTypes, "type", "t", nil, "filter by entity types")
	searchCmd.Flags().BoolVar(&searchVerified, "verified", false, "only return verified entities")
	searchCmd.Flags().IntVarP(&searchLimit, "limit", "n", 10, "max results")
}

func runSearch(cmd *cobra.Command, args []string) error {
	query := args[0]
	ctx := context.Background()

	// Get services (with LLM for query embedding)
	_, searchSvc, _, err := getServices(ctx, true)
	if err != nil {
		return fmt.Errorf("init services: %w", err)
	}

	opts := service.SearchOptions{
		Query:        query,
		Labels:       searchLabels,
		Types:        searchTypes,
		VerifiedOnly: searchVerified,
		Limit:        searchLimit,
	}

	results, err := searchSvc.Search(ctx, opts)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}

	if len(results) == 0 {
		fmt.Println("No results found.")
		return nil
	}

	fmt.Printf("Found %d results:\n\n", len(results))
	for i, entity := range results {
		fmt.Printf("%d. %s [%s]\n", i+1, entity.Name, entity.Type)
		if entity.Summary != nil && *entity.Summary != "" {
			fmt.Printf("   %s\n", *entity.Summary)
		} else if entity.Content != nil && len(*entity.Content) > 100 {
			fmt.Printf("   %s...\n", (*entity.Content)[:100])
		} else if entity.Content != nil {
			fmt.Printf("   %s\n", *entity.Content)
		}
		if verbose && len(entity.Labels) > 0 {
			fmt.Printf("   Labels: %v\n", entity.Labels)
		}
		fmt.Println()
	}

	return nil
}
