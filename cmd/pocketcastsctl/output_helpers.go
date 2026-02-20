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

func printRawOrPrettyJSON(body []byte, raw bool) {
	if raw {
		outln(string(body))
		return
	}
	if len(body) == 0 {
		outln("ok")
		return
	}
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		outln(string(body))
		return
	}
	_ = printJSON(v)
}
