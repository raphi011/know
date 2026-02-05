package cli

import (
	"context"
	"fmt"
	"time"

	"charm.land/bubbles/v2/progress"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/raphaelgruber/memcp-go/internal/client"
)

const pollInterval = time.Second

// Theme holds the color scheme for the progress display.
type Theme struct {
	Status    lipgloss.Color
	Success   lipgloss.Color
	Error     lipgloss.Color
	Hint      lipgloss.Color
	ProgressBg lipgloss.Color
}

// defaultTheme provides default colors.
var defaultTheme = Theme{
	Status:    lipgloss.Color("#5FAFD7"), // light blue
	Success:   lipgloss.Color("#00D787"), // green
	Error:     lipgloss.Color("#FF005F"), // red
	Hint:      lipgloss.Color("#6C6C6C"), // dim gray
	ProgressBg: lipgloss.Color("#3A3A3A"), // dark gray
}

// Style functions for dynamic theming
func (t Theme) statusStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(t.Status)
}

func (t Theme) completedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(t.Success).Bold(true)
}

func (t Theme) errorStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(t.Error).Bold(true)
}

func (t Theme) hintStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(t.Hint).Italic(true)
}

// tickMsg triggers polling the job status
type tickMsg time.Time

// jobUpdateMsg carries the updated job data
type jobUpdateMsg struct {
	job *client.Job
	err error
}

// progressModel is the bubbletea model for job progress.
type progressModel struct {
	client   *client.Client
	jobID    string
	job      *client.Job
	progress progress.Model
	theme    Theme
	done     bool
	quitting bool
	err      error
}

// newProgressModel creates a new progress model.
func newProgressModel(c *client.Client, job *client.Job) progressModel {
	// Create progress bar with color blend
	prog := progress.New(
		progress.WithDefaultBlend(),
		progress.WithWidth(40),
	)

	return progressModel{
		client:   c,
		jobID:    job.ID,
		job:      job,
		progress: prog,
		theme:    defaultTheme,
	}
}

// Init returns the initial command (start polling).
func (m progressModel) Init() tea.Cmd {
	return tea.Batch(
		tickCmd(),
		m.progress.Init(),
	)
}

// Update handles messages and returns the updated model.
func (m progressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		}

	case tickMsg:
		// Fetch job status
		return m, m.fetchJob()

	case jobUpdateMsg:
		if msg.err != nil {
			m.err = fmt.Errorf("failed to fetch job status: %w", msg.err)
			m.done = true
			return m, tea.Quit
		}

		m.job = msg.job

		// Check for terminal states
		switch m.job.Status {
		case "completed":
			m.done = true
			return m, tea.Quit
		case "failed":
			m.done = true
			if m.job.Error != nil {
				m.err = fmt.Errorf("%s", *m.job.Error)
			} else {
				m.err = fmt.Errorf("job failed with unknown error")
			}
			return m, tea.Quit
		}

		// Continue polling for running jobs
		return m, tickCmd()

	case progress.FrameMsg:
		// Update progress bar animation
		var cmd tea.Cmd
		m.progress, cmd = m.progress.Update(msg)
		return m, cmd
	}

	return m, nil
}

// View renders the progress display.
func (m progressModel) View() tea.View {
	return tea.NewView(m.renderContent())
}

// renderContent builds the display string.
func (m progressModel) renderContent() string {
	if m.done {
		return m.finalView()
	}

	if m.job == nil {
		return "Loading job status...\n"
	}

	// Calculate progress percentage
	var pct float64
	if m.job.Total > 0 {
		pct = float64(m.job.Progress) / float64(m.job.Total)
	}

	// Status line with color
	status := m.theme.statusStyle().Render(fmt.Sprintf("[%s]", m.job.Status))

	// Progress bar with counts
	progressBar := m.progress.ViewAs(pct)
	counts := fmt.Sprintf("%d/%d files", m.job.Progress, m.job.Total)

	// Hint about background operation
	hint := m.theme.hintStyle().Render("Press Ctrl+C to continue in background")

	return fmt.Sprintf("%s %s %s\n%s\n", status, progressBar, counts, hint)
}

// finalView renders the completion message.
func (m progressModel) finalView() string {
	if m.quitting {
		msg := fmt.Sprintf("\nJob %s continues in background.\nUse 'knowhow jobs %s' to check status.\n",
			m.jobID, m.jobID)
		return m.theme.hintStyle().Render(msg)
	}

	if m.err != nil {
		return m.theme.errorStyle().Render(fmt.Sprintf("\n✗ Job failed: %s\n", m.err))
	}

	// Success with results
	if m.job != nil && m.job.Result != nil {
		r := m.job.Result
		var output string
		output += m.theme.completedStyle().Render("✓ Completed") + "\n\n"
		output += fmt.Sprintf("  Files processed:   %d\n", r.FilesProcessed)
		output += fmt.Sprintf("  Entities created:  %d\n", r.EntitiesCreated)
		output += fmt.Sprintf("  Chunks created:    %d\n", r.ChunksCreated)
		if r.RelationsCreated > 0 {
			output += fmt.Sprintf("  Relations created: %d\n", r.RelationsCreated)
		}
		if len(r.Errors) > 0 {
			output += m.theme.errorStyle().Render(fmt.Sprintf("\nWarnings (%d):\n", len(r.Errors)))
			for _, e := range r.Errors {
				output += fmt.Sprintf("  • %s\n", e)
			}
		}
		return output
	}

	return m.theme.completedStyle().Render("✓ Completed\n")
}

// fetchJob fetches the current job status from the server.
// Runs in a separate goroutine (command) to avoid blocking Update().
func (m progressModel) fetchJob() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		job, err := m.client.GetJob(ctx, m.jobID)
		return jobUpdateMsg{job: job, err: err}
	}
}

// tickCmd returns a command that sends a tick after the poll interval.
func tickCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// RunJobProgress runs the interactive progress UI for a job.
// Returns nil on success or Ctrl+C (background), error on job failure.
//
// Example usage:
//
//	job, err := client.IngestDirectoryAsync(ctx, path, opts)
//	if err != nil {
//	    return err
//	}
//	return RunJobProgress(client, job)
func RunJobProgress(c *client.Client, job *client.Job) error {
	model := newProgressModel(c, job)
	p := tea.NewProgram(model)

	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("progress UI error: %w", err)
	}

	// Check final state
	if m, ok := finalModel.(progressModel); ok {
		// If user quit with Ctrl+C, job continues in background - not an error
		if m.quitting {
			return nil
		}
		// If job failed, return the error
		if m.err != nil {
			return m.err
		}
	}

	return nil
}
