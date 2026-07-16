package runner

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/vtrpza/reconctx/internal/canonical"
	"github.com/vtrpza/reconctx/internal/model"
	"github.com/vtrpza/reconctx/internal/preflight"
)

type Limits struct {
	Timeout        time.Duration
	GracePeriod    time.Duration
	MaxStdoutBytes int64
	MaxStderrBytes int64
	MaxNativeBytes int64
	MaxRecords     int
	MaxLineBytes   int
}

type NativeOutput struct {
	Path     string `json:"path"`
	Required bool   `json:"required"`
}

type Request struct {
	ExecutionID          string
	WorkspaceRoot        string
	OutputDir            string
	Tool                 model.ToolPlan
	Environment          []string
	EnvironmentAllowlist []string
	RequireStdout        bool
	StdoutIsResult       bool
	NativeOutputs        []NativeOutput
	Limits               Limits
}

type boundedCapture struct {
	file       *os.File
	hash       hash.Hash
	maxBytes   int64
	maxRecords int
	maxLine    int
	written    int64
	records    int
	pending    []byte
	truncated  bool
	writeErr   error
}

var ErrUnsupportedPlatform = errors.New("runner is supported only on Linux")
var ErrContainmentUnavailable = errors.New("required process containment is unavailable")

func Run(ctx context.Context, request Request) (Result, error) {
	if !platformSupported() {
		return Result{}, ErrUnsupportedPlatform
	}
	if err := validateRequest(request); err != nil {
		return Result{}, err
	}
	identity, err := preflight.ResolveTool(request.Tool.ResolvedPath)
	if err != nil || !sameBinary(identity, request.Tool) {
		return Result{}, errors.New("tool identity changed before execution")
	}
	environment, err := preflight.FilterEnvironment(request.Environment, request.EnvironmentAllowlist)
	if err != nil {
		return Result{}, err
	}
	directory, err := createExecutionDir(request.OutputDir)
	if err != nil {
		return Result{}, err
	}
	defer directory.close()
	stdout, err := newBoundedCapture(directory, "stdout.partial", request.Limits.MaxStdoutBytes, request.Limits.MaxRecords, request.Limits.MaxLineBytes)
	if err != nil {
		return Result{}, err
	}
	stderr, err := newBoundedCapture(directory, "stderr.partial", request.Limits.MaxStderrBytes, request.Limits.MaxRecords, request.Limits.MaxLineBytes)
	if err != nil {
		stdout.file.Close()
		return Result{}, err
	}

	runCtx, cancel := context.WithTimeout(ctx, request.Limits.Timeout)
	defer cancel()
	started := time.Now().UTC()
	command, contained, pid, err := startLimitedCommand(runCtx, request, environment, stdout, stderr)
	if err != nil {
		stdout.file.Close()
		stderr.file.Close()
		return Result{}, err
	}
	wait := make(chan error, 1)
	go func() { wait <- command.Wait() }()
	waitErr, interruption, terminationErr := waitForProcess(runCtx, wait, pid, request.Limits.GracePeriod, contained.kill)
	leaked, containmentErr := contained.finish(pid, request.Limits.GracePeriod, interruption != nil)
	if err := errors.Join(terminationErr, containmentErr); err != nil {
		stdout.file.Close()
		stderr.file.Close()
		return Result{}, err
	}
	finished := time.Now().UTC()

	stdoutArtifact, stdoutErr := stdout.finalize(directory, "stdout.partial", "stdout.raw", "stdout")
	stderrArtifact, stderrErr := stderr.finalize(directory, "stderr.partial", "stderr.raw", "stderr")
	if stdoutErr != nil || stderrErr != nil {
		return Result{}, errors.Join(stdoutErr, stderrErr)
	}
	artifacts := []Artifact{stdoutArtifact, stderrArtifact}
	nativeArtifacts, missingRequiredOutput, nativeErr := captureNativeOutputs(directory, request.NativeOutputs, request.Limits.MaxNativeBytes)
	artifacts = append(artifacts, nativeArtifacts...)
	envelope := buildEnvelope(request, environment, started, finished, waitErr, interruption, leaked, artifacts...)
	contained.applyOutcome(&envelope)
	if nativeErr != nil && envelope.Status != StatusInterrupted {
		envelope.Status = StatusFailed
		envelope.Reason = "native_artifact_invalid"
	}
	if missingRequiredOutput && envelope.Status == StatusSuccess {
		envelope.Status = StatusFailed
		envelope.Reason = "missing_required_output"
	}
	if request.RequireStdout && stdoutArtifact.Size == 0 && envelope.Status == StatusSuccess {
		envelope.Status = StatusFailed
		envelope.Reason = "missing_required_stdout"
	}
	if err := persistEnvelope(directory, envelope); err != nil {
		return Result{}, err
	}
	return Result{Envelope: envelope}, nil
}

