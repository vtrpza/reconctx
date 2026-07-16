//go:build linux

package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/vtrpza/reconctx/internal/model"
	"github.com/vtrpza/reconctx/internal/preflight"
)

func TestRunnerCapturesImmutableArtifactAndReusesVerifiedSuccess(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "shell-injection")
	argument := "$(touch " + marker + ")"
	tool := runnerTestTool(t, "printf '%s\\n' \"$1\"\nprintf 'warn\\n' >&2\n", argument)
	output := filepath.Join(privateParent(t), "tx_success")
	request := testRequest(output, tool)
	redirected := request
	redirected.OutputDir = filepath.Join(request.WorkspaceRoot, "tx_unapproved")
	if _, err := Run(context.Background(), redirected); err == nil {
		t.Fatal("runner accepted output directory outside approved paths")
	}
	result, err := Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Envelope.Status != StatusSuccess || result.Envelope.ExitCode != 0 {
		t.Fatalf("envelope = %#v", result.Envelope)
	}
	stdout, err := os.ReadFile(filepath.Join(output, "stdout.raw"))
	if err != nil {
		t.Fatal(err)
	}
	if string(stdout) != argument+"\n" {
		t.Fatalf("stdout = %q", stdout)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("argument was interpreted by a shell")
	}
	if info, err := os.Stat(filepath.Join(output, "artifact-envelope.json")); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("envelope mode = %v, %v", info, err)
	}
	encoded, err := os.ReadFile(filepath.Join(output, "artifact-envelope.json"))
	if err != nil {
		t.Fatal(err)
	}
	var persisted ArtifactEnvelope
	if err := json.Unmarshal(encoded, &persisted); err != nil || persisted.ToolBinary != tool.Binary {
		t.Fatalf("persisted envelope = %#v, %v", persisted, err)
	}
	reusable, err := Reusable(output, request)
	if err != nil || !reusable {
		t.Fatalf("reusable = %t, %v", reusable, err)
	}
	if err := os.Remove(filepath.Join(output, completionMarker)); err != nil {
		t.Fatal(err)
	}
	if reusable, err := Reusable(output, request); err != nil || reusable {
		t.Fatalf("uncommitted reusable = %t, %v", reusable, err)
	}
	if err := os.WriteFile(filepath.Join(output, completionMarker), []byte(completionMarkerContent), 0o600); err != nil {
		t.Fatal(err)
	}
	changedRequest := request
	changedRequest.RequireStdout = true
	if reusable, err := Reusable(output, changedRequest); err != nil || reusable {
		t.Fatalf("changed request reusable = %t, %v", reusable, err)
	}
	if err := os.WriteFile(filepath.Join(output, "stdout.raw"), []byte("tampered\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if reusable, err := Reusable(output, request); err != nil || reusable {
		t.Fatalf("tampered reusable = %t, %v", reusable, err)
	}
	if _, err := Run(context.Background(), request); err == nil {
		t.Fatal("runner reused finalized output directory")
	}
}

