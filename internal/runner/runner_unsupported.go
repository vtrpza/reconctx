//go:build !linux

package runner

import (
	"context"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
)

const helperEnvironment = "RECONCTX_INTERNAL_EXEC_HELPER"

type executionDir struct{}
type containment struct{}

func platformSupported() bool { return false }

func createExecutionDir(string) (*executionDir, error) { return nil, ErrUnsupportedPlatform }
func openExecutionDir(string) (*executionDir, error)   { return nil, ErrUnsupportedPlatform }
func startLimitedCommand(context.Context, Request, []string, io.Writer, io.Writer) (*exec.Cmd, *containment, int, error) {
	return nil, nil, 0, ErrUnsupportedPlatform
}
func signalProcessGroup(int, syscall.Signal) error { return ErrUnsupportedPlatform }
func (contained *containment) kill(int) error      { return ErrUnsupportedPlatform }
func (contained *containment) finish(int, time.Duration, bool) (bool, error) {
	return false, ErrUnsupportedPlatform
}
func (contained *containment) applyOutcome(*ArtifactEnvelope)       {}
func (directory *executionDir) close() error                        { return nil }
func (directory *executionDir) sync() error                         { return ErrUnsupportedPlatform }
func (directory *executionDir) create(string) (*os.File, error)     { return nil, ErrUnsupportedPlatform }
func (directory *executionDir) writeExclusive(string, []byte) error { return ErrUnsupportedPlatform }
func (directory *executionDir) rename(string, string) error         { return ErrUnsupportedPlatform }
func (directory *executionDir) read(string, int64) ([]byte, error) {
	return nil, ErrUnsupportedPlatform
}
func (directory *executionDir) exists(string) (bool, error)          { return false, ErrUnsupportedPlatform }
func (directory *executionDir) normalizeRegular(string, int64) error { return ErrUnsupportedPlatform }
func (directory *executionDir) truncateRegular(string, int64) error  { return ErrUnsupportedPlatform }