func RunAll(ctx context.Context, requests []Request, parallelism int) ([]Result, error) {
	if parallelism <= 0 {
		return nil, errors.New("parallelism must be positive")
	}
	for _, request := range requests {
		if parallelism > request.Tool.Limits.Parallelism {
			return nil, errors.New("parallelism exceeds approved tool limit")
		}
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make([]Result, len(requests))
	errorsByIndex := make([]error, len(requests))
	jobs := make(chan int)
	var wait sync.WaitGroup
	workers := min(parallelism, len(requests))
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for index := range jobs {
				if err := runCtx.Err(); err != nil {
					errorsByIndex[index] = err
					continue
				}
				results[index], errorsByIndex[index] = Run(runCtx, requests[index])
				if errorsByIndex[index] != nil {
					cancel()
				}
			}
		}()
	}
	for index := range requests {
		select {
		case jobs <- index:
		case <-runCtx.Done():
			errorsByIndex[index] = runCtx.Err()
		}
	}
	close(jobs)
	wait.Wait()
	for _, err := range errorsByIndex {
		if err != nil {
			return results, err
		}
	}
	return results, nil
}

func validateRequest(request Request) error {
	if request.ExecutionID == "" || strings.ContainsAny(request.ExecutionID, "/\\\x00") {
		return errors.New("invalid execution ID")
	}
	if len(request.Tool.Argv) == 0 || request.Tool.Argv[0] != request.Tool.ResolvedPath {
		return errors.New("argv is not bound to resolved tool")
	}
	for _, argument := range request.Tool.Argv {
		if strings.ContainsRune(argument, '\x00') {
			return errors.New("argv contains NUL")
		}
	}
	for _, key := range request.EnvironmentAllowlist {
		if key == helperEnvironment {
			return errors.New("environment allowlist contains runner-reserved key")
		}
	}
	if !filepath.IsAbs(request.WorkspaceRoot) || filepath.Clean(request.WorkspaceRoot) != request.WorkspaceRoot || !filepath.IsAbs(request.OutputDir) || filepath.Clean(request.OutputDir) != request.OutputDir {
		return errors.New("workspace and output paths must be absolute and clean")
	}
	approvedNative := make(map[string]bool, len(request.Tool.OutputPaths))
	seenApproved := make(map[string]bool, len(request.Tool.OutputPaths))
	approvedDirectory := ""
	hasStdout := false
	for _, output := range request.Tool.OutputPaths {
		if output == "" || path.IsAbs(output) || path.Clean(output) != output || strings.ContainsAny(output, "\\\x00\r\n") || strings.HasPrefix(output, "../") {
			return errors.New("approved output path is unsafe")
		}
		directory := path.Dir(output)
		if approvedDirectory == "" {
			approvedDirectory = directory
		} else if directory != approvedDirectory {
			return errors.New("approved outputs must share one execution directory")
		}
		name := path.Base(output)
		if seenApproved[name] {
			return errors.New("approved output path is duplicated")
		}
		seenApproved[name] = true
		if name == "stdout.raw" {
			hasStdout = true
		} else if name == "stderr.raw" {
			// stderr is always captured by the runner and may be listed in the
			// approved plan; it is not a tool-created native output.
		} else {
			approvedNative[name] = true
		}
	}
	if !hasStdout || filepath.Join(request.WorkspaceRoot, filepath.FromSlash(approvedDirectory)) != request.OutputDir {
		return errors.New("execution directory does not match approved output paths")
	}
	seenOutputs := make(map[string]bool, len(request.NativeOutputs))
	if len(request.NativeOutputs) > 16 {
		return errors.New("too many native outputs")
	}
	for _, output := range request.NativeOutputs {
		if output.Path == "" || path.Base(output.Path) != output.Path || strings.ContainsAny(output.Path, "\\\x00\r\n") || reservedArtifactName(output.Path) || seenOutputs[output.Path] || !approvedNative[output.Path] {
			return errors.New("native output is invalid or not approved")
		}
		seenOutputs[output.Path] = true
	}
	if len(seenOutputs) != len(approvedNative) {
		return errors.New("approved native outputs do not match captured native outputs")
	}
	approvedTimeout := time.Duration(request.Tool.Limits.TimeoutSeconds) * time.Second
	limits := request.Limits
	if limits.Timeout <= 0 || limits.Timeout > approvedTimeout || limits.GracePeriod <= 0 || limits.GracePeriod > 5*time.Second || limits.MaxStdoutBytes <= 0 || limits.MaxStdoutBytes > maxArtifactRead || limits.MaxStderrBytes <= 0 || limits.MaxStderrBytes > maxArtifactRead || limits.MaxNativeBytes <= 0 || limits.MaxNativeBytes > maxArtifactRead || limits.MaxRecords <= 0 || limits.MaxLineBytes <= 0 || int64(limits.MaxLineBytes) > limits.MaxStdoutBytes || int64(limits.MaxLineBytes) > limits.MaxStderrBytes {
		return errors.New("invalid or unapproved runner limits")
	}
	return nil
}

