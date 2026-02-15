package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func outln(a ...any) {
	fmt.Println(a...)
}

func outprintf(format string, a ...any) {
	fmt.Printf(format, a...)
}

func errln(a ...any) {
	fmt.Fprintln(os.Stderr, a...)
}

func errf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format, a...)
}

func printJSON(v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	outln(string(b))
	return nil
}
