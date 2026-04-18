package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"dns-update/internal/agentdocs"
)

var exit = os.Exit

func main() {
	exit(run(os.Args[1:], os.Stderr))
}

func run(args []string, stderr io.Writer) int {
	flags := flag.NewFlagSet("agentdocgen", flag.ContinueOnError)
	flags.SetOutput(stderr)

	root := flags.String("root", ".", "repository root")
	check := flags.Bool("check", false, "compare generated files without writing them")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	if *check {
		mismatches, err := agentdocs.Check(*root)
		if err != nil && err != agentdocs.ErrOutOfDate {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if len(mismatches) > 0 {
			fmt.Fprintln(stderr, agentdocs.Summary(mismatches))
			return 1
		}
		return 0
	}

	if err := agentdocs.Write(*root); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
