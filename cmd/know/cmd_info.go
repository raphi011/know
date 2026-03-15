package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

var (
	infoAPI  *apiFlags
	infoJSON bool
)

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show server and CLI info",
	RunE:  runInfo,
}

func init() {
	infoAPI = addAPIFlags(infoCmd)
	infoCmd.Flags().BoolVar(&infoJSON, "json", false, "output as JSON")
}

type serverInfo struct {
	Version                string `json:"version"`
	Commit                 string `json:"commit"`
	SurrealDBURL           string `json:"surrealdbURL"`
	AuthEnabled            bool   `json:"authEnabled"`
	LLMProvider            string `json:"llmProvider"`
	LLMModel               string `json:"llmModel"`
	EmbedProvider          string `json:"embedProvider"`
	EmbedModel             string `json:"embedModel"`
	EmbedDimension         int    `json:"embedDimension"`
	SemanticSearchEnabled  bool   `json:"semanticSearchEnabled"`
	AgentChatEnabled       bool   `json:"agentChatEnabled"`
	WebSearchEnabled       bool   `json:"webSearchEnabled"`
	ChunkThreshold         int    `json:"chunkThreshold"`
	ChunkTargetSize        int    `json:"chunkTargetSize"`
	ChunkMaxSize           int    `json:"chunkMaxSize"`
	VersionCoalesceMinutes int    `json:"versionCoalesceMinutes"`
	VersionRetentionCount  int    `json:"versionRetentionCount"`
}

func runInfo(_ *cobra.Command, _ []string) error {
	client := infoAPI.newClient()

	var cfg serverInfo
	if err := client.Get(context.Background(), "/api/config", &cfg); err != nil {
		return fmt.Errorf("info: %w", err)
	}

	if infoJSON {
		out, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return fmt.Errorf("info: marshal json: %w", err)
		}
		fmt.Println(string(out))
		return nil
	}

	type row struct{ label, value string }
	groups := [][]row{
		{
			{"CLI Version", version},
			{"CLI Commit", commit[:min(7, len(commit))]},
			{"Server Version", cfg.Version},
			{"Server Commit", cfg.Commit},
		},
		{
			{"Server URL", infoAPI.URL},
			{"SurrealDB URL", cfg.SurrealDBURL},
			{"Auth Enabled", strconv.FormatBool(cfg.AuthEnabled)},
		},
		{
			{"LLM Provider", cfg.LLMProvider},
			{"LLM Model", cfg.LLMModel},
		},
		{
			{"Embed Provider", cfg.EmbedProvider},
			{"Embed Model", cfg.EmbedModel},
			{"Embed Dimension", strconv.Itoa(cfg.EmbedDimension)},
			{"Semantic Search", strconv.FormatBool(cfg.SemanticSearchEnabled)},
		},
		{
			{"Agent Chat", strconv.FormatBool(cfg.AgentChatEnabled)},
			{"Web Search", strconv.FormatBool(cfg.WebSearchEnabled)},
		},
		{
			{"Chunk Threshold", strconv.Itoa(cfg.ChunkThreshold)},
			{"Chunk Target Size", strconv.Itoa(cfg.ChunkTargetSize)},
			{"Chunk Max Size", strconv.Itoa(cfg.ChunkMaxSize)},
		},
		{
			{"Version Coalesce (min)", strconv.Itoa(cfg.VersionCoalesceMinutes)},
			{"Version Retention", strconv.Itoa(cfg.VersionRetentionCount)},
		},
	}

	for i, group := range groups {
		if i > 0 {
			fmt.Println()
		}
		for _, r := range group {
			fmt.Printf("%-25s %s\n", r.label+":", r.value)
		}
	}
	return nil
}
