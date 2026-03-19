package main

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/raphi011/know/internal/apiclient"
	"github.com/raphi011/know/internal/tui"
	"github.com/spf13/cobra"
)

var (
	taskAPI       *apiFlags
	taskVaultID   *string
	taskLabels    string
	taskStatus    string
	taskDueBefore string
	taskDueAfter  string
	taskPath      string
)

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Browse and toggle tasks interactively",
	Long: `Open an interactive TUI for browsing and toggling tasks.

Tasks are extracted from markdown checkboxes (- [ ] / - [x]) in your documents.
Use flags to filter which tasks are shown.

Keybindings:
  enter/space  Toggle task (check/uncheck)
  /            Filter tasks
  esc          Quit
  j/k, arrows  Navigate

Environment variables:
  KNOW_VAULT    vault name (alternative to --vault flag)

Examples:
  know task
  know task --labels work,urgent
  know task --status all
  know task --path /daily/
  know task --due-before 2026-03-20`,
	RunE: runTask,
}

func init() {
	taskAPI = addAPIFlags(taskCmd)
	taskVaultID = addVaultFlag(taskCmd, taskAPI)
	taskCmd.Flags().StringVar(&taskLabels, "labels", "", "filter by labels (comma-separated)")
	taskCmd.Flags().StringVar(&taskStatus, "status", "open", "filter by status: open, done, all")
	taskCmd.Flags().StringVar(&taskDueBefore, "due-before", "", "only tasks due on or before this date (YYYY-MM-DD)")
	taskCmd.Flags().StringVar(&taskDueAfter, "due-after", "", "only tasks due on or after this date (YYYY-MM-DD)")
	taskCmd.Flags().StringVar(&taskPath, "path", "", "filter by document path or folder (path ending with / matches folder prefix)")

	if err := taskCmd.RegisterFlagCompletionFunc("labels", completeLabelNames(taskAPI, taskVaultID)); err != nil {
		panic(fmt.Sprintf("register labels completion: %v", err))
	}
	if err := taskCmd.RegisterFlagCompletionFunc("status", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return []string{"open", "done", "all"}, noFileComp
	}); err != nil {
		panic(fmt.Sprintf("register status completion: %v", err))
	}
	if err := taskCmd.RegisterFlagCompletionFunc("due-before", noFileCompletions); err != nil {
		panic(fmt.Sprintf("register due-before completion: %v", err))
	}
	if err := taskCmd.RegisterFlagCompletionFunc("due-after", noFileCompletions); err != nil {
		panic(fmt.Sprintf("register due-after completion: %v", err))
	}
	if err := taskCmd.RegisterFlagCompletionFunc("path", completeVaultPaths(taskAPI, taskVaultID, pathFilterAll)); err != nil {
		panic(fmt.Sprintf("register path completion: %v", err))
	}
}

func runTask(_ *cobra.Command, _ []string) error {
	client := taskAPI.newClient()
	ctx := context.Background()

	filter := apiclient.TaskFilter{
		Status:    taskStatus,
		DueBefore: taskDueBefore,
		DueAfter:  taskDueAfter,
		Path:      taskPath,
	}
	if taskLabels != "" {
		filter.Labels = strings.Split(taskLabels, ",")
	}

	resp, err := client.ListTasks(ctx, *taskVaultID, filter)
	if err != nil {
		return fmt.Errorf("task: %w", err)
	}

	model := tui.NewTaskModel(client, *taskVaultID, filter, resp.Items)
	p := tea.NewProgram(model)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("task: %w", err)
	}

	return nil
}
