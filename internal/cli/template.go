package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/raphaelgruber/memcp-go/internal/models"
	"github.com/spf13/cobra"
)

var templateCmd = &cobra.Command{
	Use:   "template",
	Short: "Manage output templates",
	Long: `Manage output templates for structured knowledge synthesis.

Subcommands:
  list    List all templates
  show    Show template content
  add     Add a new template from a file
  delete  Delete a template
  init    Initialize default templates

Examples:
  knowhow template list
  knowhow template show "Peer Review"
  knowhow template add ./my-template.md --name "My Template"
  knowhow template delete "Old Template"
  knowhow template init`,
}

var templateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all templates",
	RunE:  runTemplateList,
}

var templateShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show template content",
	Args:  cobra.ExactArgs(1),
	RunE:  runTemplateShow,
}

var templateAddCmd = &cobra.Command{
	Use:   "add <file>",
	Short: "Add a new template from a file",
	Args:  cobra.ExactArgs(1),
	RunE:  runTemplateAdd,
}

var templateDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a template",
	Args:  cobra.ExactArgs(1),
	RunE:  runTemplateDelete,
}

var templateInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize default templates",
	RunE:  runTemplateInit,
}

var (
	templateName        string
	templateDescription string
)

func init() {
	templateAddCmd.Flags().StringVarP(&templateName, "name", "n", "", "template name (required)")
	templateAddCmd.Flags().StringVarP(&templateDescription, "description", "d", "", "template description")
	templateAddCmd.MarkFlagRequired("name")

	templateCmd.AddCommand(templateListCmd)
	templateCmd.AddCommand(templateShowCmd)
	templateCmd.AddCommand(templateAddCmd)
	templateCmd.AddCommand(templateDeleteCmd)
	templateCmd.AddCommand(templateInitCmd)
}

func runTemplateList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	templates, err := gqlClient.ListTemplates(ctx)
	if err != nil {
		return fmt.Errorf("list templates: %w", err)
	}

	if len(templates) == 0 {
		fmt.Println("No templates found. Run 'knowhow template init' to create defaults.")
		return nil
	}

	fmt.Printf("Templates (%d):\n\n", len(templates))
	for _, t := range templates {
		desc := ""
		if t.Description != nil {
			desc = fmt.Sprintf(" - %s", *t.Description)
		}
		fmt.Printf("- %s%s\n", t.Name, desc)
	}

	return nil
}

func runTemplateShow(cmd *cobra.Command, args []string) error {
	name := args[0]
	ctx := context.Background()

	template, err := gqlClient.GetTemplate(ctx, name)
	if err != nil {
		return fmt.Errorf("get template: %w", err)
	}
	if template == nil {
		return fmt.Errorf("template not found: %s", name)
	}

	fmt.Printf("# %s\n", template.Name)
	if template.Description != nil {
		fmt.Printf("%s\n", *template.Description)
	}
	fmt.Printf("\n---\n\n")
	fmt.Println(template.Content)

	return nil
}

func runTemplateAdd(cmd *cobra.Command, args []string) error {
	filePath := args[0]
	ctx := context.Background()

	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	// Create template
	var desc *string
	if templateDescription != "" {
		desc = &templateDescription
	}

	template, err := gqlClient.CreateTemplate(ctx, templateName, desc, string(content))
	if err != nil {
		return fmt.Errorf("create template: %w", err)
	}

	fmt.Printf("Created template: %s\n", template.Name)
	return nil
}

func runTemplateDelete(cmd *cobra.Command, args []string) error {
	name := args[0]
	ctx := context.Background()

	deleted, err := gqlClient.DeleteTemplate(ctx, name)
	if err != nil {
		return fmt.Errorf("delete template: %w", err)
	}
	if !deleted {
		return fmt.Errorf("template not found: %s", name)
	}

	fmt.Printf("Deleted template: %s\n", name)
	return nil
}

func runTemplateInit(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	defaults := models.DefaultTemplates()
	created := 0

	for _, t := range defaults {
		// Check if already exists
		existing, err := gqlClient.GetTemplate(ctx, t.Name)
		if err != nil {
			slog.Warn("failed to check existing template", "name", t.Name, "error", err)
		}
		if existing != nil {
			if verbose {
				fmt.Printf("  Skipping existing: %s\n", t.Name)
			}
			continue
		}

		_, err = gqlClient.CreateTemplate(ctx, t.Name, t.Description, t.Content)
		if err != nil {
			fmt.Printf("Warning: failed to create %s: %v\n", t.Name, err)
			continue
		}
		created++
		fmt.Printf("  Created: %s\n", t.Name)
	}

	fmt.Printf("\nInitialized %d default templates.\n", created)
	return nil
}
