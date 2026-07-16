package cli

import (
	"fmt"
	"io"

	"github.com/vtrpza/reconctx/internal/version"
)

const HelpText = `reconctx - operator-run recon context compiler

Usage:
  reconctx <command> [options]

Commands:
  plan    render an offline collection plan
  run     run an approved plan
  resume  resume a run
  build   compile a handoff offline
  help    show this help
`

// Run dispatches the root command and returns a process exit code.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		if _, err := io.WriteString(stdout, HelpText); err != nil {
			return 1
		}
		return 0
	}
	if args[0] == "--version" && len(args) == 1 {
		if _, err := fmt.Fprintln(stdout, version.Version); err != nil {
			return 1
		}
		return 0
	}
	switch args[0] {
	case "plan":
		return runPlan(args[1:], stdout, stderr)
	case "run":
		return runRun(args[1:], stdin, stdout, stderr)
	case "resume":
		return runResume(args[1:], stdin, stdout, stderr)
	case "build":
		return runBuild(args[1:], stdout, stderr)
	}
	if _, err := fmt.Fprintf(stderr, "reconctx: unknown command %q\n", args[0]); err != nil {
		return 1
	}
	return 2
}
