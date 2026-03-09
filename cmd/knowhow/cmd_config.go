package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

var configJSON bool

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show server configuration",
	RunE:  runConfig,
}

func init() {
	configCmd.Flags().BoolVar(&configJSON, "json", false, "output as JSON")
}

type serverConfigResponse struct {
	ServerConfig serverConfig `json:"serverConfig"`
}

type serverConfig struct {
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

func runConfig(_ *cobra.Command, _ []string) error {
	if err := requireToken(); err != nil {
		return err
	}

	client := newGQLClient(apiURL, apiToken)

	query := `query {
		serverConfig {
			llmProvider
			llmModel
			embedProvider
			embedModel
			embedDimension
			semanticSearchEnabled
			agentChatEnabled
			webSearchEnabled
			chunkThreshold
			chunkTargetSize
			chunkMaxSize
			versionCoalesceMinutes
			versionRetentionCount
		}
	}`

	var resp serverConfigResponse
	if err := client.Do(context.Background(), query, nil, &resp); err != nil {
		return fmt.Errorf("config: %w", err)
	}

	if configJSON {
		out, err := json.MarshalIndent(resp.ServerConfig, "", "  ")
		if err != nil {
			return fmt.Errorf("config: marshal json: %w", err)
		}
		fmt.Println(string(out))
		return nil
	}

	c := resp.ServerConfig
	rows := []struct{ label, value string }{
		{"LLM Provider", c.LLMProvider},
		{"LLM Model", c.LLMModel},
		{"Embed Provider", c.EmbedProvider},
		{"Embed Model", c.EmbedModel},
		{"Embed Dimension", strconv.Itoa(c.EmbedDimension)},
		{"Semantic Search", strconv.FormatBool(c.SemanticSearchEnabled)},
		{"Agent Chat", strconv.FormatBool(c.AgentChatEnabled)},
		{"Web Search", strconv.FormatBool(c.WebSearchEnabled)},
		{"Chunk Threshold", strconv.Itoa(c.ChunkThreshold)},
		{"Chunk Target Size", strconv.Itoa(c.ChunkTargetSize)},
		{"Chunk Max Size", strconv.Itoa(c.ChunkMaxSize)},
		{"Version Coalesce (min)", strconv.Itoa(c.VersionCoalesceMinutes)},
		{"Version Retention", strconv.Itoa(c.VersionRetentionCount)},
	}

	for _, r := range rows {
		fmt.Printf("%-25s %s\n", r.label+":", r.value)
	}
	return nil
}
