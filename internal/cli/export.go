package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	exportType     string
	exportLabels   []string
	exportVerified bool
	exportEntity   string
)

var exportCmd = &cobra.Command{
	Use:   "export <path>",
	Short: "Export knowledge base to Markdown files",
	Long: `Export the knowledge base to Markdown files for backup or migration.

Creates a directory structure with entities organized by type,
preserving all metadata in frontmatter.

Examples:
  knowhow export ./backup
  knowhow export ./backup --type document
  knowhow export ./backup --labels "work,banking"
  knowhow export ./backup --verified-only
  knowhow export ./backup --entity "auth-service"`,
	Args: cobra.ExactArgs(1),
	RunE: runExport,
}

func init() {
	exportCmd.Flags().StringVarP(&exportType, "type", "t", "", "export only this entity type")
	exportCmd.Flags().StringSliceVarP(&exportLabels, "labels", "l", nil, "export entities with these labels")
	exportCmd.Flags().BoolVar(&exportVerified, "verified-only", false, "export only verified entities")
	exportCmd.Flags().StringVar(&exportEntity, "entity", "", "export specific entity")
}

func runExport(cmd *cobra.Command, args []string) error {
	exportPath := args[0]
	ctx := context.Background()

	// Create export directory
	if err := os.MkdirAll(exportPath, 0755); err != nil {
		return fmt.Errorf("create export directory: %w", err)
	}

	// Get entities to export
	entities, err := dbClient.ListEntities(ctx, exportType, exportLabels, 0) // 0 = no limit
	if err != nil {
		return fmt.Errorf("list entities: %w", err)
	}

	if len(entities) == 0 {
		fmt.Println("No entities to export.")
		return nil
	}

	// Filter by specific entity if requested
	if exportEntity != "" {
		filtered := entities[:0]
		for _, e := range entities {
			id := e.ID.ID.(string)
			if id == exportEntity || e.Name == exportEntity {
				filtered = append(filtered, e)
			}
		}
		entities = filtered
	}

	// Filter by verified if requested
	if exportVerified {
		filtered := entities[:0]
		for _, e := range entities {
			if e.Verified {
				filtered = append(filtered, e)
			}
		}
		entities = filtered
	}

	fmt.Printf("Exporting %d entities...\n", len(entities))

	// Export each entity
	exported := 0
	for _, entity := range entities {
		// Create type directory
		typeDir := filepath.Join(exportPath, "entities", entity.Type)
		if err := os.MkdirAll(typeDir, 0755); err != nil {
			return fmt.Errorf("create type directory: %w", err)
		}

		// Generate filename from ID
		id := entity.ID.ID.(string)
		filename := filepath.Join(typeDir, id+".md")

		// Build frontmatter
		frontmatter := fmt.Sprintf(`---
id: %s
type: %s
name: %s
labels: %v
verified: %v
confidence: %.2f
source: %s
created_at: %s
updated_at: %s
`, id, entity.Type, entity.Name, entity.Labels, entity.Verified,
			entity.Confidence, entity.Source, entity.CreatedAt, entity.UpdatedAt)

		if entity.SourcePath != nil {
			frontmatter += fmt.Sprintf("source_path: %s\n", *entity.SourcePath)
		}
		frontmatter += "---\n\n"

		// Build content
		content := frontmatter
		content += fmt.Sprintf("# %s\n\n", entity.Name)
		if entity.Summary != nil && *entity.Summary != "" {
			content += fmt.Sprintf("%s\n\n", *entity.Summary)
		}
		if entity.Content != nil {
			content += *entity.Content
		}

		// Write file
		if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
			fmt.Printf("Warning: failed to write %s: %v\n", filename, err)
			continue
		}
		exported++

		if verbose {
			fmt.Printf("  Exported: %s\n", filename)
		}
	}

	fmt.Printf("\nExported %d entities to %s\n", exported, exportPath)
	return nil
}