func TestRunnerRedactsSensitiveCommandAndEnvironmentArtifacts(t *testing.T) {
	tool := runnerTestTool(t, "exit 0\n", "--api-key", "argv-secret", "https://fixture.test/?token=query-secret&safe=value", "https://callback.test/#access_token=fragment-secret&state=ok", "Authorization: Bearer ***")
	output := filepath.Join(privateParent(t), "tx_redacted")
	request := testRequest(output, tool)
	request.Environment = append(request.Environment, "API_TOKEN=environment-secret")
	request.EnvironmentAllowlist = append(request.EnvironmentAllowlist, "API_TOKEN")
	if _, err := Run(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	command, err := os.ReadFile(filepath.Join(output, "command.redacted.txt"))
	if err != nil {
		t.Fatal(err)
	}
	environment, err := os.ReadFile(filepath.Join(output, "environment.safe.json"))
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := os.ReadFile(filepath.Join(output, "artifact-envelope.json"))
	if err != nil {
		t.Fatal(err)
	}
	status, err := os.ReadFile(filepath.Join(output, "process-status.json"))
	if err != nil {
		t.Fatal(err)
	}
	combined := string(command) + string(environment) + string(envelope) + string(status)
	for _, secret := range []string{"argv-secret", "query-secret", "fragment-secret", "header-secret", "environment-secret"} {
		if strings.Contains(combined, secret) {
			t.Fatalf("redacted artifacts contain %q: %s", secret, combined)
		}
	}
	if !strings.Contains(combined, "[REDACTED]") {
		t.Fatalf("redaction marker absent: %s", combined)
	}
}

func TestRunnerUsesApprovedExecutionDirectoryAsWorkingDirectory(t *testing.T) {
	tool := runnerTestTool(t, "pwd\n")
	output := filepath.Join(privateParent(t), "tx_workdir")
	request := testRequest(output, tool)
	request.RequireStdout = true
	result, err := Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := os.ReadFile(filepath.Join(output, "stdout.raw"))
	if err != nil {
		t.Fatal(err)
	}
	if string(stdout) != output+"\n" || result.Envelope.WorkingDirectory != output {
		t.Fatalf("cwd = %q, envelope = %#v", stdout, result.Envelope)
	}
}

func TestRunnerRejectsUnsafeOrUncapturedOutputs(t *testing.T) {
	stderrApproved := testRequest(filepath.Join(privateParent(t), "tx_stderr_approved"), runnerTestTool(t, "exit 0\n"))
	directory := path.Dir(stderrApproved.Tool.OutputPaths[0])
	stderrApproved.Tool.OutputPaths = append(stderrApproved.Tool.OutputPaths, path.Join(directory, "stderr.raw"))
	if err := validateRequest(stderrApproved); err != nil {
		t.Fatalf("runner rejected its managed stderr output in the approved plan: %v", err)
	}

	output := filepath.Join(privateParent(t), "tx_checksum_injection")
	request := testRequest(output, runnerTestTool(t, "exit 0\n"))
	unsafeName := "native\ninjected"
	request.Tool.OutputPaths = append(request.Tool.OutputPaths, filepath.ToSlash(filepath.Join(filepath.Base(output), unsafeName)))
	request.NativeOutputs = []NativeOutput{{Path: unsafeName}}
	if _, err := Run(context.Background(), request); err == nil {
		t.Fatal("runner accepted line break in artifact name")
	}
	omitted := testRequest(filepath.Join(privateParent(t), "tx_omitted_native"), runnerTestTool(t, "exit 0\n"))
	omitted.Tool.OutputPaths = append(omitted.Tool.OutputPaths, filepath.ToSlash(filepath.Join(filepath.Base(omitted.OutputDir), "native.json")))
	if _, err := Run(context.Background(), omitted); err == nil {
		t.Fatal("runner accepted approved native output omitted from capture")
	}

	tool := runnerTestTool(t, "exit 0\n")
	for _, test := range []struct {
		name   string
		mutate func(*Request)
	}{
		{name: "traversing approved path", mutate: func(request *Request) {
			request.Tool.OutputPaths[0] = "../stdout.raw"
		}},
		{name: "backslash in approved path", mutate: func(request *Request) {
			request.Tool.OutputPaths[0] = `tx_fixture\stdout.raw`
		}},
		{name: "duplicate approved path", mutate: func(request *Request) {
			request.Tool.OutputPaths = append(request.Tool.OutputPaths, request.Tool.OutputPaths[0])
		}},
		{name: "different approved directories", mutate: func(request *Request) {
			request.Tool.OutputPaths = append(request.Tool.OutputPaths, "other/native.json")
			request.NativeOutputs = []NativeOutput{{Path: "native.json"}}
		}},
		{name: "unapproved native output", mutate: func(request *Request) {
			request.NativeOutputs = []NativeOutput{{Path: "native.json"}}
		}},
		{name: "reserved native output", mutate: func(request *Request) {
			directory := path.Dir(request.Tool.OutputPaths[0])
			request.Tool.OutputPaths = append(request.Tool.OutputPaths, path.Join(directory, "checksums.sha256"))
			request.NativeOutputs = []NativeOutput{{Path: "checksums.sha256"}}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := testRequest(filepath.Join(privateParent(t), "tx_unsafe"), tool)
			test.mutate(&request)
			if err := validateRequest(request); err == nil {
				t.Fatal("runner accepted unsafe output contract")
			}
		})
	}
}

func TestRunnerBindsTotalDeadlineToApprovedExecutionTimeout(t *testing.T) {
	tool := runnerTestTool(t, "exit 0\n")
	request := testRequest(filepath.Join(privateParent(t), "tx_execution_timeout"), tool)
	request.Limits.Timeout = time.Duration(tool.Limits.ExecutionTimeoutSeconds) * time.Second
	if err := validateRequest(request); err != nil {
		t.Fatalf("runner rejected exact approved execution timeout: %v", err)
	}
	request.Limits.Timeout++
	if err := validateRequest(request); err == nil {
		t.Fatal("runner accepted deadline above approved execution timeout")
	}
}

func TestRunnerTimeoutKillsProcessGroupAndMarksPartial(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	tool := runnerTestTool(t, "trap '' TERM\nsh -c 'pidfile=$1; read stat < /proc/self/stat; set -- $stat; printf %s \"$1\" > \"$pidfile\"; trap \"\" TERM; while :; do sleep 1; done' sh \"$1\" &\nwhile [ ! -s \"$1\" ]; do sleep 0.01; done\nwhile :; do sleep 1; done\n", pidFile)
	request := testRequest(filepath.Join(privateParent(t), "tx_timeout"), tool)
	request.Limits.Timeout = 100 * time.Millisecond
	request.Limits.GracePeriod = 50 * time.Millisecond
	result, err := Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Envelope.Status != StatusPartial || result.Envelope.Reason != "timeout" {
		t.Fatalf("timeout envelope = %#v", result.Envelope)
	}
	pidBytes, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(string(pidBytes))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for syscall.Kill(pid, 0) == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("grandchild %d survived: %v", pid, err)
	}
}