func reservedArtifactName(name string) bool {
	switch name {
	case "stdout.raw", "stderr.raw", "stdout.partial", "stderr.partial", "stdout.interrupted.raw", "stderr.interrupted.raw",
		"artifact-envelope.json", "checksums.sha256", "process-status.json", "command.redacted.txt", "version.txt", "environment.safe.json", completionMarker:
		return true
	default:
		return false
	}
}

func waitForProcess(ctx context.Context, wait <-chan error, pid int, grace time.Duration, killAll func(int) error) (error, error, error) {
	select {
	case err := <-wait:
		return err, nil, nil
	case <-ctx.Done():
		interruption := ctx.Err()
		_ = signalProcessGroup(pid, syscall.SIGTERM)
		_ = signalProcessGroup(pid, syscall.SIGCONT)
		timer := time.NewTimer(grace)
		defer timer.Stop()
		select {
		case err := <-wait:
			return err, interruption, nil
		case <-timer.C:
			_ = signalProcessGroup(pid, syscall.SIGKILL)
			_ = killAll(pid)
			finalTimer := time.NewTimer(grace)
			defer finalTimer.Stop()
			select {
			case err := <-wait:
				return err, interruption, nil
			case <-finalTimer.C:
				return nil, interruption, errors.New("process did not become waitable after forced termination")
			}
		}
	}
}

func buildEnvelope(request Request, environment []string, started, finished time.Time, waitErr, interruption error, leaked bool, artifacts ...Artifact) ArtifactEnvelope {
	envelope := ArtifactEnvelope{
		EnvelopeVersion: envelopeVersion, ExecutionID: request.ExecutionID, WorkspaceRoot: request.WorkspaceRoot, WorkingDirectory: request.OutputDir,
		ToolName: request.Tool.Name, ToolPath: request.Tool.ResolvedPath, ToolVersion: request.Tool.Version, ToolBinary: request.Tool.Binary,
		ActivityClass: request.Tool.ActivityClass, ToolLimits: request.Tool.Limits,
		Argv: redactedArguments(request.Tool.Argv), ArgvSHA256: digestStrings(request.Tool.Argv),
		Environment: redactedEnvironment(environment), EnvironmentSHA256: digestStrings(environment),
		EnvironmentAllowlist: append([]string(nil), request.EnvironmentAllowlist...), RunnerLimits: request.Limits,
		RequireStdout: request.RequireStdout, StdoutIsResult: request.StdoutIsResult, NativeOutputs: append([]NativeOutput(nil), request.NativeOutputs...),
		StartedAt: started.Format(time.RFC3339Nano), FinishedAt: finished.Format(time.RFC3339Nano), DurationMillis: finished.Sub(started).Milliseconds(),
		Status: StatusSuccess, ExitCode: 0, Artifacts: artifacts,
	}
	for _, artifact := range artifacts {
		envelope.Truncated = envelope.Truncated || artifact.Truncated
	}
	if interruption != nil {
		envelope.Status = StatusInterrupted
		envelope.Reason = "cancelled"
		if errors.Is(interruption, context.DeadlineExceeded) {
			envelope.Status = StatusPartial
			envelope.Reason = "timeout"
			envelope.TimedOut = true
		}
	} else if leaked {
		envelope.Status, envelope.Reason = StatusPartial, "descendant_leak"
	} else if envelope.Truncated {
		envelope.Status, envelope.Reason = StatusPartial, "output_limit"
	}
	if waitErr != nil {
		envelope.ExitCode = -1
		if exitError, ok := waitErr.(*exec.ExitError); ok {
			envelope.ExitCode = exitError.ExitCode()
			if status, ok := exitError.Sys().(syscall.WaitStatus); ok && status.Signaled() {
				envelope.Signal = status.Signal().String()
			}
		}
		if interruption == nil && !leaked {
			hasResult := false
			for _, artifact := range artifacts {
				hasResult = hasResult || artifact.Size > 0 && (artifact.Role == "native" || artifact.Role == "stdout" && request.StdoutIsResult)
			}
			if hasResult {
				envelope.Status = StatusPartial
			} else {
				envelope.Status = StatusFailed
			}
			envelope.Reason = "exit_nonzero"
		}
	}
	return envelope
}

