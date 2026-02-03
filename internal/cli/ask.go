package cli

import (
	"context"
	"fmt"

	"github.com/raphaelgruber/memcp-go/internal/db"
	"github.com/spf13/cobra"
)

var (
	askTemplate    string
	askLabels      []string
	askTypes       []string
	askVerified    bool
	askLimit       int
	askOutputFile  string
)

var askCmd = &cobra.Command{
	Use:   "ask <query>",
	Short: "Ask a question and get an LLM-synthesized answer",
	Long: `Ask a question about your knowledge base and get an LLM-synthesized answer.

Uses hybrid search to find relevant entities and chunks, then synthesizes
an answer using the configured LLM.

Optionally use --template to format the response using a predefined template.

Examples:
  knowhow ask "What do I know about John Doe?"
  knowhow ask "How does the auth service work?"
  knowhow ask "John Doe" --template "Peer Review"
  knowhow ask "auth-service" --template "Service Summary" -o summary.md`,
	Args: cobra.ExactArgs(1),
	RunE: runAsk,
}

func init() {
	askCmd.Flags().StringVar(&askTemplate, "template", "", "use a template for structured output")
	askCmd.Flags().StringSliceVarP(&askLabels, "labels", "l", nil, "filter by labels")
	askCmd.Flags().StringSliceVarP(&askTypes, "type", "t", nil, "filter by entity types")
	askCmd.Flags().BoolVar(&askVerified, "verified", false, "only use verified knowledge")
	askCmd.Flags().IntVarP(&askLimit, "limit", "n", 20, "max context entities")
	askCmd.Flags().StringVarP(&askOutputFile, "output", "o", "", "write output to file")
}

func runAsk(cmd *cobra.Command, args []string) error {
	query := args[0]
	ctx := context.Background()

	// TODO: Generate embedding with LLM service

	opts := db.SearchOptions{
		Query:        query,
		Labels:       askLabels,
		Types:        askTypes,
		VerifiedOnly: askVerified,
		Limit:        askLimit,
	}

	results, err := dbClient.SearchWithChunks(ctx, opts)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}

	if len(results) == 0 {
		fmt.Println("No relevant knowledge found for this query.")
		return nil
	}

	// TODO: If template specified, fetch it and use for structured output
	// TODO: Synthesize answer using LLM service

	// For now, just show what we found
	fmt.Printf("Found %d relevant entities. LLM synthesis not yet implemented.\n\n", len(results))
	for i, result := range results {
		if i >= 5 {
			fmt.Printf("... and %d more\n", len(results)-5)
			break
		}
		fmt.Printf("- %s [%s]\n", result.Name, result.Type)
		if len(result.MatchedChunks) > 0 {
			for _, chunk := range result.MatchedChunks {
				if chunk.HeadingPath != nil {
					fmt.Printf("  @ %s\n", *chunk.HeadingPath)
				}
			}
		}
	}

	return nil
}
