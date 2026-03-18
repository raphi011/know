package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
)

type jobStatusResponse struct {
	Stats        models.JobStats            `json:"stats"`
	Durations    []models.JobTypeDuration   `json:"durations"`
	Active       []models.PipelineJobDetail `json:"active"`
	RecentFailed []models.PipelineJobDetail `json:"recent_failed"`
}

func (s *Server) getJobStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	db := s.app.DBClient()
	logger := logutil.FromCtx(ctx)

	// Parse "since" parameter (default 24h). Supports: "1h", "24h", "7d", "30d".
	sinceStr := r.URL.Query().Get("since")
	if sinceStr == "" {
		sinceStr = "24h"
	}
	sinceDur, err := parseSinceDuration(sinceStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid since parameter: %q is not a valid duration", sinceStr))
		return
	}
	since := time.Now().Add(-sinceDur)

	// Parse limit for recent jobs (default 10).
	limit := 10
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = n
	}

	stats, err := db.GetJobStats(ctx, since)
	if err != nil {
		logger.Error("get job stats", "since", since, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get job stats")
		return
	}

	active, err := db.ListRecentJobs(ctx, limit, []string{"running"})
	if err != nil {
		logger.Error("list active jobs", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list active jobs")
		return
	}

	recentFailed, err := db.ListRecentJobs(ctx, limit, []string{"failed"})
	if err != nil {
		logger.Error("list failed jobs", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list failed jobs")
		return
	}

	durations, err := db.GetJobTypeDurations(ctx, since)
	if err != nil {
		logger.Error("get job type durations", "since", since, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get job durations")
		return
	}

	writeJSON(w, http.StatusOK, jobStatusResponse{
		Stats:        *stats,
		Durations:    durations,
		Active:       active,
		RecentFailed: recentFailed,
	})
}

// parseSinceDuration parses a human-friendly duration like "1h", "24h", "7d".
func parseSinceDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	var d time.Duration
	var err error

	if before, ok := strings.CutSuffix(s, "d"); ok {
		n, parseErr := strconv.Atoi(before)
		if parseErr != nil {
			return 0, fmt.Errorf("invalid day count %q", s)
		}
		d = time.Duration(n) * 24 * time.Hour
	} else {
		d, err = time.ParseDuration(s)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q", s)
		}
	}

	if d <= 0 {
		return 0, fmt.Errorf("duration must be positive, got %q", s)
	}
	return d, nil
}
