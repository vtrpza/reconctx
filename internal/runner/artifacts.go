package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"sort"

	"github.com/vtrpza/reconctx/internal/model"
	"github.com/vtrpza/reconctx/internal/preflight"
)

const (
	StatusSuccess     = "success"
	StatusPartial     = "partial"
	StatusFailed      = "failed"
	StatusInterrupted = "interrupted"

	envelopeVersion         = "reconctx-artifact-envelope/v0"
	maxArtifactRead         = 128 << 20
	completionMarker        = "COMMITTED"
	completionMarkerContent = "reconctx-run-complete/v0\n"
)

type Artifact struct {
	Role      string `json:"role"`
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
	Truncated bool   `json:"truncated"`
}

type ArtifactEnvelope struct {
	EnvelopeVersion      string           `json:"envelope_version"`
	ExecutionID          string           `json:"execution_id"`
	WorkspaceRoot        string           `json:"workspace_root"`
	WorkingDirectory     string           `json:"working_directory"`
	ToolName             string           `json:"tool_name"`
	ToolPath             string           `json:"tool_path"`
	ToolVersion          string           `json:"tool_version"`
	ToolBinary           model.ToolBinary `json:"tool_binary"`
	ActivityClass        string           `json:"activity_class"`
	ToolLimits           model.ToolLimits `json:"tool_limits"`
	Argv                 []string         `json:"argv"`
	ArgvSHA256           string           `json:"argv_sha256"`
	Environment          []string         `json:"environment"`
	EnvironmentSHA256    string           `json:"environment_sha256"`
	EnvironmentAllowlist []string         `json:"environment_allowlist"`
	RunnerLimits         Limits           `json:"runner_limits"`
	RequireStdout        bool             `json:"require_stdout"`
	StdoutIsResult       bool             `json:"stdout_is_result"`
	NativeOutputs        []NativeOutput   `json:"native_outputs,omitempty"`
	StartedAt            string           `json:"started_at"`
	FinishedAt           string           `json:"finished_at"`
	DurationMillis       int64            `json:"duration_millis"`
	Status               string           `json:"status"`
	Reason               string           `json:"reason,omitempty"`
	ExitCode             int              `json:"exit_code"`
	Signal               string           `json:"signal,omitempty"`
	TimedOut             bool             `json:"timed_out"`
	Truncated            bool             `json:"truncated"`
	Artifacts            []Artifact       `json:"artifacts"`
}

type Result struct {
	Envelope ArtifactEnvelope
}

func Reusable(outputDir string, request Request) (bool, error) {
	if err := validateRequest(request); err != nil {
		return false, err
	}
	expectedEnvironment, err := preflight.FilterEnvironment(request.Environment, request.EnvironmentAllowlist)
	if err != nil {
		return false, err
	}
	directory, err := openExecutionDir(outputDir)
	if err != nil {
		return false, err
	}
	defer directory.close()
	marker, err := directory.read(completionMarker, int64(len(completionMarkerContent)))
	if err != nil || string(marker) != completionMarkerContent {
		return false, nil
	}
	encoded, err := directory.read("artifact-envelope.json", 1<<20)
	if err != nil {
		return false, err
	}
	var envelope ArtifactEnvelope
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		return false, err
	}
	tool := request.Tool
	if envelope.EnvelopeVersion != envelopeVersion || envelope.Status != StatusSuccess || envelope.ExecutionID != request.ExecutionID || envelope.WorkspaceRoot != request.WorkspaceRoot || envelope.WorkingDirectory != request.OutputDir || envelope.ToolName != tool.Name || envelope.ToolPath != tool.ResolvedPath || envelope.ToolVersion != tool.Version || envelope.ToolBinary != tool.Binary || envelope.ActivityClass != tool.ActivityClass || envelope.ToolLimits != tool.Limits || !slices.Equal(envelope.Argv, redactedArguments(tool.Argv)) || envelope.ArgvSHA256 != digestStrings(tool.Argv) || !slices.Equal(envelope.Environment, redactedEnvironment(expectedEnvironment)) || envelope.EnvironmentSHA256 != digestStrings(expectedEnvironment) || !slices.Equal(envelope.EnvironmentAllowlist, request.EnvironmentAllowlist) || envelope.RunnerLimits != request.Limits || envelope.RequireStdout != request.RequireStdout || envelope.StdoutIsResult != request.StdoutIsResult || !slices.Equal(envelope.NativeOutputs, request.NativeOutputs) {
		return false, nil
	}
	identity, err := preflight.ResolveTool(tool.ResolvedPath)
	if err != nil || !sameBinary(identity, tool) {
		return false, nil
	}
	if len(envelope.Artifacts) < 2 || len(envelope.Artifacts) > 2+len(request.NativeOutputs) || envelope.Artifacts[0].Role != "stdout" || envelope.Artifacts[0].Path != "stdout.raw" || envelope.Artifacts[1].Role != "stderr" || envelope.Artifacts[1].Path != "stderr.raw" {
		return false, nil
	}
	expectedNative := make(map[string]NativeOutput, len(request.NativeOutputs))
	seenNative := make(map[string]bool, len(request.NativeOutputs))
	for _, output := range request.NativeOutputs {
		expectedNative[output.Path] = output
	}
	for index, artifact := range envelope.Artifacts {
		if index >= 2 {
			if artifact.Role != "native" {
				return false, nil
			}
			if _, ok := expectedNative[artifact.Path]; !ok || seenNative[artifact.Path] {
				return false, nil
			}
			seenNative[artifact.Path] = true
		}
		if artifact.Size < 0 || artifact.Size > maxArtifactRead || artifact.Role == "native" && artifact.Size > request.Limits.MaxNativeBytes {
			return false, nil
		}
		content, err := directory.read(artifact.Path, artifact.Size)
		if err != nil || int64(len(content)) != artifact.Size {
			return false, nil
		}
		digest := sha256.Sum256(content)
		if "sha256:"+hex.EncodeToString(digest[:]) != artifact.SHA256 {
			return false, nil
		}
	}
	for _, output := range request.NativeOutputs {
		if output.Required && !seenNative[output.Path] {
			return false, nil
		}
	}
	return true, nil
}

func RecoverPartial(outputDir string) ([]string, error) {
	directory, err := openExecutionDir(outputDir)
	if err != nil {
		return nil, err
	}
	defer directory.close()
	mapping := map[string]string{
		"stderr.partial": "stderr.interrupted.raw",
		"stdout.partial": "stdout.interrupted.raw",
	}
	sources := make([]string, 0, len(mapping))
	for source := range mapping {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	recovered := make([]string, 0, len(sources))
	for _, source := range sources {
		exists, err := directory.exists(source)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		if _, err := directory.read(source, maxArtifactRead); err != nil {
			return nil, fmt.Errorf("recover %s: %w", source, err)
		}
		destination := mapping[source]
		if err := directory.rename(source, destination); err != nil {
			return nil, err
		}
		recovered = append(recovered, destination)
	}
	return recovered, nil
}

func sameBinary(identity preflight.ToolIdentity, tool model.ToolPlan) bool {
	return identity.ResolvedPath == tool.ResolvedPath && identity.SHA256 == tool.Binary.SHA256 && uint32(identity.Mode) == tool.Binary.Mode && identity.UID == tool.Binary.UID && identity.GID == tool.Binary.GID && identity.Device == tool.Binary.Device && identity.Inode == tool.Binary.Inode
}
