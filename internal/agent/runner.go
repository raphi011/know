package agent

import (
	"context"
	"fmt"
	"sync"

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
	// Detach auth from request context into a background context
	bgCtx, err := auth.DetachContext(requestCtx)
	if err != nil {
		return "", fmt.Errorf("detach auth context: %w", err)
	}

	// Propagate the request logger's fields (request_id, etc.) into the background context
	logger := logutil.FromCtx(requestCtx).With(
		"vault_id", req.VaultID,
	)
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

	bgCtx, cancel := context.WithCancel(bgCtx)
	logger = logger.With("conversation_id", req.ConversationID)
	bgCtx = logutil.WithLogger(bgCtx, logger)

	buf := NewRingBuffer[StreamEvent](defaultBufferCapacity)
	done := make(chan struct{})

	task := &runningTask{
		cancel: cancel,
		events: buf,
		done:   done,
		userID: req.UserID,
	}

	r.mu.Lock()
	r.tasks[req.ConversationID] = task
	r.mu.Unlock()

	// Mark conversation as running in DB
	if err := r.db.SetConversationBgRunning(bgCtx, req.ConversationID); err != nil {
		logger.Warn("failed to set bg_status running", "error", err)
	}

	// Emit conv_id so subscribers know which conversation this is
	buf.Push(StreamEvent{Type: "conv_id", ConvID: req.ConversationID})

	convID := req.ConversationID

	go func() {
		defer close(done)
		defer buf.Close()

		emit := func(event StreamEvent) {
			buf.Push(event)
		}

		chatErr := r.service.Chat(bgCtx, req, emit)

		task.err = chatErr

		if chatErr != nil {
			logger.Error("agent chat error", "error", chatErr)
			buf.Push(StreamEvent{Type: "error", Content: "Failed to process chat request. Please try again."})
			if dbErr := r.db.SetConversationBgFailed(context.Background(), convID, chatErr.Error()); dbErr != nil {
				logger.Warn("failed to set bg_status failed", "error", dbErr)
			}
		} else {
			if dbErr := r.db.SetConversationBgCompleted(context.Background(), convID); dbErr != nil {
				logger.Warn("failed to set bg_status completed", "error", dbErr)
			}
		}

		// Clean up task from map after a brief period to allow final event reads
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
// If ctx expires before all agents complete, it returns ctx.Err().
func (r *Runner) Shutdown(ctx context.Context) error {
	r.mu.Lock()
	tasks := make([]*runningTask, 0, len(r.tasks))
	for _, t := range r.tasks {
		tasks = append(tasks, t)
		t.cancel()
	}
	r.mu.Unlock()

	logger := logutil.FromCtx(ctx)
	logger.Info("agent runner: shutting down", "agents", len(tasks))

	for _, t := range tasks {
		select {
		case <-t.done:
		case <-ctx.Done():
			logger.Warn("agent runner: shutdown deadline exceeded, some agents did not finish")
			return ctx.Err()
		}
	}
	return nil
}