func TestRunnerCancellationIsInterrupted(t *testing.T) {
	tool := runnerTestTool(t, "trap '' TERM\nwhile :; do sleep 1; done\n")
	request := testRequest(filepath.Join(privateParent(t), "tx_cancel"), tool)
	request.Limits.GracePeriod = 25 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	first := time.AfterFunc(50*time.Millisecond, cancel)
	second := time.AfterFunc(60*time.Millisecond, cancel)
	defer first.Stop()
	defer second.Stop()
	started := time.Now()
	result, err := Run(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Envelope.Status != StatusInterrupted || result.Envelope.Reason != "cancelled" {
		t.Fatalf("cancel envelope = %#v", result.Envelope)
	}
	if time.Since(started) > time.Second {
		t.Fatal("repeated cancellation did not remain bounded")
	}
}

func TestWaitForProcessBoundsFinalWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	_, _, err := waitForProcess(ctx, make(chan error), os.Getpid(), 10*time.Millisecond, func(int) error {
		return errors.New("fixture kill failure")
	})
	if err == nil || time.Since(started) > 100*time.Millisecond {
		t.Fatalf("final wait was not bounded: %v", err)
	}
}

func TestHelperMarkerAloneDoesNotActivateHelperMode(t *testing.T) {
	command := exec.Command(os.Args[0], "-test.run=^$")
	command.Env = append(os.Environ(), helperEnvironment+"=attacker-controlled")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("ordinary invocation entered helper mode: %v: %s", err, output)
	}
}

func TestRunnerCancellationContinuesStoppedProcessForGracefulExit(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "terminated")
	tool := runnerTestTool(t, "stopped() {\n  trap 'printf terminated > \"$1\"; exit 0' TERM\n  : > \"$1.ready\"\n  while :; do :; done\n}\nstopped \"$1\" &\nchild=$!\nwhile [ ! -e \"$1.ready\" ]; do sleep 0.01; done\ntrap '' TERM\nkill -STOP \"$child\"\n: > \"$1.stopped\"\nwait \"$child\"\n", marker)
	request := testRequest(filepath.Join(privateParent(t), "tx_stopped"), tool)
	request.Limits.GracePeriod = 250 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	type outcome struct {
		result Result
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := Run(ctx, request)
		done <- outcome{result: result, err: err}
	}()
	waitForFile(t, marker+".stopped")
	cancel()
	completed := <-done
	result, err := completed.result, completed.err
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil || result.Envelope.Status != StatusInterrupted {
		t.Fatalf("stopped process did not exit gracefully: %#v, %v", result.Envelope, err)
	}
}

