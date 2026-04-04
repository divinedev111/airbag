// Package api provides HTTP handlers for viewing issues and reports.
package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/divinedev111/airbag/internal/store"
)

// Handler provides read endpoints for the dashboard.
type Handler struct {
	db *store.DB
}

// New creates an API Handler.
func New(db *store.DB) *Handler {
	return &Handler{db: db}
}

// Routes returns an http.Handler with dashboard API routes.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/projects/{project}/issues", h.listIssues)
	mux.HandleFunc("GET /api/projects/{project}/stats", h.projectStats)
	mux.HandleFunc("GET /api/issues/{id}/events", h.issueEvents)
	mux.HandleFunc("POST /api/issues/{id}/resolve", h.resolveIssue)
	mux.HandleFunc("POST /api/issues/{id}/ignore", h.ignoreIssue)

	return withCORS(mux)
}

func (h *Handler) listIssues(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	status := r.URL.Query().Get("status")
	limit := intParam(r, "limit", 50)

	issues, err := h.db.ListIssues(project, status, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, issues)
}

func (h *Handler) projectStats(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	open, events, err := h.db.ProjectStats(project, time.Now().Add(-24*time.Hour))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]int{"open_issues": open, "events_24h": events})
}

func (h *Handler) issueEvents(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	limit := intParam(r, "limit", 20)

	// Get the fingerprint for this issue
	issues, _ := h.db.ListIssues("", "", 1000)
	var fp string
	for _, iss := range issues {
		if iss.ID == id {
			fp = iss.Fingerprint
			break
		}
	}
	if fp == "" {
		writeError(w, http.StatusNotFound, "issue not found")
		return
	}

	events, err := h.db.GetIssueEvents(fp, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, events)
}

func (h *Handler) resolveIssue(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := h.db.UpdateIssueStatus(id, "resolved"); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "resolved"})
}

func (h *Handler) ignoreIssue(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := h.db.UpdateIssueStatus(id, "ignored"); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "ignored"})
}

func intParam(r *http.Request, key string, defaultVal int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultVal
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
