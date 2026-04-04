package ingest

import "testing"

func TestFingerprint(t *testing.T) {
	fp1 := fingerprint("error", "connection refused", "main.go:42")
	fp2 := fingerprint("error", "connection refused", "main.go:42")
	fp3 := fingerprint("error", "timeout", "main.go:42")

	if fp1 != fp2 {
		t.Error("same input should produce same fingerprint")
	}
	if fp1 == fp3 {
		t.Error("different input should produce different fingerprint")
	}
	if len(fp1) != 16 {
		t.Errorf("fingerprint length = %d, want 16", len(fp1))
	}
}

func TestFirstLine(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"hello world", "hello world"},
		{"line 1\nline 2", "line 1"},
		{"", ""},
	}
	for _, tt := range tests {
		got := firstLine(tt.input)
		if got != tt.want {
			t.Errorf("firstLine(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExtractProjectID(t *testing.T) {
	tests := []struct {
		path, want string
	}{
		{"/api/ingest/my-project", "my-project"},
		{"/api/ingest/test-123", "test-123"},
		{"/api/ingest/", ""},
		{"/api/", ""},
	}
	for _, tt := range tests {
		got := extractProjectID(tt.path)
		if got != tt.want {
			t.Errorf("extractProjectID(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
