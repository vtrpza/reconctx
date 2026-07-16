//go:build linux

package runner

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/vtrpza/reconctx/internal/model"
	"golang.org/x/sys/unix"
)

const (
	helperEnvironment = "RECONCTX_INTERNAL_EXEC_HELPER"
	helperMagic       = "reconctx-exec-config/v0"
	helperConfigLimit = 1 << 20
	helperReportLimit = 1 << 10
)

type helperConfig struct {
	Magic           string         `json:"magic"`
	Nonce           string         `json:"nonce"`
	Tool            model.ToolPlan `json:"tool"`
	NativeFileLimit int64          `json:"native_file_limit"`
	GracePeriod     time.Duration  `json:"grace_period"`
	Supervise       bool           `json:"supervise"`
}

type helperReport struct {
	Leaked   bool `json:"leaked"`
	ExitCode int  `json:"exit_code"`
	Signal   int  `json:"signal"`
}

type helperExitError struct{ code int }

func (err *helperExitError) Error() string {
	return fmt.Sprintf("tool exited with status %d", err.code)
}

type executionDir struct {
	path string
	fd   int
}

type containment struct {
	mode      uint8
	path      string
	directory *os.File
	report    *os.File
	outcome   *helperReport
}

const (
	containmentCgroup uint8 = iota + 1
	containmentNamespace
)

var containmentSequence atomic.Uint64

func platformSupported() bool { return true }

func init() {
	nonce := os.Getenv(helperEnvironment)
	if nonce == "" {
		return
	}
	if !authenticatedHelperInvocation(nonce) {
		_ = os.Unsetenv(helperEnvironment)
		return
	}
	if err := runExecutionHelper(nonce); err != nil {
		var exit *helperExitError
		if errors.As(err, &exit) {
			// The parent owns race reporting; os.Exit's race-build delay can turn a completed tool into a timeout.
			syscall.Exit(exit.code)
			return
		}
		fmt.Fprintf(os.Stderr, "reconctx execution helper: %v\n", err)
		os.Exit(126)
	}
}

func limitedCommand(request Request, environment []string, contained *containment) (*exec.Cmd, func(), error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return nil, nil, err
	}
	nonce := hex.EncodeToString(random)
	encoded, err := json.Marshal(helperConfig{Magic: helperMagic, Nonce: nonce, Tool: request.Tool, NativeFileLimit: request.Limits.MaxNativeBytes + 1, GracePeriod: request.Limits.GracePeriod, Supervise: contained.mode == containmentNamespace})
	if err != nil {
		return nil, nil, err
	}
	if len(encoded) > helperConfigLimit {
		return nil, nil, errors.New("execution helper configuration is too large")
	}
	toolFile, err := openApprovedTool(request.Tool, true)
	if err != nil {
		return nil, nil, err
	}
	fd, err := unix.MemfdCreate("reconctx-exec", unix.MFD_CLOEXEC)
	if err != nil {
		toolFile.Close()
		return nil, nil, err
	}
	configFile := os.NewFile(uintptr(fd), "reconctx-exec")
	if _, err := configFile.Write(encoded); err != nil {
		configFile.Close()
		toolFile.Close()
		return nil, nil, err
	}
	if _, err := configFile.Seek(0, 0); err != nil {
		configFile.Close()
		toolFile.Close()
		return nil, nil, err
	}
	command := exec.Command("/proc/self/exe")
	command.Dir = request.OutputDir
	command.Env = append(append([]string(nil), environment...), helperEnvironment+"="+nonce)
	command.ExtraFiles = []*os.File{configFile, toolFile}
	var reportWriter *os.File
	if contained.mode == containmentNamespace {
		contained.report, reportWriter, err = os.Pipe()
		if err != nil {
			configFile.Close()
			toolFile.Close()
			return nil, nil, err
		}
		command.ExtraFiles = append(command.ExtraFiles, reportWriter)
	}
	return command, func() {
		_ = configFile.Close()
		_ = toolFile.Close()
		if reportWriter != nil {
			_ = reportWriter.Close()
		}
	}, nil
}

