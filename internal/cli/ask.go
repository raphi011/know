package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/raphaelgruber/memcp-go/internal/client"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	askTemplate   string
	askLabels     []string
	askTypes      []string
	askVerified   bool
	askLimit      int
	askOutputFile string
	askNoStream   bool
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
	askCmd.Flags().BoolVar(&askNoStream, "no-stream", false, "disable streaming output")
}

func runAsk(cmd *cobra.Command, args []string) error {
	query := args[0]
	ctx := context.Background()

	opts := &client.SearchOptions{
		Query:        query,
		Labels:       askLabels,
		Types:        askTypes,
		VerifiedOnly: &askVerified,
		Limit:        &askLimit,
	}

	var templateName *string
	if askTemplate != "" {
		templateName = &askTemplate
	}

	// Auto-detect: stream unless writing to file, not a TTY, or explicitly disabled
	// Templates don't support streaming yet
	shouldStream := !askNoStream &&
		askOutputFile == "" &&
		askTemplate == "" &&
		term.IsTerminal(int(os.Stdout.Fd()))

	if shouldStream {
		// Streaming mode - tokens printed as they arrive
		var fullAnswer strings.Builder
		err := gqlClient.AskStream(ctx, query, opts, templateName, func(token string) error {
			fmt.Print(token)
			fullAnswer.WriteString(token)
			return nil
		})
		fmt.Println() // Final newline after stream completes

		if err != nil {
			return fmt.Errorf("ask stream: %w", err)
		}
		return nil
	}

	// Non-streaming mode - wait for complete response
	answer, err := gqlClient.Ask(ctx, query, opts, templateName)
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
