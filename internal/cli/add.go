package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/raphaelgruber/memcp-go/internal/models"
	"github.com/spf13/cobra"
)

var (
	addType     string
	addLabels   []string
	addSummary  string
	addRelatesTo []string
)

var addCmd = &cobra.Command{
	Use:   "add <content>",
	Short: "Add a new entity to the knowledge base",
	Long: `Add a new entity to the knowledge base.

The content can be a simple note, fact, or any piece of information.
Use --type to specify the entity type (concept, note, task, etc.).
Use --labels to add organizational tags.

Examples:
  knowhow add "SurrealDB supports HNSW indexes for vector search"
  knowhow add "John Doe is a senior SRE" --type person --labels "work,team-platform"
  knowhow add "Fix token refresh bug" --type task --labels "work,auth-service"
  knowhow add "Meeting notes from standup" --relates-to "john-doe:mentioned_in"`,
	Args: cobra.ExactArgs(1),
	RunE: runAdd,
}

func init() {
	addCmd.Flags().StringVarP(&addType, "type", "t", "concept", "entity type (concept, person, task, document, service)")
	addCmd.Flags().StringSliceVarP(&addLabels, "labels", "l", nil, "labels/tags for organization")
	addCmd.Flags().StringVarP(&addSummary, "summary", "s", "", "short summary (auto-generated if not provided)")
	addCmd.Flags().StringSliceVar(&addRelatesTo, "relates-to", nil, "relations in format entity:rel_type")
}

func runAdd(cmd *cobra.Command, args []string) error {
	content := args[0]

	// Use content as name if short, otherwise truncate
	name := content
	if len(name) > 50 {
		name = name[:47] + "..."
	}

	// Create entity
	ctx := context.Background()
	input := models.EntityInput{
		Type:    addType,
		Name:    name,
		Content: &content,
		Labels:  addLabels,
	}
	if addSummary != "" {
		input.Summary = &addSummary
	}

	// TODO: Generate embedding with LLM service

	entity, err := dbClient.CreateEntity(ctx, input)
	if err != nil {
		return fmt.Errorf("create entity: %w", err)
	}

	// Create relations if specified
	if len(addRelatesTo) > 0 {
		entityID := entity.ID.ID.(string)
		for _, rel := range addRelatesTo {
			parts := strings.SplitN(rel, ":", 2)
			if len(parts) != 2 {
				fmt.Printf("Warning: invalid relation format %q (expected entity:rel_type)\n", rel)
				continue
			}
			targetID, relType := parts[0], parts[1]

			err := dbClient.CreateRelation(ctx, models.RelationInput{
				FromID:  entityID,
				ToID:    targetID,
				RelType: relType,
			})
			if err != nil {
				fmt.Printf("Warning: failed to create relation to %s: %v\n", targetID, err)
			}
		}
	}

	fmt.Printf("Created entity: %s (%s)\n", entity.Name, entity.ID.ID)
	if verbose {
		fmt.Printf("  Type: %s\n", entity.Type)
		fmt.Printf("  Labels: %v\n", entity.Labels)
	}

	return nil
}