func captureNativeOutputs(directory *executionDir, outputs []NativeOutput, maxBytes int64) ([]Artifact, bool, error) {
	artifacts := make([]Artifact, 0, len(outputs))
	missingRequired := false
	for _, output := range outputs {
		exists, err := directory.exists(output.Path)
		if err != nil {
			return artifacts, missingRequired, err
		}
		if !exists {
			missingRequired = missingRequired || output.Required
			continue
		}
		if err := directory.normalizeRegular(output.Path, maxBytes+1); err != nil {
			return artifacts, missingRequired, err
		}
		content, err := directory.read(output.Path, maxBytes+1)
		if err != nil {
			return artifacts, missingRequired, err
		}
		truncated := int64(len(content)) > maxBytes
		if truncated {
			if err := directory.truncateRegular(output.Path, maxBytes); err != nil {
				return artifacts, missingRequired, err
			}
			content = content[:maxBytes]
		}
		digest := sha256.Sum256(content)
		artifacts = append(artifacts, Artifact{Role: "native", Path: output.Path, SHA256: "sha256:" + hex.EncodeToString(digest[:]), Size: int64(len(content)), Truncated: truncated})
	}
	return artifacts, missingRequired, nil
}

func persistEnvelope(directory *executionDir, envelope ArtifactEnvelope) error {
	command, err := canonical.Marshal(redactedArguments(envelope.Argv))
	if err != nil {
		return err
	}
	environment, err := canonical.Marshal(redactedEnvironment(envelope.Environment))
	if err != nil {
		return err
	}
	encoded, err := canonical.Marshal(envelope)
	if err != nil {
		return err
	}
	files := []struct {
		name string
		data []byte
	}{
		{"command.redacted.txt", append(command, '\n')},
		{"version.txt", []byte(envelope.ToolVersion + "\n")},
		{"environment.safe.json", environment},
		{"process-status.json", encoded},
		{"artifact-envelope.json", encoded},
	}
	for _, file := range files {
		if err := directory.writeExclusive(file.name, file.data); err != nil {
			return err
		}
	}
	var checksums strings.Builder
	for _, artifact := range envelope.Artifacts {
		fmt.Fprintf(&checksums, "%s  %s\n", strings.TrimPrefix(artifact.SHA256, "sha256:"), artifact.Path)
	}
	if err := directory.writeExclusive("checksums.sha256", []byte(checksums.String())); err != nil {
		return err
	}
	if err := directory.sync(); err != nil {
		return err
	}
	if err := directory.writeExclusive(completionMarker, []byte(completionMarkerContent)); err != nil {
		return err
	}
	return directory.sync()
}

func redactedArguments(arguments []string) []string {
	redacted := make([]string, len(arguments))
	redactNext := false
	for index, argument := range arguments {
		if redactNext {
			redacted[index] = "[REDACTED]"
			redactNext = false
			continue
		}
		if key, value, found := strings.Cut(argument, "="); found {
			if sensitiveName(key) {
				redacted[index] = key + "=[REDACTED]"
			} else {
				redacted[index] = key + "=" + redactValue(value)
			}
			continue
		}
		if strings.HasPrefix(argument, "-") && sensitiveName(argument) {
			redacted[index] = argument
			redactNext = true
			continue
		}
		redacted[index] = redactValue(argument)
	}
	return redacted
}

