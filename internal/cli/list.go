package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var (
	listType   string
	listLabels []string
	listLimit  int
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List entities, labels, or types",
	Long: `List entities in the knowledge base with optional filtering.

Subcommands:
  entities  List entities (default)
  labels    List all labels with counts
  types     List all entity types with counts

Examples:
  knowhow list
  knowhow list --type person
  knowhow list --labels "work,banking"
  knowhow list labels
  knowhow list types`,
	RunE: runList,
}

var listEntitiesCmd = &cobra.Command{
	Use:   "entities",
	Short: "List entities",
	RunE:  runListEntities,
}

var listLabelsCmd = &cobra.Command{
	Use:   "labels",
	Short: "List all labels with counts",
	RunE:  runListLabels,
}

var listTypesCmd = &cobra.Command{
	Use:   "types",
	Short: "List all entity types with counts",
	RunE:  runListTypes,
}

func init() {
	listCmd.Flags().StringVarP(&listType, "type", "t", "", "filter by entity type")
	listCmd.Flags().StringSliceVarP(&listLabels, "labels", "l", nil, "filter by labels")
	listCmd.Flags().IntVarP(&listLimit, "limit", "n", 50, "max results")

	listEntitiesCmd.Flags().StringVarP(&listType, "type", "t", "", "filter by entity type")
	listEntitiesCmd.Flags().StringSliceVarP(&listLabels, "labels", "l", nil, "filter by labels")
	listEntitiesCmd.Flags().IntVarP(&listLimit, "limit", "n", 50, "max results")

	listCmd.AddCommand(listEntitiesCmd)
	listCmd.AddCommand(listLabelsCmd)
	listCmd.AddCommand(listTypesCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	// If no subcommand, run entities
	return runListEntities(cmd, args)
}

func runListEntities(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	entities, err := dbClient.ListEntities(ctx, listType, listLabels, listLimit)
	if err != nil {
		return fmt.Errorf("list entities: %w", err)
	}

	if len(entities) == 0 {
		fmt.Println("No entities found.")
		return nil
	}

	fmt.Printf("Entities (%d):\n\n", len(entities))
	for _, entity := range entities {
		verifiedMark := ""
		if entity.Verified {
			verifiedMark = " [verified]"
		}
		fmt.Printf("- %s [%s]%s\n", entity.Name, entity.Type, verifiedMark)
		if verbose {
			if entity.Summary != nil && *entity.Summary != "" {
				fmt.Printf("  %s\n", *entity.Summary)
			}
			if len(entity.Labels) > 0 {
				fmt.Printf("  Labels: %v\n", entity.Labels)
			}
		}
	}

	return nil
}

func runListLabels(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	labels, err := dbClient.ListLabels(ctx)
	if err != nil {
		return fmt.Errorf("list labels: %w", err)
	}

	if len(labels) == 0 {
		fmt.Println("No labels found.")
		return nil
	}

	fmt.Printf("Labels (%d):\n\n", len(labels))
	for _, l := range labels {
		fmt.Printf("- %s (%d)\n", l.Label, l.Count)
	}

	return nil
}

func runListTypes(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	types, err := dbClient.ListTypes(ctx)
	if err != nil {
		return fmt.Errorf("list types: %w", err)
	}

	if len(types) == 0 {
		fmt.Println("No entity types found.")
		return nil
	}

	fmt.Printf("Types (%d):\n\n", len(types))
	for _, t := range types {
		fmt.Printf("- %s (%d)\n", t.Type, t.Count)
	}

	return nil
}