func startLimitedCommand(ctx context.Context, request Request, environment []string, stdout, stderr io.Writer) (*exec.Cmd, *containment, int, error) {
	selected := newContainment()
	for {
		if err := ctx.Err(); err != nil {
			selected.abort()
			return nil, nil, 0, err
		}
		command, closeConfig, err := limitedCommand(request, environment, selected)
		if err != nil {
			selected.abort()
			return nil, nil, 0, err
		}
		configureProcessGroup(command, selected)
		command.Stdout = stdout
		command.Stderr = stderr
		command.WaitDelay = request.Limits.GracePeriod
		err = command.Start()
		closeConfig()
		if err == nil {
			return command, selected, command.Process.Pid, nil
		}
		if selected.mode != containmentCgroup || !containmentStartError(err) {
			selected.abort()
			if selected.mode == containmentNamespace && containmentStartError(err) {
				return nil, nil, 0, fmt.Errorf("%w: cgroup v2 delegation and unprivileged user/PID namespaces are unavailable: %v", ErrContainmentUnavailable, err)
			}
			return nil, nil, 0, err
		}
		selected.abort()
		selected = &containment{mode: containmentNamespace}
	}
}

func newContainment() *containment {
	contained, err := newCgroupContainment()
	if err == nil {
		return contained
	}
	return &containment{mode: containmentNamespace}
}

func newCgroupContainment() (*containment, error) {
	content, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return nil, err
	}
	relative := ""
	found := false
	for _, line := range strings.Split(string(content), "\n") {
		if strings.HasPrefix(line, "0::") {
			relative = strings.TrimPrefix(strings.TrimPrefix(line, "0::"), "/")
			if relative == "" {
				relative = "."
			}
			found = true
			break
		}
	}
	if !found || filepath.Clean(relative) != relative || strings.HasPrefix(relative, "..") {
		return nil, errors.New("current cgroup v2 path is unavailable")
	}
	parent := filepath.Join("/sys/fs/cgroup", relative)
	name := fmt.Sprintf("reconctx-%d-%d", os.Getpid(), containmentSequence.Add(1))
	path := filepath.Join(parent, name)
	if err := os.Mkdir(path, 0o700); err != nil {
		return nil, err
	}
	directory, err := os.Open(path)
	if err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(path, "cgroup.kill"), []byte("1"), 0o200); err != nil {
		directory.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return &containment{mode: containmentCgroup, path: path, directory: directory}, nil
}

func containmentStartError(err error) bool {
	return errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOSYS) || errors.Is(err, syscall.EOPNOTSUPP)
}

func (contained *containment) kill(pid int) error {
	if contained.mode == containmentNamespace {
		return signalProcessGroup(pid, syscall.SIGKILL)
	}
	if err := os.WriteFile(filepath.Join(contained.path, "cgroup.kill"), []byte("1"), 0o200); err == nil {
		return nil
	}
	content, err := os.ReadFile(filepath.Join(contained.path, "cgroup.procs"))
	if err != nil {
		return err
	}
	for _, value := range strings.Fields(string(content)) {
		process, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		if err := syscall.Kill(process, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return err
		}
	}
	return nil
}

