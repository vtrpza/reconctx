package preflight

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"syscall"
	"time"
)

const maxVersionOutputBytes = 64 << 10

var versionPatterns = map[string]*regexp.Regexp{
	"gau":    regexp.MustCompile(`(?mi)^gau(?: version)?\s+v?([0-9]+\.[0-9]+\.[0-9]+)\s*$`),
	"katana": regexp.MustCompile(`(?mi)^(?:\[INF\]\s*)?(?:Current Version:\s*)?(v[0-9]+\.[0-9]+\.[0-9]+)\s*$`),
	"arjun":  regexp.MustCompile(`(?mi)^arjun(?:=|(?: version)?\s+)([0-9]+\.[0-9]+\.[0-9]+)\s*$`),
}

var versionProbeArgs = map[string][]string{
	"gau":    {"--version"},
	"katana": {"-version"},
	"arjun":  {"--version"},
}

var supportedVersions = map[string]string{
	"gau":    "2.2.4",
	"katana": "v1.6.1",
	"arjun":  "2.2.7",
}

type ToolResult struct {
	Identity ToolIdentity
	Version  string
}

func ParseVersion(tool, output string) (string, error) {
	pattern, ok := versionPatterns[tool]
	if !ok {
		return "", fmt.Errorf("unsupported tool %q", tool)
	}
	match := pattern.FindStringSubmatch(output)
	if len(match) != 2 {
		return "", fmt.Errorf("%s version not found in anchored runtime output", tool)
	}
	return match[1], nil
}

func InspectTool(ctx context.Context, name, candidate string, baseEnvironment, allowlist []string) (ToolResult, error) {
	arguments, supported := versionProbeArgs[name]
	if !supported {
		return ToolResult{}, fmt.Errorf("unsupported tool %q", name)
	}
	identity, err := ResolveTool(candidate)
	if err != nil {
		return ToolResult{}, err
	}
	environment, err := FilterEnvironment(baseEnvironment, allowlist)
	if err != nil {
		return ToolResult{}, err
	}

	probeContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	command := exec.Command(identity.ResolvedPath, arguments...)
	command.Dir = filepath.Dir(identity.ResolvedPath)
	command.Env = environment
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var stdout, stderr boundedOutput
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return ToolResult{}, err
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case err = <-done:
	case <-probeContext.Done():
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		<-done
		return ToolResult{}, probeContext.Err()
	}
	if err != nil {
		return ToolResult{}, fmt.Errorf("%s version probe: %w", name, err)
	}
	if stdout.truncated || stderr.truncated {
		return ToolResult{}, fmt.Errorf("%s version output exceeds limit", name)
	}
	version, err := ParseVersion(name, string(stdout.data)+"\n"+string(stderr.data))
	if err != nil {
		return ToolResult{}, err
	}
	if version != supportedVersions[name] {
		return ToolResult{}, fmt.Errorf("unsupported %s version %q", name, version)
	}
	return ToolResult{Identity: identity, Version: version}, nil
}

type boundedOutput struct {
	data      []byte
	truncated bool
}

func (output *boundedOutput) Write(value []byte) (int, error) {
	remaining := maxVersionOutputBytes - len(output.data)
	if remaining > 0 {
		if remaining > len(value) {
			remaining = len(value)
		}
		output.data = append(output.data, value[:remaining]...)
	}
	output.truncated = output.truncated || remaining < len(value)
	return len(value), nil
}
