package api

import "net/http"

func (s *Server) getConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.app.Config()
	writeJSON(w, http.StatusOK, ServerConfig{
		Version:                cfg.Version,
		Commit:                 cfg.Commit,
		SurrealDBURL:           cfg.SurrealDBURL,
		AuthEnabled:            cfg.AuthEnabled,
		LLMProvider:            cfg.LLMProvider,
		LLMModel:               cfg.LLMModel,
		EmbedProvider:          cfg.EmbedProvider,
		EmbedModel:             cfg.EmbedModel,
		EmbedDimension:         cfg.EmbedDimension,
		SemanticSearchEnabled:  cfg.SemanticSearchEnabled,
		AgentChatEnabled:       cfg.AgentChatEnabled,
		WebSearchEnabled:       cfg.WebSearchEnabled,
		ChunkThreshold:         cfg.ChunkThreshold,
		ChunkTargetSize:        cfg.ChunkTargetSize,
		ChunkMaxSize:           cfg.ChunkMaxSize,
		VersionCoalesceMinutes: cfg.VersionCoalesceMinutes,
		VersionRetentionCount:  cfg.VersionRetentionCount,
	})
}
