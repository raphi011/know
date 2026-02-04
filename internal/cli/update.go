package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/raphaelgruber/memcp-go/internal/models"
	"github.com/spf13/cobra"
)

var (
	updateContent    string
	updateContentFile string
	updateSummary    string
	updateLabels     string // "add:label1,label2" or "remove:label1" or "set:label1,label2"
	updateVerified   bool
	updateSetVerified bool
)

var updateCmd = &cobra.Command{
	Use:   "update <entity>",
	Short: "Update an existing entity",
	Long: `Update an existing entity's content, labels, or verification status.

Entity can be specified by ID or name.

Examples:
  knowhow update "auth-service" --content "New documentation..."
  knowhow update "john-doe" --labels "add:senior,promoted"
  knowhow update "auth-service" --labels "remove:deprecated"
  knowhow update "auth-service" --verified
  knowhow update "concept-123" --content-file ./updated.md`,
	Args: cobra.ExactArgs(1),
	RunE: runUpdate,
}

func init() {
	updateCmd.Flags().StringVarP(&updateContent, "content", "c", "", "new content")
	updateCmd.Flags().StringVar(&updateContentFile, "content-file", "", "read new content from file")
	updateCmd.Flags().StringVarP(&updateSummary, "summary", "s", "", "new summary")
	updateCmd.Flags().StringVarP(&updateLabels, "labels", "l", "", "label changes: add:x,y / remove:x,y / set:x,y")
	updateCmd.Flags().BoolVar(&updateVerified, "verified", false, "mark as verified")
	updateCmd.Flags().BoolVar(&updateSetVerified, "set-verified", false, "explicitly set verified flag")
}

func runUpdate(cmd *cobra.Command, args []string) error {
	entityRef := args[0]
	ctx := context.Background()

	// Get services (with LLM for re-embedding if content changes)
	entitySvc, _, _, err := getServices(ctx, true)
	if err != nil {
		return fmt.Errorf("init services: %w", err)
	}

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

	// Build update
	update := models.EntityUpdate{}
	hasUpdate := false

	// Content from file or flag
	if updateContentFile != "" {
		data, err := os.ReadFile(updateContentFile)
		if err != nil {
			return fmt.Errorf("read content file: %w", err)
		}
		content := string(data)
		update.Content = &content
		hasUpdate = true
	} else if updateContent != "" {
		update.Content = &updateContent
		hasUpdate = true
	}

	// Summary
	if updateSummary != "" {
		update.Summary = &updateSummary
		hasUpdate = true
	}

	// Labels
	if updateLabels != "" {
		if strings.HasPrefix(updateLabels, "add:") {
			labels := strings.Split(strings.TrimPrefix(updateLabels, "add:"), ",")
			update.AddLabels = labels
			hasUpdate = true
		} else if strings.HasPrefix(updateLabels, "remove:") {
			labels := strings.Split(strings.TrimPrefix(updateLabels, "remove:"), ",")
			update.DelLabels = labels
			hasUpdate = true
		} else if strings.HasPrefix(updateLabels, "set:") {
			labels := strings.Split(strings.TrimPrefix(updateLabels, "set:"), ",")
			update.Labels = labels
			hasUpdate = true
		} else {
			return fmt.Errorf("invalid labels format: use add:x,y, remove:x,y, or set:x,y")
		}
	}

	// Verified
	if updateSetVerified || updateVerified {
		update.Verified = &updateVerified
		hasUpdate = true
	}

	if !hasUpdate {
		fmt.Println("No updates specified.")
		return nil
	}

	// Apply update (service handles re-embedding if content changed)
	entityID, err := models.RecordIDString(entity.ID)
	if err != nil {
		return fmt.Errorf("get entity ID: %w", err)
	}
	updated, err := entitySvc.Update(ctx, entityID, update)
	if err != nil {
		return fmt.Errorf("update entity: %w", err)
	}

	fmt.Printf("Updated entity: %s\n", updated.Name)
	if verbose {
		fmt.Printf("  Type: %s\n", updated.Type)
		fmt.Printf("  Labels: %v\n", updated.Labels)
		fmt.Printf("  Verified: %v\n", updated.Verified)
	}

	return nil
}
