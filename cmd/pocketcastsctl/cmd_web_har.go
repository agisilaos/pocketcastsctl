package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"pocketcastsctl/internal/browsercontrol"
	"pocketcastsctl/internal/config"
	"pocketcastsctl/internal/har"
)

func runWeb(args []string, cfg config.Config) int {
	if len(args) == 0 || isHelpArg(args[0]) {
		printWebHelp()
		return 0
	}
	if args[0] == "login" {
		return runWebLogin(args[1:], cfg)
	}
	if args[0] == "tabs" {
		return runWebTabs(args[1:], cfg)
	}

	fs := flag.NewFlagSet("web", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "output JSON (status only)")
	plain := fs.Bool("plain", false, "plain output (status only)")
	details := fs.Bool("details", false, "show rich playback details (status only)")
	browser := fs.String("browser", cfg.Browser, `browser name`)
	browserApp := fs.String("browser-app", cfg.BrowserApp, `macOS application name (optional)`)
	urlContains := fs.String("url-contains", cfg.URLContains, `substring to match the Pocket Casts tab URL`)
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}
	subcommand := args[0]
	switch subcommand {
	case "play", "pause", "toggle", "next", "prev", "status":
	default:
		fmt.Fprintf(os.Stderr, "unknown web subcommand: %s\n", subcommand)
		return 2
	}
	if subcommand != "status" && *details {
		fmt.Fprintf(os.Stderr, "web %s: --details is only supported by web status\n", subcommand)
		return 2
	}

	controller, err := browsercontrol.New(browsercontrol.Options{
		Browser:     *browser,
		BrowserApp:  *browserApp,
		URLContains: *urlContains,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid browser options: %v\n", err)
		return 2
	}
	target := newBrowserTarget(*browser, *browserApp, *urlContains)
	if err := target.applicationError(); err != nil {
		target.printFailure(subcommand, err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if subcommand != "status" {
		return runWebAction(ctx, controller, browsercontrol.Action(subcommand), target)
	}

	var st browsercontrol.PlaybackSnapshot
	err = retryTransient(ctx, 3, 150*time.Millisecond, func() error {
		var statusErr error
		st, statusErr = controller.Status(ctx)
		return statusErr
	})
	if err != nil {
		target.printFailure("status", err)
		return 1
	}
	if *jsonOut {
		_ = printJSON(st)
		return 0
	}
	if *plain {
		if *details {
			printWebStatusDetailsPlain(st)
			return 0
		}
		fmt.Println(st.State)
		return 0
	}
	if *details {
		printWebStatusDetails(st)
		return 0
	}
	fmt.Println(st.State)
	return 0
}

func runWebAction(ctx context.Context, controller *browsercontrol.Controller, action browsercontrol.Action, target browserTarget) int {
	res, err := controller.Do(ctx, action)
	if err != nil {
		target.printFailure(string(action), err)
		return 1
	}
	if res.ClickedLabel != "" {
		fmt.Println(res.ClickedLabel)
		return 0
	}
	fmt.Println("ok")
	return 0
}

func runHAR(args []string) int {
	if len(args) == 0 || isHelpArg(args[0]) {
		printHARHelp()
		return 0
	}

	switch args[0] {
	case "summarize":
		return runHARSummarize(args[1:])
	case "graphql":
		return runHARGraphQL(args[1:])
	case "redact":
		return runHARRedact(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown har subcommand: %s\n", args[0])
		return 2
	}
}

func runHARSummarize(args []string) int {
	fs := flag.NewFlagSet("har summarize", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	host := fs.String("host", "api.pocketcasts.com", "filter requests by host (empty = no filter)")
	jsonOut := fs.Bool("json", false, "output JSON")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl har summarize [--host host] [--json] <file.har>")
		return 2
	}

	f := fs.Arg(0)
	sum, err := har.SummarizeFile(f, har.SummarizeOptions{Host: strings.TrimSpace(*host)})
	if err != nil {
		fmt.Fprintf(os.Stderr, "summarize failed: %v\n", err)
		return 1
	}

	if *jsonOut {
		b, _ := json.MarshalIndent(sum, "", "  ")
		fmt.Println(string(b))
		return 0
	}
	fmt.Print(har.FormatSummaryText(sum))
	return 0
}

func runHARRedact(args []string) int {
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl har redact <in.har> <out.har>")
		return 2
	}
	if err := har.RedactFile(args[0], args[1], har.DefaultRedactOptions()); err != nil {
		fmt.Fprintf(os.Stderr, "redact failed: %v\n", err)
		return 1
	}
	fmt.Println("wrote:", args[1])
	return 0
}

func runHARGraphQL(args []string) int {
	fs := flag.NewFlagSet("har graphql", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	host := fs.String("host", "api.pocketcasts.com", "filter requests by host (empty = no filter)")
	jsonOut := fs.Bool("json", false, "output JSON")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl har graphql [--host host] [--json] <file.har>")
		return 2
	}

	f := fs.Arg(0)
	sum, err := har.GraphQLOpsFile(f, har.GraphQLOpsOptions{Host: strings.TrimSpace(*host)})
	if err != nil {
		fmt.Fprintf(os.Stderr, "graphql failed: %v\n", err)
		return 1
	}
	if *jsonOut {
		b, _ := json.MarshalIndent(sum, "", "  ")
		fmt.Println(string(b))
		return 0
	}
	fmt.Print(har.FormatGraphQLOpsText(sum))
	return 0
}
