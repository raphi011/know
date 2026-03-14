package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
)

func (s *Server) listConversations(w http.ResponseWriter, r *http.Request) {
	vaultID := r.URL.Query().Get("vault")
	if vaultID == "" {
		writeError(w, http.StatusBadRequest, "vault query parameter required")
		return
	}

	if err := auth.RequireVaultRole(r.Context(), vaultID, models.RoleRead); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	ac, err := auth.FromContext(r.Context())
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	convs, err := s.app.DBClient().ListConversations(r.Context(), vaultID, ac.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list conversations")
		logutil.FromCtx(r.Context()).Error("list conversations", "error", err)
		return
	}

	result := make([]Conversation, len(convs))
	for i := range convs {
		result[i] = conversationFromModel(&convs[i])
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) getConversation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	logger := logutil.FromCtx(r.Context())

	conv, err := s.app.DBClient().GetConversation(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get conversation")
		logger.Error("get conversation", "error", err)
		return
	}
	if conv == nil {
		writeError(w, http.StatusNotFound, "conversation not found")
		return
	}

	vaultID, err := models.RecordIDString(conv.Vault)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "invalid vault ID")
		return
	}
	if err := auth.RequireVaultRole(r.Context(), vaultID, models.RoleRead); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	result := conversationFromModel(conv)

	msgs, err := s.app.DBClient().ListMessages(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list messages")
		logger.Error("list messages", "error", err)
		return
	}
	result.Messages = make([]*ChatMessage, len(msgs))
	for i := range msgs {
		result.Messages[i] = messageFromModel(&msgs[i])
	}

	writeJSON(w, http.StatusOK, result)
}

type createConversationRequest struct {
	VaultID string `json:"vaultId"`
}

func (s *Server) createConversation(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024) // 64 KB
	var body createConversationRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.VaultID == "" {
		writeError(w, http.StatusBadRequest, "vaultId is required")
		return
	}

	if err := auth.RequireVaultRole(r.Context(), body.VaultID, models.RoleWrite); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	ac, err := auth.FromContext(r.Context())
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	conv, err := s.app.DBClient().CreateConversation(r.Context(), body.VaultID, ac.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create conversation")
		logutil.FromCtx(r.Context()).Error("create conversation", "error", err)
		return
	}

	result := conversationFromModel(conv)
	result.Messages = []*ChatMessage{}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) deleteConversation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	logger := logutil.FromCtx(r.Context())

	conv, err := s.app.DBClient().GetConversation(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get conversation")
		logger.Error("get conversation for delete", "error", err)
		return
	}
	if conv == nil {
		writeError(w, http.StatusNotFound, "conversation not found")
		return
	}

	vaultID, err := models.RecordIDString(conv.Vault)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "invalid vault ID")
		return
	}
	if err := auth.RequireVaultRole(r.Context(), vaultID, models.RoleWrite); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	if err := s.app.DBClient().DeleteConversation(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete conversation")
		logger.Error("delete conversation", "error", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type renameConversationRequest struct {
	Title string `json:"title"`
}

func (s *Server) renameConversation(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024) // 64 KB
	id := r.PathValue("id")
	logger := logutil.FromCtx(r.Context())

	var body renameConversationRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	conv, err := s.app.DBClient().GetConversation(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get conversation")
		logger.Error("get conversation for rename", "error", err)
		return
	}
	if conv == nil {
		writeError(w, http.StatusNotFound, "conversation not found")
		return
	}

	vaultID, err := models.RecordIDString(conv.Vault)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "invalid vault ID")
		return
	}
	if err := auth.RequireVaultRole(r.Context(), vaultID, models.RoleWrite); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	if err := s.app.DBClient().UpdateConversationTitle(r.Context(), id, body.Title); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to rename conversation")
		logger.Error("rename conversation", "error", err)
		return
	}

	updated, err := s.app.DBClient().GetConversation(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get updated conversation")
		logger.Error("get renamed conversation", "error", err)
		return
	}

	result := conversationFromModel(updated)
	result.Messages = []*ChatMessage{}
	writeJSON(w, http.StatusOK, result)
}

// conversationFromModel converts a models.Conversation to an API Conversation.
func conversationFromModel(c *models.Conversation) Conversation {
	id, err := models.RecordIDString(c.ID)
	if err != nil {
		slog.Warn("unexpected conversation ID format", "error", err)
		id = fmt.Sprintf("%v", c.ID.ID)
	}
	vaultID, err := models.RecordIDString(c.Vault)
	if err != nil {
		slog.Warn("unexpected conversation vault ID format", "error", err)
		vaultID = fmt.Sprintf("%v", c.Vault.ID)
	}
	return Conversation{
		ID:          id,
		VaultID:     vaultID,
		Title:       c.Title,
		TokenInput:  c.TokenInput,
		TokenOutput: c.TokenOutput,
		BgStatus:    c.BgStatus,
		CreatedAt:   c.CreatedAt,
		UpdatedAt:   c.UpdatedAt,
	}
}

// messageFromModel converts a models.Message to an API ChatMessage.
func messageFromModel(m *models.Message) *ChatMessage {
	id, err := models.RecordIDString(m.ID)
	if err != nil {
		slog.Warn("unexpected message ID format", "error", err)
		id = fmt.Sprintf("%v", m.ID.ID)
	}
	docRefs := m.DocRefs
	if docRefs == nil {
		docRefs = []string{}
	}
	return &ChatMessage{
		ID:         id,
		Role:       string(m.Role),
		Content:    m.Content,
		DocRefs:    docRefs,
		ToolName:   m.ToolName,
		ToolInput:  m.ToolInput,
		ToolCallID: m.ToolCallID,
		ToolCalls:  m.ToolCalls,
		CreatedAt:  m.CreatedAt,
	}
}
