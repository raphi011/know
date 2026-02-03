package cli

import (
	"context"
	"fmt"

	"github.com/raphaelgruber/memcp-go/internal/models"
	"github.com/spf13/cobra"
)

var (
	linkType     string
	linkStrength float64
)

var linkCmd = &cobra.Command{
	Use:   "link <from> <to>",
	Short: "Create a relation between two entities",
	Long: `Create a relation between two entities.

Use --type to specify the relationship type (e.g., "works_on", "depends_on").

Examples:
  knowhow link "john-doe" "auth-service" --type "works_on"
  knowhow link "auth-service" "user-service" --type "depends_on"
  knowhow link "meeting-notes" "auth-bug" --type "references"`,
	Args: cobra.ExactArgs(2),
	RunE: runLink,
}

func init() {
	linkCmd.Flags().StringVarP(&linkType, "type", "t", "relates_to", "relationship type")
	linkCmd.Flags().Float64Var(&linkStrength, "strength", 1.0, "relationship strength (0-1)")
}

func runLink(cmd *cobra.Command, args []string) error {
	fromID, toID := args[0], args[1]
	ctx := context.Background()

	// Verify both entities exist
	from, err := dbClient.GetEntity(ctx, fromID)
	if err != nil {
		return fmt.Errorf("get source entity: %w", err)
	}
	if from == nil {
		// Try by name
		from, err = dbClient.GetEntityByName(ctx, fromID)
		if err != nil {
			return fmt.Errorf("get source entity by name: %w", err)
		}
		if from == nil {
			return fmt.Errorf("source entity not found: %s", fromID)
		}
		fromID = from.ID.ID.(string)
	}

	to, err := dbClient.GetEntity(ctx, toID)
	if err != nil {
		return fmt.Errorf("get target entity: %w", err)
	}
	if to == nil {
		// Try by name
		to, err = dbClient.GetEntityByName(ctx, toID)
		if err != nil {
			return fmt.Errorf("get target entity by name: %w", err)
		}
		if to == nil {
			return fmt.Errorf("target entity not found: %s", toID)
		}
		toID = to.ID.ID.(string)
	}

	// Create relation
	err = dbClient.CreateRelation(ctx, models.RelationInput{
		FromID:   fromID,
		ToID:     toID,
		RelType:  linkType,
		Strength: &linkStrength,
	})
	if err != nil {
		return fmt.Errorf("create relation: %w", err)
	}

	fmt.Printf("Created relation: %s -[%s]-> %s\n", from.Name, linkType, to.Name)
	return nil
}