func (contained *containment) finish(pid int, grace time.Duration, graceful bool) (bool, error) {
	if contained.mode == containmentNamespace {
		reportErr := contained.readReport()
		if graceful && errors.Is(reportErr, io.EOF) {
			reportErr = nil
		}
		leaked := contained.outcome != nil && contained.outcome.Leaked
		groupLeaked := processGroupExists(pid)
		leaked = leaked || groupLeaked
		if graceful {
			deadline := time.Now().Add(grace)
			for processGroupExists(pid) && time.Now().Before(deadline) {
				time.Sleep(10 * time.Millisecond)
			}
		}
		if groupLeaked {
			_ = contained.kill(pid)
		}
		deadline := time.Now().Add(grace)
		for processGroupExists(pid) && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		if processGroupExists(pid) {
			return leaked, errors.Join(reportErr, errors.New("process group survived containment cleanup"))
		}
		return leaked, reportErr
	}
	populated, stateErr := contained.populated()
	if stateErr != nil {
		populated = true
	}
	leaked := populated
	if graceful {
		deadline := time.Now().Add(grace)
		for populated && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
			next, err := contained.populated()
			if err != nil {
				stateErr = errors.Join(stateErr, err)
				continue
			}
			populated = next
		}
	}
	deadline := time.Now().Add(grace)
	for populated {
		killErr := contained.kill(pid)
		if time.Now().After(deadline) {
			_ = contained.kill(pid)
			if contained.directory != nil {
				_ = contained.directory.Close()
				contained.directory = nil
			}
			return leaked, errors.Join(stateErr, killErr, errors.New("cgroup survived containment cleanup"))
		}
		time.Sleep(10 * time.Millisecond)
		next, err := contained.populated()
		if err != nil {
			stateErr = errors.Join(stateErr, err)
			continue
		}
		populated = next
	}
	return leaked, contained.remove()
}

func (contained *containment) readReport() error {
	if contained.report == nil {
		return nil
	}
	defer func() {
		_ = contained.report.Close()
		contained.report = nil
	}()
	decoder := json.NewDecoder(io.LimitReader(contained.report, helperReportLimit+1))
	decoder.DisallowUnknownFields()
	var report helperReport
	if err := decoder.Decode(&report); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("execution helper report has trailing data")
	}
	if report.ExitCode < -1 || report.ExitCode > 255 || report.Signal < 0 || report.Signal > 64 || report.Signal != 0 && report.ExitCode != -1 {
		return errors.New("execution helper report is invalid")
	}
	contained.outcome = &report
	return nil
}

func (contained *containment) applyOutcome(envelope *ArtifactEnvelope) {
	if contained.outcome == nil {
		return
	}
	envelope.ExitCode = contained.outcome.ExitCode
	if contained.outcome.Signal != 0 {
		envelope.Signal = syscall.Signal(contained.outcome.Signal).String()
	}
}

func (contained *containment) populated() (bool, error) {
	content, err := os.ReadFile(filepath.Join(contained.path, "cgroup.events"))
	if err != nil {
		return false, err
	}
	fields := strings.Fields(string(content))
	for index := 0; index+1 < len(fields); index += 2 {
		if fields[index] == "populated" {
			return fields[index+1] == "1", nil
		}
	}
	return false, errors.New("cgroup populated state is unavailable")
}

func (contained *containment) remove() error {
	if contained.directory != nil {
		if err := contained.directory.Close(); err != nil {
			return err
		}
		contained.directory = nil
	}
	if contained.path == "" {
		return nil
	}
	err := os.Remove(contained.path)
	contained.path = ""
	return err
}

func (contained *containment) abort() {
	if contained == nil {
		return
	}
	if contained.directory != nil {
		_ = contained.directory.Close()
		contained.directory = nil
	}
	if contained.report != nil {
		_ = contained.report.Close()
		contained.report = nil
	}
	if contained.path != "" {
		_ = os.Remove(contained.path)
		contained.path = ""
	}
}

func authenticatedHelperInvocation(nonce string) bool {
	if len(nonce) != 64 {
		return false
	}
	var configStat, toolStat unix.Stat_t
	if unix.Fstat(3, &configStat) != nil || unix.Fstat(4, &toolStat) != nil || configStat.Mode&unix.S_IFMT != unix.S_IFREG || configStat.Nlink != 0 || toolStat.Mode&unix.S_IFMT != unix.S_IFREG || toolStat.Mode&0o111 == 0 {
		return false
	}
	encoded := make([]byte, helperConfigLimit+1)
	size, err := unix.Pread(3, encoded, 0)
	if err != nil && !errors.Is(err, io.EOF) || size > helperConfigLimit {
		return false
	}
	var config helperConfig
	if json.Unmarshal(encoded[:size], &config) != nil || config.Magic != helperMagic || len(config.Nonce) != len(nonce) {
		return false
	}
	if config.Supervise {
		var reportStat unix.Stat_t
		if unix.Fstat(5, &reportStat) != nil || reportStat.Mode&unix.S_IFMT != unix.S_IFIFO {
			return false
		}
	}
	return subtle.ConstantTimeCompare([]byte(config.Nonce), []byte(nonce)) == 1
}

