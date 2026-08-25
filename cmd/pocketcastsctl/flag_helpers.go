package main

import (
	"errors"
	"flag"
	"io"
	"os"
	"strings"
)

type flagHelpProbeState struct {
	output    strings.Builder
	requested bool
}

var activeFlagHelpProbe *flagHelpProbeState

func parseCommandFlags(fs *flag.FlagSet, args []string) error {
	if activeFlagHelpProbe == nil {
		return fs.Parse(args)
	}
	fs.SetOutput(&activeFlagHelpProbe.output)
	err := fs.Parse(args)
	activeFlagHelpProbe.requested = errors.Is(err, flag.ErrHelp)
	// Stop the leaf command after parsing, regardless of whether help was
	// reached. The caller uses requested to distinguish help from invalid or
	// positional input without executing the command against default config.
	return flag.ErrHelp
}

func commandErrorWriter() io.Writer {
	if activeFlagHelpProbe != nil {
		return &activeFlagHelpProbe.output
	}
	return os.Stderr
}

func parseFlagsOrExit(fs *flag.FlagSet, args []string) (bool, int) {
	if err := parseCommandFlags(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return false, 0
		}
		errf("failed to parse flags: %v\n", err)
		return false, 2
	}
	return true, 0
}

func requireNoPositionalArgsOrExit(fs *flag.FlagSet, usage string) (bool, int) {
	if fs.NArg() != 0 {
		errln(usage)
		return false, 2
	}
	return true, 0
}

func requireExactPositionalArgsOrExit(fs *flag.FlagSet, n int, usage string) (bool, int) {
	if fs.NArg() != n {
		errln(usage)
		return false, 2
	}
	return true, 0
}

func requireMinPositionalArgsOrExit(fs *flag.FlagSet, min int, usage string) (bool, int) {
	if fs.NArg() < min {
		errln(usage)
		return false, 2
	}
	return true, 0
}
