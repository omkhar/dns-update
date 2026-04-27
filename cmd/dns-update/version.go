package main

import (
	"fmt"
	"io"

	"dns-update/internal/buildinfo"
)

func printVersion(stdout io.Writer) error {
	_, err := fmt.Fprintln(stdout, buildinfo.CommandLine())
	return err
}