func runExecutionHelper(expectedNonce string) error {
	configFile := os.NewFile(3, "reconctx-exec")
	if configFile == nil {
		return errors.New("missing execution helper configuration")
	}
	decoder := json.NewDecoder(io.LimitReader(configFile, helperConfigLimit+1))
	decoder.DisallowUnknownFields()
	var config helperConfig
	if err := decoder.Decode(&config); err != nil {
		configFile.Close()
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		configFile.Close()
		return errors.New("execution helper configuration has trailing data")
	}
	if err := configFile.Close(); err != nil {
		return err
	}
	if config.Magic != helperMagic || subtle.ConstantTimeCompare([]byte(config.Nonce), []byte(expectedNonce)) != 1 || config.NativeFileLimit <= 1 || config.NativeFileLimit > maxArtifactRead+1 || config.GracePeriod <= 0 || config.GracePeriod > 5*time.Second || len(config.Tool.Argv) == 0 || config.Tool.Argv[0] != config.Tool.ResolvedPath {
		return errors.New("invalid execution helper configuration")
	}
	toolFile := os.NewFile(4, "reconctx-tool")
	if toolFile == nil {
		return errors.New("missing approved tool descriptor")
	}
	if err := verifyOpenTool(toolFile, config.Tool, false); err != nil {
		return errors.New("tool identity changed in execution helper")
	}
	limit := unix.Rlimit{Cur: uint64(config.NativeFileLimit), Max: uint64(config.NativeFileLimit)}
	if err := unix.Setrlimit(unix.RLIMIT_FSIZE, &limit); err != nil {
		return err
	}
	environment := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, helperEnvironment+"=") {
			environment = append(environment, entry)
		}
	}
	if config.Supervise {
		return superviseTool(toolFile, config.Tool.Argv, environment, config.GracePeriod)
	}
	return syscall.Exec("/proc/self/fd/4", config.Tool.Argv, environment)
}

func superviseTool(toolFile *os.File, arguments, environment []string, grace time.Duration) error {
	reportFile := os.NewFile(5, "reconctx-exec-report")
	if reportFile == nil {
		return errors.New("missing execution helper report pipe")
	}
	defer reportFile.Close()
	termination := make(chan os.Signal, 1)
	signal.Notify(termination, syscall.SIGTERM)
	defer signal.Stop(termination)
	command := exec.Command("/proc/self/fd/3")
	command.Args = arguments
	command.Env = environment
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.ExtraFiles = []*os.File{toolFile}
	if err := command.Start(); err != nil {
		return err
	}
	waitErr := command.Wait()
	leaked, err := reapDescendants(grace)
	if err != nil {
		return err
	}
	exitCode, signal, err := helperExitStatus(waitErr)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(reportFile).Encode(helperReport{Leaked: leaked, ExitCode: exitCode, Signal: signal}); err != nil {
		return err
	}
	code := exitCode
	if signal != 0 {
		code = 128 + signal
	}
	return &helperExitError{code: code}
}

