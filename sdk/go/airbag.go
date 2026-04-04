// Package airbag provides a Go SDK for reporting crashes to an airbag server.
package airbag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"
)

// Client sends crash reports to an airbag server.
type Client struct {
	endpoint  string
	projectID string
	release   string
	env       string
	http      *http.Client
}

// New creates a new airbag client.
func New(endpoint, projectID string) *Client {
	return &Client{
		endpoint:  strings.TrimRight(endpoint, "/"),
		projectID: projectID,
		env:       "production",
		http:      &http.Client{Timeout: 5 * time.Second},
	}
}

// SetRelease sets the release/version tag.
func (c *Client) SetRelease(release string) { c.release = release }

// SetEnvironment sets the environment (production, staging, etc).
func (c *Client) SetEnvironment(env string) { c.env = env }

// CaptureError reports an error.
func (c *Client) CaptureError(err error) {
	c.send("error", err.Error(), captureStack(3), "error")
}

// CapturePanic reports a panic value (call from a deferred recover).
func (c *Client) CapturePanic(v any) {
	c.send("panic", fmt.Sprintf("%v", v), captureStack(4), "fatal")
}

// CaptureMessage reports a message at a given level.
func (c *Client) CaptureMessage(level, message string) {
	c.send("error", message, "", level)
}

func (c *Client) send(typ, message, stacktrace, level string) {
	event := map[string]any{
		"type":        typ,
		"message":     message,
		"level":       level,
		"environment": c.env,
		"timestamp":   time.Now().UTC(),
	}
	if stacktrace != "" {
		event["stacktrace"] = stacktrace
	}
	if c.release != "" {
		event["release"] = c.release
	}

	body, _ := json.Marshal(event)
	url := fmt.Sprintf("%s/api/ingest/%s", c.endpoint, c.projectID)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return // fail silently — crash reporting should never crash your app
	}
	resp.Body.Close()
}

func captureStack(skip int) string {
	var pcs [32]uintptr
	n := runtime.Callers(skip, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])

	var b strings.Builder
	for {
		frame, more := frames.Next()
		fmt.Fprintf(&b, "%s\n\t%s:%d\n", frame.Function, frame.File, frame.Line)
		if !more {
			break
		}
	}
	return b.String()
}
