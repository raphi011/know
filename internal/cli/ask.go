package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/raphaelgruber/memcp-go/internal/service"
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

	// Get services (with LLM for embedding and synthesis)
	_, searchSvc, _, err := getServices(ctx, true)
	if err != nil {
		return fmt.Errorf("init services: %w", err)
	}

	opts := service.SearchOptions{
		Labels:       askLabels,
		Types:        askTypes,
		VerifiedOnly: askVerified,
		Limit:        askLimit,
	}

	var answer string
	if askTemplate != "" {
		// Use template for structured output
		answer, err = searchSvc.AskWithTemplate(ctx, query, askTemplate, opts)
	} else {
		// Free-form answer synthesis
		answer, err = searchSvc.Ask(ctx, query, opts)
	}
	if err != nil {
		return fmt.Errorf("ask: %w", err)
	}

	// Output the answer
	if askOutputFile != "" {
		if err := os.WriteFile(askOutputFile, []byte(answer), 0644); err != nil {
			return fmt.Errorf("write output file: %w", err)
		}
		fmt.Printf("Answer written to %s\n", askOutputFile)
	} else {
		fmt.Println(answer)
	}

	return nil
}
