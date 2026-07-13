package cli

import (
	"fmt"
	"io"
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

// Run dispatches the root command and returns a process exit code. G1 exposes
// help only; active and workflow subcommands are added behind later gates.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		if _, err := io.WriteString(stdout, HelpText); err != nil {
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(stderr, "reconctx: unknown command %q\n", args[0]); err != nil {
		return 1
	}
	return 2
}