func redactedEnvironment(environment []string) []string {
	redacted := make([]string, len(environment))
	for index, entry := range environment {
		key, value, found := strings.Cut(entry, "=")
		if found && sensitiveName(key) {
			redacted[index] = key + "=[REDACTED]"
		} else if found {
			redacted[index] = key + "=" + redactValue(value)
		} else {
			redacted[index] = entry
		}
	}
	return redacted
}

func redactValue(value string) string {
	if name, _, found := strings.Cut(value, ":"); found && sensitiveName(name) {
		return strings.TrimSpace(name) + ": [REDACTED]"
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" {
		return value
	}
	if parsed.User != nil {
		parsed.User = url.User("[REDACTED]")
	}
	query := parsed.Query()
	for key := range query {
		if sensitiveName(key) {
			query.Set(key, "[REDACTED]")
		}
	}
	parsed.RawQuery = query.Encode()
	if fragment, err := url.ParseQuery(parsed.Fragment); err == nil {
		for key := range fragment {
			if sensitiveName(key) {
				fragment.Set(key, "[REDACTED]")
			}
		}
		parsed.Fragment = fragment.Encode()
	}
	return parsed.String()
}

func sensitiveName(name string) bool {
	normalized := strings.ToLower(strings.Trim(strings.TrimSpace(name), "-_"))
	for _, fragment := range []string{"authorization", "cookie", "token", "secret", "password", "passwd", "api-key", "apikey", "access-key", "private-key", "session"} {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}

func digestStrings(values []string) string {
	digest := sha256.New()
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(values)))
	_, _ = digest.Write(length[:])
	for _, value := range values {
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		_, _ = digest.Write(length[:])
		_, _ = digest.Write([]byte(value))
	}
	return "sha256:" + hex.EncodeToString(digest.Sum(nil))
}

func newBoundedCapture(directory *executionDir, name string, maxBytes int64, maxRecords, maxLine int) (*boundedCapture, error) {
	file, err := directory.create(name)
	if err != nil {
		return nil, err
	}
	return &boundedCapture{file: file, hash: sha256.New(), maxBytes: maxBytes, maxRecords: maxRecords, maxLine: maxLine}, nil
}

func (capture *boundedCapture) Write(data []byte) (int, error) {
	for _, value := range data {
		if capture.truncated || capture.writeErr != nil {
			break
		}
		if value == '\n' {
			if capture.records >= capture.maxRecords {
				capture.pending = nil
				capture.truncated = true
				break
			}
			capture.pending = append(capture.pending, value)
			capture.writeBuffered(capture.pending)
			capture.pending = nil
			capture.records++
			continue
		}
		if len(capture.pending) >= capture.maxLine {
			capture.pending = nil
			capture.truncated = true
			break
		}
		capture.pending = append(capture.pending, value)
	}
	return len(data), nil
}

func (capture *boundedCapture) writeBuffered(data []byte) {
	remaining := capture.maxBytes - capture.written
	allowed := len(data)
	if int64(allowed) > remaining {
		allowed = int(remaining)
		capture.truncated = true
	}
	if allowed == 0 {
		return
	}
	written, err := capture.file.Write(data[:allowed])
	if written > 0 {
		_, _ = capture.hash.Write(data[:written])
		capture.written += int64(written)
	}
	if err != nil || written != allowed {
		capture.writeErr = errors.Join(err, ioErrShortWrite(written, allowed))
	}
}

func ioErrShortWrite(written, wanted int) error {
	if written == wanted {
		return nil
	}
	return errors.New("short artifact write")
}

func (capture *boundedCapture) finalize(directory *executionDir, partial, final, role string) (Artifact, error) {
	if len(capture.pending) > 0 && !capture.truncated && capture.writeErr == nil {
		if capture.records >= capture.maxRecords {
			capture.truncated = true
		} else {
			capture.writeBuffered(capture.pending)
			capture.records++
		}
		capture.pending = nil
	}
	if capture.writeErr != nil {
		capture.file.Close()
		return Artifact{}, capture.writeErr
	}
	if err := capture.file.Sync(); err != nil {
		capture.file.Close()
		return Artifact{}, err
	}
	if err := capture.file.Close(); err != nil {
		return Artifact{}, err
	}
	if err := directory.rename(partial, final); err != nil {
		return Artifact{}, err
	}
	return Artifact{Role: role, Path: final, SHA256: "sha256:" + hex.EncodeToString(capture.hash.Sum(nil)), Size: capture.written, Truncated: capture.truncated}, nil
}
