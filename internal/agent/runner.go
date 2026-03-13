package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/cloudwego/eino/adk"
	"github.com/raphi011/knowhow/internal/auth"
	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/logutil"
	"github.com/raphi011/knowhow/internal/models"
)

const defaultBufferCapacity = 1000

// Runner manages running agent goroutines, decoupled from HTTP requests.
type Runner struct {
	mu      sync.Mutex
	tasks   map[string]*runningTask // conversationID → task
	service *Service
	db      *db.Client
}

type runningTask struct {
	cancel context.CancelFunc
	events *RingBuffer[StreamEvent]
	done   chan struct{}
	userID string
	err    error // set on completion
}

// NewRunner creates a Runner that manages background agent goroutines.
func NewRunner(svc *Service, db *db.Client) *Runner {
	return &Runner{
		tasks:   make(map[string]*runningTask),
		service: svc,
		db:      db,
	}
}

// Start launches an agent goroutine for the given chat request.
// It returns the conversation ID (created if empty) and any validation error.
// The agent runs in a background context independent of the HTTP request.
func (r *Runner) Start(requestCtx context.Context, req ChatRequest) (string, error) {
	bgCtx, err := auth.DetachContext(requestCtx)
	if err != nil {
		return "", fmt.Errorf("detach auth context: %w", err)
	}

	logger := logutil.FromCtx(requestCtx).With("vault_id", req.VaultID)
	bgCtx = logutil.WithLogger(bgCtx, logger)

	// Create conversation upfront so we can return its ID
	if req.ConversationID == "" {
		conv, createErr := r.db.CreateConversation(bgCtx, req.VaultID, req.UserID)
		if createErr != nil {
			return "", fmt.Errorf("create conversation: %w", createErr)
		}
		convID, idErr := models.RecordIDString(conv.ID)
		if idErr != nil {
			return "", fmt.Errorf("extract conversation ID: %w", idErr)
		}
		req.ConversationID = convID
	}

	return r.runTask(bgCtx, req.ConversationID, req.UserID, logger, func(ctx context.Context, emit func(StreamEvent)) error {
		return r.service.Chat(ctx, req, emit)
	})
}

// ResumeRequest contains the parameters for resuming an interrupted agent.
type ResumeRequest struct {
	ConversationID string
	VaultID        string
	UserID         string
	InterruptID    string
	Response       ApprovalResponse
}

// Resume resumes an interrupted agent from its checkpoint.
// It rebuilds the agent, loads the checkpoint from SurrealDB, and continues
// execution with the user's approval response as resume data.
func (r *Runner) Resume(requestCtx context.Context, req ResumeRequest) (string, error) {
	bgCtx, err := auth.DetachContext(requestCtx)
	if err != nil {
		return "", fmt.Errorf("detach auth context: %w", err)
	}

	logger := logutil.FromCtx(requestCtx).With(
		"vault_id", req.VaultID,
		"conversation_id", req.ConversationID,
	)
	bgCtx = logutil.WithLogger(bgCtx, logger)

	return r.runTask(bgCtx, req.ConversationID, req.UserID, logger, func(ctx context.Context, emit func(StreamEvent)) error {
		return r.service.ResumeChat(ctx, req, emit)
	})
}

