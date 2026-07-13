package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestRootHelp(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{nil, {"help"}, {"-h"}, {"--help"}} {
		args := args
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			if code := Run(args, &stdout, &stderr); code != 0 {
				t.Fatalf("Run(%q) exit code = %d, want 0", args, code)
			}
			if got := stdout.String(); got != HelpText {
				t.Fatalf("Run(%q) stdout = %q, want deterministic help %q", args, got, HelpText)
			}
			if stderr.Len() != 0 {
				t.Fatalf("Run(%q) stderr = %q, want empty", args, stderr.String())
			}
		})
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestRootReturnsFailureWhenOutputCannotBeWritten(t *testing.T) {
	t.Parallel()
	if code := Run([]string{"help"}, failingWriter{}, &bytes.Buffer{}); code == 0 {
		t.Fatal("help writer failure returned success")
	}
	if code := Run([]string{"unknown"}, &bytes.Buffer{}, failingWriter{}); code == 0 {
		t.Fatal("error writer failure returned success")
	}
}

func TestRootUnknownCommand(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"scan"}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if got := stderr.String(); got != "reconctx: unknown command \"scan\"\n" {
		t.Fatalf("stderr = %q", got)
	}
}

func TestRootRejectsImplicitApprovalFlag(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"--yes"}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}
