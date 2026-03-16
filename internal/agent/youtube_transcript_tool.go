package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/raphi011/know/internal/apify"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/tools"
)

// YouTubeTranscriptTool is an agent-only InvokableTool that fetches YouTube
// video transcripts via the Apify API.
type YouTubeTranscriptTool struct {
	client *apify.Client
}

func (t *YouTubeTranscriptTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "fetch_youtube_transcript",
		Desc: "Fetch the transcript of a YouTube video. Accepts a YouTube URL or video ID. Returns the transcript text with video metadata.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"url": {
				Type:     schema.String,
				Desc:     "YouTube video URL (e.g. https://www.youtube.com/watch?v=...) or video ID",
				Required: true,
			},
		}),
	}, nil
}

func (t *YouTubeTranscriptTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var input struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(argumentsInJSON), &input); err != nil {
		return "", fmt.Errorf("parse fetch_youtube_transcript input: %w", err)
	}

	start := time.Now()
	result, err := apify.FetchTranscript(ctx, t.client, input.URL)
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		logutil.FromCtx(ctx).Warn("fetch youtube transcript failed",
			"url", input.URL, "duration_ms", durationMs, "error", err)
		return "", fmt.Errorf("fetch youtube transcript: %w", err)
	}

	tools.SetResultMeta(ctx, &tools.ToolResultMeta{
		DurationMs: durationMs,
	})

	return apify.FormatTranscript(result), nil
}
