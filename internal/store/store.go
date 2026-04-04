// Package store provides SQLite storage for crash reports and projects.
package store

import (
	"database/sql"
	"encoding/json"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// CrashReport represents a single crash/error event.
type CrashReport struct {
	ID          int64             `json:"id"`
	ProjectID   string            `json:"project_id"`
	Type        string            `json:"type"`        // "error", "panic", "crash"
	Message     string            `json:"message"`
	Stacktrace  string            `json:"stacktrace,omitempty"`
	Level       string            `json:"level"`       // "error", "fatal", "warning"
	Environment string            `json:"environment"` // "production", "staging", etc.
	Release     string            `json:"release,omitempty"`
	UserID      string            `json:"user_id,omitempty"`
	Tags        map[string]string `json:"tags,omitempty"`
	Extra       map[string]any    `json:"extra,omitempty"`
	Fingerprint string            `json:"fingerprint"` // dedup key
	Timestamp   time.Time         `json:"timestamp"`
	CreatedAt   time.Time         `json:"created_at"`
}

// Issue groups related crash reports by fingerprint.
type Issue struct {
	ID          int64     `json:"id"`
	ProjectID   string    `json:"project_id"`
	Fingerprint string    `json:"fingerprint"`
	Type        string    `json:"type"`
	Message     string    `json:"message"`
	Level       string    `json:"level"`
	Status      string    `json:"status"` // "open", "resolved", "ignored"
	EventCount  int       `json:"event_count"`
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
}

// DB wraps SQLite for airbag storage.
type DB struct {
	db *sql.DB
}

// Open creates or opens a SQLite database.
func Open(path string) (*DB, error) {
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return &DB{db: db}, nil
}

// Close closes the database.
func (d *DB) Close() error { return d.db.Close() }

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS crash_reports (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id TEXT NOT NULL,
			type TEXT NOT NULL DEFAULT 'error',
			message TEXT NOT NULL,
			stacktrace TEXT,
			level TEXT NOT NULL DEFAULT 'error',
			environment TEXT DEFAULT 'production',
			release_tag TEXT,
			user_id TEXT,
			tags_json TEXT,
			extra_json TEXT,
			fingerprint TEXT NOT NULL,
			timestamp DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS issues (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id TEXT NOT NULL,
			fingerprint TEXT NOT NULL UNIQUE,
			type TEXT NOT NULL,
			message TEXT NOT NULL,
			level TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'open',
			event_count INTEGER NOT NULL DEFAULT 1,
			first_seen DATETIME NOT NULL,
			last_seen DATETIME NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_reports_project ON crash_reports(project_id, timestamp);
		CREATE INDEX IF NOT EXISTS idx_reports_fingerprint ON crash_reports(fingerprint);
		CREATE INDEX IF NOT EXISTS idx_issues_project ON issues(project_id, status);
	`)
	return err
}

// InsertReport saves a crash report and updates or creates the corresponding issue.
func (d *DB) InsertReport(r *CrashReport) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	tagsJSON := "{}"
	extraJSON := "{}"
	if r.Tags != nil {
		if b, err := marshalJSON(r.Tags); err == nil {
			tagsJSON = string(b)
		}
	}
	if r.Extra != nil {
		if b, err := marshalJSON(r.Extra); err == nil {
			extraJSON = string(b)
		}
	}

	res, err := tx.Exec(`
		INSERT INTO crash_reports (project_id, type, message, stacktrace, level, environment, release_tag, user_id, tags_json, extra_json, fingerprint, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ProjectID, r.Type, r.Message, r.Stacktrace, r.Level, r.Environment, r.Release, r.UserID, tagsJSON, extraJSON, r.Fingerprint, r.Timestamp,
	)
	if err != nil {
		return err
	}
	r.ID, _ = res.LastInsertId()

	// Upsert issue
	_, err = tx.Exec(`
		INSERT INTO issues (project_id, fingerprint, type, message, level, event_count, first_seen, last_seen)
		VALUES (?, ?, ?, ?, ?, 1, ?, ?)
		ON CONFLICT(fingerprint) DO UPDATE SET
			event_count = event_count + 1,
			last_seen = excluded.last_seen,
			status = CASE WHEN status = 'resolved' THEN 'open' ELSE status END`,
		r.ProjectID, r.Fingerprint, r.Type, r.Message, r.Level, r.Timestamp, r.Timestamp,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// ListIssues returns issues for a project, optionally filtered by status.
func (d *DB) ListIssues(projectID, status string, limit int) ([]Issue, error) {
	query := `SELECT id, project_id, fingerprint, type, message, level, status, event_count, first_seen, last_seen FROM issues WHERE project_id = ?`
	args := []any{projectID}

	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY last_seen DESC LIMIT ?`
	args = append(args, limit)

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var issues []Issue
	for rows.Next() {
		var i Issue
		if err := rows.Scan(&i.ID, &i.ProjectID, &i.Fingerprint, &i.Type, &i.Message, &i.Level, &i.Status, &i.EventCount, &i.FirstSeen, &i.LastSeen); err != nil {
			return nil, err
		}
		issues = append(issues, i)
	}
	return issues, rows.Err()
}

// GetIssueEvents returns crash reports for a specific issue fingerprint.
func (d *DB) GetIssueEvents(fingerprint string, limit int) ([]CrashReport, error) {
	rows, err := d.db.Query(`
		SELECT id, project_id, type, message, stacktrace, level, environment, release_tag, user_id, fingerprint, timestamp, created_at
		FROM crash_reports WHERE fingerprint = ? ORDER BY timestamp DESC LIMIT ?`,
		fingerprint, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reports []CrashReport
	for rows.Next() {
		var r CrashReport
		var stack, release, userID sql.NullString
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.Type, &r.Message, &stack, &r.Level, &r.Environment, &release, &userID, &r.Fingerprint, &r.Timestamp, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.Stacktrace = stack.String
		r.Release = release.String
		r.UserID = userID.String
		reports = append(reports, r)
	}
	return reports, rows.Err()
}

// UpdateIssueStatus changes an issue's status.
func (d *DB) UpdateIssueStatus(id int64, status string) error {
	_, err := d.db.Exec(`UPDATE issues SET status = ? WHERE id = ?`, status, id)
	return err
}

// ProjectStats returns aggregate stats for a project.
func (d *DB) ProjectStats(projectID string, since time.Time) (openIssues, totalEvents int, err error) {
	d.db.QueryRow(`SELECT COUNT(*) FROM issues WHERE project_id = ? AND status = 'open'`, projectID).Scan(&openIssues)
	d.db.QueryRow(`SELECT COUNT(*) FROM crash_reports WHERE project_id = ? AND timestamp > ?`, projectID, since).Scan(&totalEvents)
	return
}

func marshalJSON(v any) ([]byte, error) {
	return json.Marshal(v)
}
