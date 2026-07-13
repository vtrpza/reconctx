package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type Event struct {
	SourceLine int     `json:"source_line"`
	Timestamp  *string `json:"timestamp"`
	Method     string  `json:"method"`
	URLRaw     string  `json:"url_raw"`
	RouteURL   string  `json:"route_url"`
	StatusCode int     `json:"status_code"`
}

type Summary struct {
	Records      int `json:"records"`
	UniqueRoutes int `json:"unique_routes"`
}

type RunResult struct {
	ExitCode int
	TimedOut bool
	Duration time.Duration
	Stdout   string
	Stderr   string
}

type nativeRecord struct {
	Timestamp *string `json:"timestamp"`
	Request   struct {
		Method   string `json:"method"`
		Endpoint string `json:"endpoint"`
	} `json:"request"`
	Response struct {
		StatusCode int `json:"status_code"`
	} `json:"response"`
}

func routeURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	return parsed.String(), nil
}

func CompileFixture(source, outputDir string) (Summary, error) {
	input, err := os.Open(source)
	if err != nil {
		return Summary{}, err
	}
	defer input.Close()
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return Summary{}, err
	}
	eventsPath := filepath.Join(outputDir, "events.jsonl")
	output, err := os.Create(eventsPath)
	if err != nil {
		return Summary{}, err
	}
	encoder := json.NewEncoder(output)
	routes := map[string]struct{}{}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	records := 0
	for scanner.Scan() {
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		records++
		var native nativeRecord
		if err := json.Unmarshal(scanner.Bytes(), &native); err != nil {
			output.Close()
			return Summary{}, fmt.Errorf("line %d: %w", records, err)
		}
		route, err := routeURL(native.Request.Endpoint)
		if err != nil {
			output.Close()
			return Summary{}, fmt.Errorf("line %d URL: %w", records, err)
		}
		event := Event{
			SourceLine: records,
			Timestamp:  native.Timestamp,
			Method:     native.Request.Method,
			URLRaw:     native.Request.Endpoint,
			RouteURL:   route,
			StatusCode: native.Response.StatusCode,
		}
		if err := encoder.Encode(event); err != nil {
			output.Close()
			return Summary{}, err
		}
		routes[route] = struct{}{}
	}
	if err := scanner.Err(); err != nil {
		output.Close()
		return Summary{}, err
	}
	if err := output.Close(); err != nil {
		return Summary{}, err
	}
	context := fmt.Sprintf("# Stack Spike Context\n\n- Records: %d\n- Unique routes: %d\n", records, len(routes))
	if err := os.WriteFile(filepath.Join(outputDir, "CONTEXT.md"), []byte(context), 0o644); err != nil {
		return Summary{}, err
	}
	return Summary{Records: records, UniqueRoutes: len(routes)}, nil
}

func RunSupervised(argv []string, timeout, grace time.Duration) (RunResult, error) {
	if len(argv) == 0 {
		return RunResult{}, fmt.Errorf("empty argv")
	}
	started := time.Now()
	command := exec.Command(argv[0], argv[1:]...)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return RunResult{}, err
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	timedOut := false
	var waitErr error
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case waitErr = <-done:
	case <-timer.C:
		timedOut = true
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGTERM)
		graceTimer := time.NewTimer(grace)
		select {
		case waitErr = <-done:
			graceTimer.Stop()
		case <-graceTimer.C:
			_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
			waitErr = <-done
		}
	}
	exitCode := 0
	if command.ProcessState != nil {
		exitCode = command.ProcessState.ExitCode()
	} else if waitErr != nil {
		exitCode = -1
	}
	return RunResult{
		ExitCode: exitCode,
		TimedOut: timedOut,
		Duration: time.Since(started),
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}, nil
}

func runCLI(args []string, stdout io.Writer) error {
	if len(args) != 3 || args[0] != "compile" {
		return fmt.Errorf("usage: stack-spike compile <source.jsonl> <output-dir>")
	}
	summary, err := CompileFixture(args[1], args[2])
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(summary)
}

func main() {
	if err := runCLI(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}