func reapDescendants(grace time.Duration) (bool, error) {
	leaked, err := descendantsRemain()
	if err != nil || !leaked {
		return leaked, err
	}
	for _, signal := range []syscall.Signal{syscall.SIGTERM, syscall.SIGCONT} {
		if err := syscall.Kill(-1, signal); err != nil && !errors.Is(err, syscall.ESRCH) {
			return true, err
		}
	}
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		remaining, err := descendantsRemain()
		if err != nil || !remaining {
			return true, err
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := syscall.Kill(-1, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return true, err
	}
	for {
		var status syscall.WaitStatus
		_, err := syscall.Wait4(-1, &status, 0, nil)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if errors.Is(err, syscall.ECHILD) {
			return true, nil
		}
		if err != nil {
			return true, err
		}
	}
}

func descendantsRemain() (bool, error) {
	for {
		var status syscall.WaitStatus
		pid, err := syscall.Wait4(-1, &status, syscall.WNOHANG, nil)
		if pid > 0 {
			continue
		}
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if errors.Is(err, syscall.ECHILD) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return true, nil
	}
}

func helperExitStatus(waitErr error) (int, int, error) {
	if waitErr == nil {
		return 0, 0, nil
	}
	exit, ok := waitErr.(*exec.ExitError)
	if !ok {
		return 0, 0, waitErr
	}
	status, ok := exit.Sys().(syscall.WaitStatus)
	if ok && status.Signaled() {
		return -1, int(status.Signal()), nil
	}
	return exit.ExitCode(), 0, nil
}

func openApprovedTool(tool model.ToolPlan, checkOwners bool) (*os.File, error) {
	fd, err := unix.Open(tool.ResolvedPath, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), "reconctx-tool")
	if err := verifyOpenTool(file, tool, checkOwners); err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}

func verifyOpenTool(file *os.File, tool model.ToolPlan, checkOwners bool) error {
	var stat unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &stat); err != nil {
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 || uint32(stat.Mode&0o777) != tool.Binary.Mode || uint64(stat.Dev) != tool.Binary.Device || stat.Ino != tool.Binary.Inode || checkOwners && (stat.Uid != tool.Binary.UID || stat.Gid != tool.Binary.GID) {
		return errors.New("opened tool identity differs from approved identity")
	}
	if _, err := file.Seek(0, 0); err != nil {
		return err
	}
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return err
	}
	if "sha256:"+hex.EncodeToString(digest.Sum(nil)) != tool.Binary.SHA256 {
		return errors.New("opened tool digest differs from approved digest")
	}
	_, err := file.Seek(0, 0)
	return err
}

func createExecutionDir(name string) (*executionDir, error) {
	parentFD, base, err := openExecutionParent(name)
	if err != nil {
		return nil, err
	}
	defer unix.Close(parentFD)
	if err := unix.Mkdirat(parentFD, base, 0o700); err != nil {
		return nil, fmt.Errorf("create execution directory: %w", err)
	}
	if err := unix.Fsync(parentFD); err != nil {
		return nil, err
	}
	return openExecutionAt(parentFD, base, name)
}

func openExecutionDir(name string) (*executionDir, error) {
	parentFD, base, err := openExecutionParent(name)
	if err != nil {
		return nil, err
	}
	defer unix.Close(parentFD)
	return openExecutionAt(parentFD, base, name)
}