func TestRunnerContainsNewSessionBeforeHashingNativeArtifacts(t *testing.T) {
	output := filepath.Join(privateParent(t), "tx_setsid")
	nativePath := filepath.Join(output, "native.json")
	pidFile := filepath.Join(t.TempDir(), "daemon.pid")
	tool := runnerTestTool(t, "setsid sh -c 'native=$1; pidfile=$2; read stat < /proc/self/stat; set -- $stat; printf %s \"$1\" > \"$pidfile\"; trap \"\" TERM; while :; do printf x >> \"$native\"; sleep 0.01; done' sh \"$1\" \"$2\" >/dev/null 2>&1 &\nwhile [ ! -s \"$2\" ]; do sleep 0.01; done\n", nativePath, pidFile)
	tool.OutputPaths = []string{"native.json"}
	request := testRequest(output, tool)
	request.NativeOutputs = []NativeOutput{{Path: "native.json", Required: true}}
	result, err := Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	pidBytes, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(string(pidBytes))
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Kill(pid, syscall.SIGKILL)
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("new-session descendant %d survived: %v", pid, err)
	}
	before, err := os.ReadFile(nativePath)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	after, err := os.ReadFile(nativePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || result.Envelope.Status == StatusFailed {
		t.Fatalf("native artifact changed after hash or execution failed: %#v", result.Envelope)
	}
}

