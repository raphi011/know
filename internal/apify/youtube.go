package apify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// defaultActorID is the Apify actor used for YouTube transcript extraction.
// This can be changed if a better actor becomes available.
const defaultActorID = "topaz_sharingan/youtube-transcript-scraper"

// TranscriptResult holds a fetched YouTube transcript.
type TranscriptResult struct {
	Title       string
	Channel     string
	URL         string
	Content     string // full transcript text
	LanguageTag string // e.g. "en"
}

// actorInput is the input schema for the YouTube transcript actor.
type actorInput struct {
	StartURLs         []actorURL `json:"startUrls"`
	IncludeTimestamps string     `json:"includeTimestamps"`
}

type actorURL struct {
	URL string `json:"url"`
}

// actorOutput is the output schema from the YouTube transcript actor.
type actorOutput struct {
	Title         string `json:"title"`
	URL           string `json:"url"`
	ChannelName   string `json:"channelName"`
	TranscriptRaw string `json:"transcript"`
	Language      string `json:"language"`
	// Some actors return structured segments instead of flat text.
	Segments []struct {
		Text string `json:"text"`
	} `json:"segments"`
}

// FetchTranscript fetches the transcript for a YouTube video.
func FetchTranscript(ctx context.Context, client *Client, videoURL string) (*TranscriptResult, error) {
	if client == nil {
		return nil, fmt.Errorf("apify client is nil")
	}

	normalized, err := normalizeYouTubeURL(videoURL)
	if err != nil {
		return nil, err
	}

	input := actorInput{
		StartURLs:         []actorURL{{URL: normalized}},
		IncludeTimestamps: "No",
	}

	items, err := client.RunActorSync(ctx, defaultActorID, input)
	if err != nil {
		return nil, fmt.Errorf("fetch transcript: %w", err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("no transcript available for %s (the video may not have captions)", normalized)
	}

	var out actorOutput
	if err := json.Unmarshal(items[0], &out); err != nil {
		return nil, fmt.Errorf("unmarshal transcript: %w", err)
	}

	content := out.TranscriptRaw
	if content == "" && len(out.Segments) > 0 {
		var sb strings.Builder
		for _, seg := range out.Segments {
			sb.WriteString(seg.Text)
			sb.WriteString(" ")
		}
		content = strings.TrimSpace(sb.String())
	}
	if content == "" {
		return nil, fmt.Errorf("transcript is empty for %s", normalized)
	}

	return &TranscriptResult{
		Title:       out.Title,
		Channel:     out.ChannelName,
		URL:         out.URL,
		Content:     content,
		LanguageTag: out.Language,
	}, nil
}

// FormatTranscript formats a TranscriptResult as a human-readable markdown string.
func FormatTranscript(r *TranscriptResult) string {
	var sb strings.Builder
	if r.Title != "" {
		fmt.Fprintf(&sb, "# %s\n", r.Title)
	}
	var meta []string
	if r.Channel != "" {
		meta = append(meta, "Channel: "+r.Channel)
	}
	if r.URL != "" {
		meta = append(meta, "URL: "+r.URL)
	}
	if len(meta) > 0 {
		sb.WriteString(strings.Join(meta, " | "))
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
	sb.WriteString(r.Content)
	return sb.String()
}

// normalizeYouTubeURL parses a YouTube URL or video ID and returns a
// canonical youtube.com/watch?v= URL.
func normalizeYouTubeURL(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("empty YouTube URL")
	}

	// Bare video ID (11 characters, alphanumeric + _ + -)
	if !strings.Contains(input, "/") && !strings.Contains(input, ".") {
		if len(input) >= 10 && len(input) <= 12 {
			return "https://www.youtube.com/watch?v=" + input, nil
		}
		return "", fmt.Errorf("invalid YouTube video ID: %s", input)
	}

	// Add scheme if missing.
	if !strings.HasPrefix(input, "http://") && !strings.HasPrefix(input, "https://") {
		input = "https://" + input
	}

	u, err := url.Parse(input)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	host := strings.ToLower(u.Hostname())

	switch {
	case host == "youtu.be":
		videoID := strings.TrimPrefix(u.Path, "/")
		if videoID == "" {
			return "", fmt.Errorf("missing video ID in youtu.be URL")
		}
		return "https://www.youtube.com/watch?v=" + videoID, nil

	case host == "youtube.com" || host == "www.youtube.com" || host == "m.youtube.com":
		// /watch?v=ID
		if v := u.Query().Get("v"); v != "" {
			return "https://www.youtube.com/watch?v=" + v, nil
		}
		// /shorts/ID or /embed/ID or /v/ID
		parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
		if len(parts) >= 2 {
			switch parts[0] {
			case "shorts", "embed", "v", "live":
				return "https://www.youtube.com/watch?v=" + parts[1], nil
			}
		}
		return "", fmt.Errorf("could not extract video ID from YouTube URL: %s", input)

	default:
		return "", fmt.Errorf("not a YouTube URL: %s", input)
	}
}
