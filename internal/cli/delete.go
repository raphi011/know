package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var (
	deleteForce bool
)

var deleteCmd = &cobra.Command{
	Use:   "delete <entity>",
	Short: "Delete an entity from the knowledge base",
	Long: `Delete an entity from the knowledge base.

This will also delete associated chunks and relations (cascade delete).
Requires confirmation unless --force is used.

Examples:
  knowhow delete "auth-service"
  knowhow delete "old-notes" --force`,
	Args: cobra.ExactArgs(1),
	RunE: runDelete,
}

func init() {
	deleteCmd.Flags().BoolVarP(&deleteForce, "force", "f", false, "skip confirmation")
}

func runDelete(cmd *cobra.Command, args []string) error {
	entityRef := args[0]
	ctx := context.Background()

	// Find entity
	entity, err := dbClient.GetEntity(ctx, entityRef)
	if err != nil {
		return fmt.Errorf("get entity: %w", err)
	}
	if entity == nil {
		entity, err = dbClient.GetEntityByName(ctx, entityRef)
		if err != nil {
			return fmt.Errorf("get entity by name: %w", err)
		}
		if entity == nil {
			return fmt.Errorf("entity not found: %s", entityRef)
		}
	}

	entityID := entity.ID.ID.(string)

	// Get related counts for warning
	chunks, err := dbClient.GetChunks(ctx, entityID)
	if err != nil {
		return fmt.Errorf("get chunks: %w", err)
	}
	relations, err := dbClient.GetRelations(ctx, entityID)
	if err != nil {
		return fmt.Errorf("get relations: %w", err)
	}

	// Confirm deletion
	if !deleteForce {
		fmt.Printf("About to delete: %s (%s)\n", entity.Name, entityID)
		if len(chunks) > 0 {
			fmt.Printf("  - %d chunks will be deleted\n", len(chunks))
		}
		if len(relations) > 0 {
			fmt.Printf("  - %d relations will be deleted\n", len(relations))
		}
		fmt.Print("\nContinue? [y/N]: ")

		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		if response != "y" && response != "yes" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// Delete entity
	deleted, err := dbClient.DeleteEntity(ctx, entityID)
	if err != nil {
		return fmt.Errorf("delete entity: %w", err)
	}
	if !deleted {
		return fmt.Errorf("entity not found or already deleted")
	}

	fmt.Printf("Deleted: %s\n", entity.Name)
	return nil
}
