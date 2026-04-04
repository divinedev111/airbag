// Package ingest handles incoming crash reports from client SDKs.
package ingest

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/divinedev111/airbag/internal/store"
)

// Handler processes incoming crash report submissions.
type Handler struct {
	db *store.DB
}

// New creates an ingest Handler.
func New(db *store.DB) *Handler {
	return &Handler{db: db}
}

// Event is the incoming JSON payload from client SDKs.
type Event struct {
	Type        string            `json:"type"`
	Message     string            `json:"message"`
	Stacktrace  string            `json:"stacktrace"`
	Level       string            `json:"level"`
	Environment string            `json:"environment"`
	Release     string            `json:"release"`
	UserID      string            `json:"user_id"`
	Tags        map[string]string `json:"tags"`
	Extra       map[string]any    `json:"extra"`
	Timestamp   *time.Time        `json:"timestamp"`
}

// ServeHTTP handles POST /api/ingest/:project_id
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	projectID := extractProjectID(r.URL.Path)
	if projectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project_id required"})
		return
	}

	var event Event
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if event.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message required"})
		return
	}

	// Defaults
	if event.Type == "" {
		event.Type = "error"
	}
	if event.Level == "" {
		event.Level = "error"
	}
	if event.Environment == "" {
		event.Environment = "production"
	}

	ts := time.Now()
	if event.Timestamp != nil {
		ts = *event.Timestamp
	}

	report := &store.CrashReport{
		ProjectID:   projectID,
		Type:        event.Type,
		Message:     event.Message,
		Stacktrace:  event.Stacktrace,
		Level:       event.Level,
		Environment: event.Environment,
		Release:     event.Release,
		UserID:      event.UserID,
		Tags:        event.Tags,
		Extra:       event.Extra,
		Fingerprint: fingerprint(event.Type, event.Message, event.Stacktrace),
		Timestamp:   ts,
	}

	if err := h.db.InsertReport(report); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{"id": report.ID})
}

func fingerprint(typ, message, stacktrace string) string {
	// Fingerprint by type + first line of message + first frame of stack
	input := typ + "|" + firstLine(message)
	if stacktrace != "" {
		input += "|" + firstLine(stacktrace)
	}
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", hash[:8])
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}

func extractProjectID(path string) string {
	// /api/ingest/my-project -> my-project
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) >= 3 {
		return parts[2]
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
