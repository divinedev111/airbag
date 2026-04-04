package store

import (
	"os"
	"testing"
	"time"
)

func testDB(t *testing.T) *DB {
	t.Helper()
	f, err := os.CreateTemp("", "airbag-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	db, err := Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestInsertAndListIssues(t *testing.T) {
	db := testDB(t)

	r := &CrashReport{
		ProjectID:   "my-app",
		Type:        "error",
		Message:     "connection refused",
		Level:       "error",
		Environment: "production",
		Fingerprint: "abc123",
		Timestamp:   time.Now(),
	}

	if err := db.InsertReport(r); err != nil {
		t.Fatal(err)
	}
	if r.ID == 0 {
		t.Error("expected non-zero ID")
	}

	issues, err := db.ListIssues("my-app", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 {
		t.Fatalf("issues = %d", len(issues))
	}
	if issues[0].EventCount != 1 {
		t.Errorf("event count = %d", issues[0].EventCount)
	}
}

func TestIssueDeduplication(t *testing.T) {
	db := testDB(t)
	now := time.Now()

	for i := 0; i < 5; i++ {
		db.InsertReport(&CrashReport{
			ProjectID: "app", Type: "error", Message: "timeout",
			Level: "error", Fingerprint: "same-fp", Timestamp: now,
		})
	}

	issues, _ := db.ListIssues("app", "", 50)
	if len(issues) != 1 {
		t.Fatalf("should group into 1 issue, got %d", len(issues))
	}
	if issues[0].EventCount != 5 {
		t.Errorf("event count = %d, want 5", issues[0].EventCount)
	}
}

func TestIssueStatusUpdate(t *testing.T) {
	db := testDB(t)
	db.InsertReport(&CrashReport{
		ProjectID: "app", Type: "error", Message: "test",
		Level: "error", Fingerprint: "fp1", Timestamp: time.Now(),
	})

	issues, _ := db.ListIssues("app", "", 50)
	if issues[0].Status != "open" {
		t.Errorf("initial status = %q", issues[0].Status)
	}

	db.UpdateIssueStatus(issues[0].ID, "resolved")
	issues, _ = db.ListIssues("app", "resolved", 50)
	if len(issues) != 1 {
		t.Fatal("should have 1 resolved issue")
	}
}

func TestResolvedIssueReopens(t *testing.T) {
	db := testDB(t)
	now := time.Now()

	db.InsertReport(&CrashReport{
		ProjectID: "app", Type: "error", Message: "test",
		Level: "error", Fingerprint: "fp2", Timestamp: now,
	})

	issues, _ := db.ListIssues("app", "", 50)
	db.UpdateIssueStatus(issues[0].ID, "resolved")

	// New event with same fingerprint should reopen
	db.InsertReport(&CrashReport{
		ProjectID: "app", Type: "error", Message: "test",
		Level: "error", Fingerprint: "fp2", Timestamp: now.Add(time.Minute),
	})

	issues, _ = db.ListIssues("app", "open", 50)
	if len(issues) != 1 {
		t.Error("resolved issue should reopen on new event")
	}
}

func TestGetIssueEvents(t *testing.T) {
	db := testDB(t)
	now := time.Now()

	db.InsertReport(&CrashReport{
		ProjectID: "app", Type: "error", Message: "err 1",
		Stacktrace: "main.go:10", Level: "error", Fingerprint: "fp3", Timestamp: now,
	})
	db.InsertReport(&CrashReport{
		ProjectID: "app", Type: "error", Message: "err 1",
		Stacktrace: "main.go:10", Level: "error", Fingerprint: "fp3", Timestamp: now.Add(time.Second),
	})

	events, err := db.GetIssueEvents("fp3", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Errorf("events = %d, want 2", len(events))
	}
}

func TestProjectStats(t *testing.T) {
	db := testDB(t)
	now := time.Now()

	db.InsertReport(&CrashReport{
		ProjectID: "app", Type: "error", Message: "a",
		Level: "error", Fingerprint: "f1", Timestamp: now,
	})
	db.InsertReport(&CrashReport{
		ProjectID: "app", Type: "error", Message: "b",
		Level: "error", Fingerprint: "f2", Timestamp: now,
	})

	open, events, err := db.ProjectStats("app", now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if open != 2 {
		t.Errorf("open = %d", open)
	}
	if events != 2 {
		t.Errorf("events = %d", events)
	}
}