// runTask handles the shared lifecycle of running an agent function in a background
// goroutine: buffer setup, task registration, bg_status tracking, and cleanup.
func (r *Runner) runTask(bgCtx context.Context, convID, userID string, logger *slog.Logger, fn func(ctx context.Context, emit func(StreamEvent)) error) (string, error) {
	bgCtx, cancel := context.WithCancel(bgCtx)
	logger = logger.With("conversation_id", convID)
	bgCtx = logutil.WithLogger(bgCtx, logger)

	buf := NewRingBuffer[StreamEvent](defaultBufferCapacity)
	done := make(chan struct{})

	task := &runningTask{
		cancel: cancel,
		events: buf,
		done:   done,
		userID: userID,
	}

	r.mu.Lock()
	r.tasks[convID] = task
	r.mu.Unlock()

	if err := r.db.SetConversationBgRunning(bgCtx, convID); err != nil {
		logger.Warn("failed to set bg_status running", "error", err)
	}

	buf.Push(StreamEvent{Type: "conv_id", ConvID: convID})

	go func() {
		defer close(done)
		defer buf.Close()

		emit := func(event StreamEvent) {
			buf.Push(event)
		}

		taskErr := fn(bgCtx, emit)
		task.err = taskErr

		if taskErr != nil {
			logger.Error("agent task error", "error", taskErr)
			buf.Push(StreamEvent{Type: "error", Content: "Failed to process request. Please try again."})
			if dbErr := r.db.SetConversationBgFailed(context.Background(), convID, taskErr.Error()); dbErr != nil {
				logger.Warn("failed to set bg_status failed", "error", dbErr)
			}
		} else {
			if dbErr := r.db.SetConversationBgCompleted(context.Background(), convID); dbErr != nil {
				logger.Warn("failed to set bg_status completed", "error", dbErr)
			}
		}

		r.mu.Lock()
		delete(r.tasks, convID)
		r.mu.Unlock()
	}()

	return convID, nil
}

// Subscribe returns the event history and a live channel for a running task.
// Returns an error if the task is not found (check DB bg_status for completed tasks).
func (r *Runner) Subscribe(conversationID string) ([]StreamEvent, <-chan StreamEvent, func(), error) {
	r.mu.Lock()
	task, ok := r.tasks[conversationID]
	r.mu.Unlock()

	if !ok {
		return nil, nil, nil, fmt.Errorf("no running task for conversation %s", conversationID)
	}

	history, ch, unsub := task.events.Subscribe()
	return history, ch, unsub, nil
}

// Cancel cancels a running agent goroutine.
func (r *Runner) Cancel(conversationID string) error {
	r.mu.Lock()
	task, ok := r.tasks[conversationID]
	r.mu.Unlock()

	if !ok {
		return fmt.Errorf("no running task for conversation %s", conversationID)
	}

	task.cancel()
	return nil
}

// IsRunning returns true if an agent is currently running for the given conversation.
func (r *Runner) IsRunning(conversationID string) bool {
	r.mu.Lock()
	_, ok := r.tasks[conversationID]
	r.mu.Unlock()
	return ok
}

// Shutdown cancels all running agents and waits for them to finish.
func (r *Runner) Shutdown() {
	r.mu.Lock()
	tasks := make([]*runningTask, 0, len(r.tasks))
	for _, t := range r.tasks {
		tasks = append(tasks, t)
		t.cancel()
	}
	r.mu.Unlock()

	for _, t := range tasks {
		<-t.done
	}
}

// ResumeChat rebuilds the agent and resumes from checkpoint using the approval response.
func (s *Service) ResumeChat(ctx context.Context, req ResumeRequest, emit func(StreamEvent)) error {
	model := s.getModel()
	if model == nil {
		return fmt.Errorf("agent not available: no LLM configured")
	}

	// Build a ChatRequest for agent construction (no new user message — resuming)
	chatReq := &ChatRequest{
		ConversationID: req.ConversationID,
		VaultID:        req.VaultID,
		UserID:         req.UserID,
	}

	adkRunner, err := s.buildAgent(ctx, model, chatReq, emit)
	if err != nil {
		return fmt.Errorf("build agent for resume: %w", err)
	}

	// Resume from checkpoint with the approval response targeting the interrupt ID
	iter, err := adkRunner.ResumeWithParams(ctx, req.ConversationID, &adk.ResumeParams{
		Targets: map[string]any{
			req.InterruptID: &req.Response,
		},
	})
	if err != nil {
		return fmt.Errorf("resume from checkpoint: %w", err)
	}

	result := consumeAgentEvents(ctx, iter, emit)

	return s.finalizeRun(ctx, req.ConversationID, model, &result, emit)
}