func TestRunnerKillsAndClassifiesDescendantsLeftAfterMainExit(t *testing.T) {
	for _, exitCode := range []int{0, 3} {
		t.Run(strconv.Itoa(exitCode), func(t *testing.T) {
			tool := runnerTestTool(t, "sh -c 'sleep 1' &\nexit "+strconv.Itoa(exitCode)+"\n")
			request := testRequest(filepath.Join(privateParent(t), "tx_descendant"), tool)
			result, err := Run(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			if result.Envelope.Status != StatusPartial || result.Envelope.Reason != "descendant_leak" || result.Envelope.ExitCode != exitCode {
				t.Fatalf("descendant envelope = %#v", result.Envelope)
			}
		})
	}
}

func TestRunnerPreservesSignaledToolStatus(t *testing.T) {
	tool := runnerTestTool(t, "kill -TERM $$\n")
	request := testRequest(filepath.Join(privateParent(t), "tx_signaled"), tool)
	result, err := Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Envelope.Status != StatusFailed || result.Envelope.Reason != "exit_nonzero" || result.Envelope.ExitCode != -1 || result.Envelope.Signal != syscall.SIGTERM.String() {
		t.Fatalf("signaled envelope = %#v", result.Envelope)
	}
}

func TestRunnerCapturesAndVerifiesNativeOutputs(t *testing.T) {
	output := filepath.Join(privateParent(t), "tx_native")
	nativePath := filepath.Join(output, "native.json")
	tool := runnerTestTool(t, "printf '{\"ok\":true}\\n' > \"$1\"\n", nativePath)
	tool.OutputPaths = []string{"native.json"}
	request := testRequest(output, tool)
	request.NativeOutputs = []NativeOutput{{Path: "native.json", Required: true}}
	result, err := Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Envelope.Status != StatusSuccess || len(result.Envelope.Artifacts) != 3 || result.Envelope.Artifacts[2].Path != "native.json" {
		t.Fatalf("native envelope = %#v", result.Envelope)
	}
	if reusable, err := Reusable(output, request); err != nil || !reusable {
		t.Fatalf("native reusable = %t, %v", reusable, err)
	}
	if err := os.WriteFile(nativePath, []byte("tampered\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if reusable, err := Reusable(output, request); err != nil || reusable {
		t.Fatalf("tampered native reusable = %t, %v", reusable, err)
	}

	missingOutput := filepath.Join(privateParent(t), "tx_missing_native")
	missingTool := runnerTestTool(t, "exit 0\n")
	missingTool.OutputPaths = []string{"native.json"}
	missingRequest := testRequest(missingOutput, missingTool)
	missingRequest.NativeOutputs = []NativeOutput{{Path: "native.json", Required: true}}
	missing, err := Run(context.Background(), missingRequest)
	if err != nil {
		t.Fatal(err)
	}
	if missing.Envelope.Status != StatusFailed || missing.Envelope.Reason != "missing_required_output" {
		t.Fatalf("missing native envelope = %#v", missing.Envelope)
	}

	reservedTool := runnerTestTool(t, "exit 0\n")
	reservedTool.OutputPaths = []string{"artifact-envelope.json"}
	reservedRequest := testRequest(filepath.Join(privateParent(t), "tx_reserved"), reservedTool)
	reservedRequest.NativeOutputs = []NativeOutput{{Path: "artifact-envelope.json", Required: true}}
	if _, err := Run(context.Background(), reservedRequest); err == nil {
		t.Fatal("runner accepted native output colliding with metadata")
	}
}

func TestRunnerPersistsFailedStatusForInvalidNativeArtifact(t *testing.T) {
	output := filepath.Join(privateParent(t), "tx_invalid_native")
	nativePath := filepath.Join(output, "native.json")
	tool := runnerTestTool(t, "mkdir \"$1\"\n", nativePath)
	tool.OutputPaths = []string{"native.json"}
	request := testRequest(output, tool)
	request.NativeOutputs = []NativeOutput{{Path: "native.json", Required: true}}
	result, err := Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Envelope.Status != StatusFailed || result.Envelope.Reason != "native_artifact_invalid" {
		t.Fatalf("invalid native envelope = %#v", result.Envelope)
	}
	for _, name := range []string{"process-status.json", "artifact-envelope.json", completionMarker} {
		if _, err := os.Stat(filepath.Join(output, name)); err != nil {
			t.Fatalf("missing persisted status %s: %v", name, err)
		}
	}
}

func TestRunnerBoundsNativeOutputDuringExecution(t *testing.T) {
	output := filepath.Join(privateParent(t), "tx_native_limit")
	nativePath := filepath.Join(output, "native.json")
	tool := runnerTestTool(t, "sh -c 'set -e; i=0; while [ \"$i\" -lt 10000 ]; do printf 0123456789abcdef >> \"$1\"; i=$((i+1)); done' sh \"$1\" &\nchild=$!\nwait \"$child\"\n", nativePath)
	tool.OutputPaths = []string{"native.json"}
	request := testRequest(output, tool)
	request.NativeOutputs = []NativeOutput{{Path: "native.json", Required: true}}
	request.Limits.MaxNativeBytes = 64
	result, err := Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(nativePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > request.Limits.MaxNativeBytes || result.Envelope.Status == StatusSuccess {
		t.Fatalf("native size = %d, envelope = %#v", info.Size(), result.Envelope)
	}

	exactOutput := filepath.Join(privateParent(t), "tx_native_exact")
	exactPath := filepath.Join(exactOutput, "native.json")
	exactTool := runnerTestTool(t, "printf '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef' > \"$1\"\n", exactPath)
	exactTool.OutputPaths = []string{"native.json"}
	exactRequest := testRequest(exactOutput, exactTool)
	exactRequest.NativeOutputs = []NativeOutput{{Path: "native.json", Required: true}}
	exactRequest.Limits.MaxNativeBytes = 64
	exact, err := Run(context.Background(), exactRequest)
	if err != nil {
		t.Fatal(err)
	}
	if exact.Envelope.Status != StatusSuccess || exact.Envelope.Artifacts[2].Truncated {
		t.Fatalf("exact-boundary native output = %#v", exact.Envelope)
	}

	caughtOutput := filepath.Join(privateParent(t), "tx_native_caught_limit")
	caughtPath := filepath.Join(caughtOutput, "native.json")
	caughtTool := runnerTestTool(t, "set +e\ntrap '' XFSZ\ni=0\nwhile [ \"$i\" -lt 100 ]; do printf 0123456789abcdef >> \"$1\"; i=$((i+1)); done\nexit 0\n", caughtPath)
	caughtTool.OutputPaths = []string{"native.json"}
	caughtRequest := testRequest(caughtOutput, caughtTool)
	caughtRequest.NativeOutputs = []NativeOutput{{Path: "native.json", Required: true}}
	caughtRequest.Limits.MaxNativeBytes = 64
	caught, err := Run(context.Background(), caughtRequest)
	if err != nil {
		t.Fatal(err)
	}
	info, err = os.Stat(caughtPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 64 || caught.Envelope.Status != StatusPartial || !caught.Envelope.Artifacts[2].Truncated {
		t.Fatalf("caught overflow size = %d, envelope = %#v", info.Size(), caught.Envelope)
	}
}

func TestRunnerBoundsOutputAndRecoversPartial(t *testing.T) {
	tool := runnerTestTool(t, "i=0\nwhile [ \"$i\" -lt 8 ]; do printf '0123456789abcdef\\n'; i=$((i+1)); done\n")
	output := filepath.Join(privateParent(t), "tx_bounded")
	request := testRequest(output, tool)
	request.Limits.MaxStdoutBytes = 32
	request.Limits.MaxRecords = 2
	request.Limits.MaxLineBytes = 8
	result, err := Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Envelope.Status != StatusPartial || !result.Envelope.Truncated {
		t.Fatalf("bounded envelope = %#v", result.Envelope)
	}
	if info, err := os.Stat(filepath.Join(output, "stdout.raw")); err != nil || info.Size() > 32 {
		t.Fatalf("stdout size = %v, %v", info, err)
	}
	invalid := testRequest(filepath.Join(privateParent(t), "tx_line_limit"), tool)
	invalid.Limits.MaxLineBytes = int(invalid.Limits.MaxStdoutBytes) + 1
	if _, err := Run(context.Background(), invalid); err == nil {
		t.Fatal("runner accepted line buffer above artifact byte cap")
	}

	recovery := filepath.Join(privateParent(t), "tx_recovery")
	if err := os.Mkdir(recovery, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"stdout.partial", "stderr.partial"} {
		if err := os.WriteFile(filepath.Join(recovery, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	recovered, err := RecoverPartial(recovery)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"stderr.interrupted.raw", "stdout.interrupted.raw"}
	if strings.Join(recovered, ",") != strings.Join(want, ",") {
		t.Fatalf("recovered = %v", recovered)
	}
	if recovered, err := RecoverPartial(recovery); err != nil || len(recovered) != 0 {
		t.Fatalf("second recovery = %v, %v", recovered, err)
	}
}

func TestRunnerRecordCapDropsWholeExcessRecord(t *testing.T) {
	tool := runnerTestTool(t, "printf 'one\\ntwo\\nEXCESS'\n")
	output := filepath.Join(privateParent(t), "tx_records")
	request := testRequest(output, tool)
	request.Limits.MaxRecords = 2
	result, err := Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := os.ReadFile(filepath.Join(output, "stdout.raw"))
	if err != nil {
		t.Fatal(err)
	}
	if string(stdout) != "one\ntwo\n" || result.Envelope.Status != StatusPartial {
		t.Fatalf("record-capped stdout = %q, envelope = %#v", stdout, result.Envelope)
	}
}

func TestRunnerNeverCallsIncompleteExecutionSuccess(t *testing.T) {
	tests := []struct {
		name           string
		script         string
		stdoutIsResult bool
		status         string
		reason         string
	}{
		{name: "missing required stdout", script: "exit 0\n", status: StatusFailed, reason: "missing_required_stdout"},
		{name: "nonzero partial result", script: "printf 'partial\\n'\nexit 3\n", stdoutIsResult: true, status: StatusPartial, reason: "exit_nonzero"},
		{name: "diagnostic stderr", script: "printf 'request failed\\n' >&2\nexit 1\n", status: StatusFailed, reason: "exit_nonzero"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tool := runnerTestTool(t, test.script)
			request := testRequest(filepath.Join(privateParent(t), "tx_case"), tool)
			request.RequireStdout = true
			request.StdoutIsResult = test.stdoutIsResult
			result, err := Run(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			if result.Envelope.Status != test.status || result.Envelope.Reason != test.reason {
				t.Fatalf("envelope = %#v", result.Envelope)
			}
		})
	}
}

func TestRunnerRunAllIsBoundedAndOrdered(t *testing.T) {
	coordination := privateParent(t)
	tool := runnerTestTool(t, ": > \"$1/$2.started\"\nwhile [ ! -e \"$1/$2.release\" ]; do sleep 0.01; done\nprintf '%s\\n' \"$2\"\n")
	tool.Limits.Parallelism = 2
	outputParent := privateParent(t)
	requests := make([]Request, 3)
	for index := range requests {
		copy := tool
		copy.Argv = []string{tool.ResolvedPath, coordination, strconv.Itoa(index)}
		requests[index] = testRequest(filepath.Join(outputParent, "tx_"+strconv.Itoa(index)), copy)
		requests[index].Limits.Timeout = 2 * time.Second
	}
	type outcome struct {
		results []Result
		err     error
	}
	done := make(chan outcome, 1)
	go func() {
		results, err := RunAll(context.Background(), requests, 2)
		done <- outcome{results: results, err: err}
	}()
	waitForFile(t, filepath.Join(coordination, "0.started"))
	waitForFile(t, filepath.Join(coordination, "1.started"))
	if _, err := os.Stat(filepath.Join(coordination, "2.started")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("third execution started above approved parallelism")
	}
	if err := os.WriteFile(filepath.Join(coordination, "0.release"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	waitForFile(t, filepath.Join(coordination, "2.started"))
	for _, index := range []int{1, 2} {
		if err := os.WriteFile(filepath.Join(coordination, strconv.Itoa(index)+".release"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	completed := <-done
	if completed.err != nil {
		t.Fatal(completed.err)
	}
	for index, result := range completed.results {
		if result.Envelope.Status != StatusSuccess {
			t.Fatalf("result %d = %#v", index, result)
		}
		stdout, err := os.ReadFile(filepath.Join(requests[index].OutputDir, "stdout.raw"))
		if err != nil || string(stdout) != strconv.Itoa(index)+"\n" {
			t.Fatalf("stdout %d = %q, %v", index, stdout, err)
		}
	}
}

func runnerTestTool(t *testing.T, script string, arguments ...string) model.ToolPlan {
	t.Helper()
	directory := privateParent(t)
	path := filepath.Join(directory, "fake-tool")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nset -eu\n"+script), 0o700); err != nil {
		t.Fatal(err)
	}
	identity, err := preflight.ResolveTool(path)
	if err != nil {
		t.Fatal(err)
	}
	return model.ToolPlan{
		Name: "fake", ResolvedPath: identity.ResolvedPath, Version: "1.0.0", ActivityClass: "fixture",
		Binary: model.ToolBinary{SHA256: identity.SHA256, Mode: uint32(identity.Mode), UID: identity.UID, GID: identity.GID, Device: identity.Device, Inode: identity.Inode},
		Argv:   append([]string{identity.ResolvedPath}, arguments...),
		Limits: model.ToolLimits{RatePerSecond: 1, Concurrency: 1, Parallelism: 1, RequestTimeoutSeconds: 2, ExecutionTimeoutSeconds: 3},
	}
}

func testRequest(output string, tool model.ToolPlan) Request {
	relativeDirectory := filepath.Base(output)
	approvedOutputs := []string{filepath.ToSlash(filepath.Join(relativeDirectory, "stdout.raw"))}
	for _, output := range tool.OutputPaths {
		if filepath.Base(output) != "stdout.raw" {
			approvedOutputs = append(approvedOutputs, filepath.ToSlash(filepath.Join(relativeDirectory, filepath.Base(output))))
		}
	}
	tool.OutputPaths = approvedOutputs
	return Request{
		ExecutionID: "tx_fixture", WorkspaceRoot: filepath.Dir(output), OutputDir: output, Tool: tool,
		Environment: os.Environ(), EnvironmentAllowlist: []string{"LANG", "PATH"}, RequireStdout: false,
		Limits: Limits{Timeout: time.Second, GracePeriod: 50 * time.Millisecond, MaxStdoutBytes: 1 << 20, MaxStderrBytes: 1 << 20, MaxNativeBytes: 1 << 20, MaxRecords: 10_000, MaxLineBytes: 64 << 10},
	}
}

func TestRunnerCreatesApprovedNestedExecutionParents(t *testing.T) {
	workspaceRoot := privateParent(t)
	output := filepath.Join(workspaceRoot, "runs", "run_test", "executions", "tx_fixture")
	request := testRequest(output, runnerTestTool(t, "printf nested"))
	request.WorkspaceRoot = workspaceRoot
	request.Tool.OutputPaths = []string{"runs/run_test/executions/tx_fixture/stdout.raw"}

	result, err := Run(context.Background(), request)
	if err != nil || result.Envelope.Status != StatusSuccess {
		t.Fatalf("Run() = %#v, %v", result.Envelope, err)
	}
	stdout, err := os.ReadFile(filepath.Join(output, "stdout.raw"))
	if err != nil || string(stdout) != "nested" {
		t.Fatalf("stdout = %q, %v", stdout, err)
	}
}

func privateParent(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	return directory
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}