func openExecutionParent(name string) (int, string, error) {
	if !filepath.IsAbs(name) || filepath.Clean(name) != name {
		return -1, "", errors.New("execution directory must be an absolute clean path")
	}
	base := filepath.Base(name)
	if base == "." || base == string(filepath.Separator) || strings.ContainsRune(base, '\x00') {
		return -1, "", errors.New("invalid execution directory name")
	}
	parent := filepath.Dir(name)
	fd, err := unix.Openat2(unix.AT_FDCWD, parent, &unix.OpenHow{
		Flags:   unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC,
		Resolve: unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	})
	if err != nil {
		return -1, "", fmt.Errorf("open execution parent: %w", err)
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		unix.Close(fd)
		return -1, "", err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Mode&0o077 != 0 || stat.Uid != 0 && stat.Uid != uint32(os.Getuid()) {
		unix.Close(fd)
		return -1, "", errors.New("execution parent is not a trusted private directory")
	}
	return fd, base, nil
}

func openExecutionAt(parentFD int, base, path string) (*executionDir, error) {
	fd, err := unix.Openat(parentFD, base, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		unix.Close(fd)
		return nil, err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Mode&0o077 != 0 {
		unix.Close(fd)
		return nil, errors.New("execution directory is not private")
	}
	return &executionDir{path: path, fd: fd}, nil
}

func (directory *executionDir) close() error { return unix.Close(directory.fd) }
func (directory *executionDir) sync() error  { return unix.Fsync(directory.fd) }

func (directory *executionDir) create(name string) (*os.File, error) {
	if !validArtifactName(name) {
		return nil, errors.New("invalid artifact name")
	}
	fd, err := unix.Openat(directory.fd, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), name), nil
}

func (directory *executionDir) writeExclusive(name string, data []byte) error {
	file, err := directory.create(name)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func (directory *executionDir) rename(oldName, newName string) error {
	if !validArtifactName(oldName) || !validArtifactName(newName) {
		return errors.New("invalid artifact name")
	}
	if err := unix.Renameat2(directory.fd, oldName, directory.fd, newName, unix.RENAME_NOREPLACE); err != nil {
		return err
	}
	return unix.Fsync(directory.fd)
}

func (directory *executionDir) read(name string, limit int64) ([]byte, error) {
	if !validArtifactName(name) || limit < 0 {
		return nil, errors.New("invalid artifact read")
	}
	fd, err := unix.Openat(directory.fd, name, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Sys().(*syscall.Stat_t).Nlink != 1 {
		return nil, errors.New("artifact is not a private single-link regular file")
	}
	content, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > limit {
		return nil, errors.New("artifact exceeds read limit")
	}
	return content, nil
}

func (directory *executionDir) normalizeRegular(name string, limit int64) error {
	if !validArtifactName(name) || limit < 0 {
		return errors.New("invalid native artifact")
	}
	fd, err := unix.Openat(directory.fd, name, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 || stat.Size < 0 || stat.Size > limit || stat.Uid != 0 && stat.Uid != uint32(os.Getuid()) {
		return errors.New("native artifact is not a bounded owned single-link regular file")
	}
	if err := unix.Fchmod(fd, 0o600); err != nil {
		return err
	}
	return unix.Fsync(fd)
}

func (directory *executionDir) truncateRegular(name string, size int64) error {
	if !validArtifactName(name) || size < 0 {
		return errors.New("invalid native artifact truncation")
	}
	fd, err := unix.Openat(directory.fd, name, unix.O_WRONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 || stat.Size < size {
		return errors.New("native artifact changed before truncation")
	}
	if err := unix.Ftruncate(fd, size); err != nil {
		return err
	}
	return unix.Fsync(fd)
}

func (directory *executionDir) exists(name string) (bool, error) {
	var stat unix.Stat_t
	err := unix.Fstatat(directory.fd, name, &stat, unix.AT_SYMLINK_NOFOLLOW)
	if errors.Is(err, unix.ENOENT) {
		return false, nil
	}
	return err == nil, err
}

func validArtifactName(name string) bool {
	return name != "" && name != "." && name != ".." && filepath.Base(name) == name && !strings.ContainsAny(name, "\x00\r\n")
}

func configureProcessGroup(command *exec.Cmd, contained *containment) {
	attributes := &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	if contained.mode == containmentCgroup {
		attributes.UseCgroupFD = true
		attributes.CgroupFD = int(contained.directory.Fd())
	} else {
		attributes.Cloneflags = syscall.CLONE_NEWUSER | syscall.CLONE_NEWPID
		attributes.UidMappings = []syscall.SysProcIDMap{{ContainerID: os.Getuid(), HostID: os.Getuid(), Size: 1}}
		attributes.GidMappings = []syscall.SysProcIDMap{{ContainerID: os.Getgid(), HostID: os.Getgid(), Size: 1}}
		attributes.GidMappingsEnableSetgroups = false
	}
	command.SysProcAttr = attributes
}

func signalProcessGroup(pid int, signal syscall.Signal) error {
	err := syscall.Kill(-pid, signal)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

func processGroupExists(pid int) bool {
	return syscall.Kill(-pid, 0) == nil
}
