package main

import (
	"errors"
	"flag"
)

func parseFlagsOrExit(fs *flag.FlagSet, args []string) (bool, int) {
	if err := fs.Parse(args); err != nil {
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
