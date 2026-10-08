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
	args = interspersedFlagArgs(fs, args)
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

// The CLI documents flags after selectors. Move flags ahead of operands before
// using Go's parser, retaining flag values and an explicit -- boundary.
func interspersedFlagArgs(fs *flag.FlagSet, args []string) []string {
	flags := make([]string, 0, len(args))
	operands := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			operands = append(operands, args[i+1:]...)
			break
		}
		if len(arg) < 2 || arg[0] != '-' {
			operands = append(operands, arg)
			continue
		}
		flags = append(flags, arg)
		name := strings.TrimPrefix(strings.TrimPrefix(arg, "-"), "-")
		if strings.Contains(name, "=") {
			continue
		}
		f := fs.Lookup(name)
		if f == nil {
			continue // Let flag.Parse report unknown flags and handle help.
		}
		if value, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && value.IsBoolFlag() {
			continue
		}
		if i+1 == len(args) {
			return flags // Preserve Parse's missing-value error, not an operand as its value.
		}
		i++
		flags = append(flags, args[i])
	}
	return append(append(flags, "--"), operands...)
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
