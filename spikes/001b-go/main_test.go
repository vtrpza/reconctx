package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCompileFixture(t *testing.T) {
	fixture := filepath.Join("..", "..", "fixtures", "cases", "katana", "v1.6.1", "KAT-NORMAL-MINIMAL", "native-output.jsonl")
	out := t.TempDir()
	summary, err := CompileFixture(fixture, out)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Records != 6 {
		t.Fatalf("records=%d, want 6", summary.Records)
	}
	data, err := os.ReadFile(filepath.Join(out, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 6 {
		t.Fatalf("event lines=%d, want 6", len(lines))
	}
	var users Event
	found := false
	for _, line := range lines {
		var event Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(event.URLRaw, "api/users") {
			users = event
			found = true
		}
	}
	if !found {
		t.Fatal("api/users event not found")
	}
	if users.URLRaw != "http://127.0.0.1:18080/api/users?id=1" || users.RouteURL != "http://127.0.0.1:18080/api/users" {
		t.Fatalf("unexpected URLs: raw=%q route=%q", users.URLRaw, users.RouteURL)
	}
	context, err := os.ReadFile(filepath.Join(out, "CONTEXT.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(context), "Records: 6") || !strings.Contains(string(context), "Unique routes: 6") {
		t.Fatalf("unexpected context: %s", context)
	}
}

func TestCLICompileMaterializesArtifactsAndPrintsSummary(t *testing.T) {
	fixture := filepath.Join("..", "..", "fixtures", "cases", "katana", "v1.6.1", "KAT-NORMAL-MINIMAL", "native-output.jsonl")
	out := t.TempDir()
	var stdout bytes.Buffer
	if err := runCLI([]string{"compile", fixture, out}, &stdout); err != nil {
		t.Fatal(err)
	}
	var summary map[string]int
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatal(err)
	}
	if summary["records"] != 6 || summary["unique_routes"] != 6 {
		t.Fatalf("unexpected summary: %v", summary)
	}
	if _, err := os.Stat(filepath.Join(out, "events.jsonl")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "CONTEXT.md")); err != nil {
		t.Fatal(err)
	}
}

func TestTimeoutKillsFakeProcessGroupAndPreservesStreams(t *testing.T) {
	result, err := RunSupervised(
		[]string{"python3", filepath.Join("..", "fake_tool.py"), "--mode", "hang"},
		250*time.Millisecond,
		100*time.Millisecond,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.TimedOut {
		t.Fatal("expected timeout")
	}
	if result.Duration >= 2*time.Second {
		t.Fatalf("duration=%s, want <2s", result.Duration)
	}
	if !strings.Contains(result.Stdout, "started") || !strings.Contains(result.Stderr, "child_started") {
		t.Fatalf("streams not preserved: stdout=%q stderr=%q", result.Stdout, result.Stderr)
	}
	if result.ExitCode == 0 {
		t.Fatal("timed out process unexpectedly exited 0")
	}
}
