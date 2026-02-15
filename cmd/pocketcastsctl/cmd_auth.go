package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"pocketcastsctl/internal/app"
	"pocketcastsctl/internal/authutil"
	"pocketcastsctl/internal/browsercontrol"
	"pocketcastsctl/internal/config"
)

func runAuth(args []string, cfg config.Config) int {
	if len(args) == 0 || isHelpArg(args[0]) {
		printAuthHelp()
		return 0
	}

	switch args[0] {
	case "login":
		return runAuthLogin(args[1:], cfg)
	case "refresh":
		return runAuthRefresh(args[1:], cfg)
	case "status":
		return runAuthStatus(args[1:], cfg)
	case "verify":
		return runAuthVerify(args[1:], cfg)
	case "sync":
		fs := flag.NewFlagSet("auth sync", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		browser := fs.String("browser", cfg.Browser, `chrome or safari`)
		browserApp := fs.String("browser-app", cfg.BrowserApp, `macOS application name (optional)`)
		urlContains := fs.String("url-contains", cfg.URLContains, `substring to match the Pocket Casts tab URL`)
		header := fs.String("header", "Authorization", "header name to store in config")
		prefix := fs.String("prefix", "Bearer ", "prefix to add to token (set empty to store raw token)")
		keyContains := fs.String("key-contains", "", "prefer tokens whose sourceKey contains this substring")
		dryRun := fs.Bool("dry-run", false, "print token candidate keys only (no token values) and exit")
		if err := fs.Parse(args[1:]); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return 0
			}
			fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
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

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var cands []browsercontrol.TokenCandidate
		err = retryTransient(ctx, 3, 150*time.Millisecond, func() error {
			var tokenErr error
			cands, tokenErr = controller.TokenCandidates(ctx)
			return tokenErr
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "auth sync failed: %v\n", err)
			if isBrowserAutomationHintError(err) {
				_ = printTabHints(ctx, controller)
				fmt.Fprintln(os.Stderr, "tip: run `pocketcastsctl auth login` (or `pocketcastsctl login`) then try again")
				fmt.Fprintln(os.Stderr, "tip: if your Pocket Casts URL is `pocketcasts.com/...`, use `--url-contains pocketcasts.com`")
				fmt.Fprintln(os.Stderr, "tip: if this browser isn't scriptable, try `--browser chrome` or `--browser safari`")
			}
			return 1
		}
		if len(cands) == 0 {
			fmt.Fprintln(os.Stderr, "no token candidates found in localStorage (try reloading play.pocketcasts.com while logged in)")
			return 1
		}

		if *dryRun {
			for _, c := range cands {
				fmt.Printf("%s (len=%d)\n", c.SourceKey, len(c.Token))
			}
			return 0
		}

		token := selectBestToken(cands, *keyContains)
		if token == "" {
			fmt.Fprintln(os.Stderr, "no suitable token candidate found (try --dry-run and --key-contains)")
			return 1
		}

		value := token
		if *prefix != "" && !strings.HasPrefix(strings.ToLower(value), strings.ToLower(*prefix)) {
			value = *prefix + value
		}

		if cfg.APIHeaders == nil {
			cfg.APIHeaders = map[string]string{}
		}
		cfg.APIHeaders[*header] = value

		if err := config.Save(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "failed to save config: %v\n", err)
			return 1
		}
		fmt.Printf("stored %q header in: %s\n", *header, config.Path())
		return 0

	case "clear":
		cfg.APIHeaders = map[string]string{}
		if err := config.Save(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "failed to save config: %v\n", err)
			return 1
		}
		fmt.Println("cleared API auth in:", config.Path())
		return 0
	case "tabs":
		return runAuthTabs(args[1:], cfg)

	default:
		fmt.Fprintf(os.Stderr, "unknown auth subcommand: %s\n", args[0])
		return 2
	}
}

func runAuthStatus(args []string, cfg config.Config) int {
	fs := flag.NewFlagSet("auth status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "output JSON")
	plain := fs.Bool("plain", false, "plain line-oriented output")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl auth status [--json] [--plain]")
		return 2
	}

	headers := cfg.APIHeaders
	if headers == nil {
		headers = map[string]string{}
	}
	count := 0
	for _, v := range headers {
		if strings.TrimSpace(v) != "" {
			count++
		}
	}
	authHeader := false
	authHeader = authutil.HasAuthorizationHeader(headers)

	status := map[string]any{
		"config_path":            redactUserPath(config.Path()),
		"api_headers_count":      count,
		"authorization_present":  authHeader,
		"authorization_verified": false,
		"token_expiry_known":     false,
		"browser":                cfg.Browser,
		"url_contains":           cfg.URLContains,
	}
	var tokenExpiryText string
	if exp, ok := authutil.TokenExpiryUnix(headers); ok {
		status["token_expiry_known"] = true
		status["token_expiry_unix"] = exp
		remaining := exp - time.Now().Unix()
		status["token_seconds_remaining"] = remaining
		switch {
		case remaining <= 0:
			tokenExpiryText = "expired"
		case remaining < 3600:
			tokenExpiryText = fmt.Sprintf("expiring soon (%dm)", remaining/60)
		default:
			tokenExpiryText = fmt.Sprintf("valid (~%dh remaining)", remaining/3600)
		}
	}

	if *jsonOut {
		if err := printJSON(status); err != nil {
			errf("failed to render auth status JSON: %v\n", err)
			return 1
		}
		return 0
	}
	if *plain {
		keys := []string{
			"config_path",
			"api_headers_count",
			"authorization_present",
			"authorization_verified",
			"token_expiry_known",
			"browser",
			"url_contains",
		}
		for _, k := range keys {
			fmt.Printf("%s\t%v\n", k, status[k])
		}
		if exp, ok := status["token_expiry_unix"]; ok {
			fmt.Printf("token_expiry_unix\t%v\n", exp)
		}
		if rem, ok := status["token_seconds_remaining"]; ok {
			fmt.Printf("token_seconds_remaining\t%v\n", rem)
		}
		return 0
	}

	overall := "WARN"
	if authHeader {
		fmt.Println("auth status:", overall)
		fmt.Println("[OK] authorization: configured")
		fmt.Println("[WARN] authorization validity: not verified (run `pocketcastsctl doctor`)")
		if tokenExpiryText != "" {
			fmt.Printf("[OK] token_expiry: %s\n", tokenExpiryText)
		} else {
			fmt.Println("[WARN] token_expiry: unknown (token is not a JWT or has no exp claim)")
		}
	} else {
		overall = "WARN"
		fmt.Println("auth status:", overall)
		fmt.Println("[WARN] authorization: missing")
		fmt.Println("      next: pocketcastsctl auth login")
		fmt.Println("      next: pocketcastsctl auth sync")
	}
	fmt.Printf("[OK] api_headers_count: %v\n", status["api_headers_count"])
	fmt.Printf("[OK] browser: %v\n", status["browser"])
	fmt.Printf("[OK] url_contains: %v\n", status["url_contains"])
	fmt.Printf("[OK] config_path: %v\n", status["config_path"])
	return 0
}

func runAuthVerify(args []string, cfg config.Config) int {
	fs := flag.NewFlagSet("auth verify", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "output JSON")
	plain := fs.Bool("plain", false, "plain line-oriented output")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl auth verify [--json] [--plain]")
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	err := app.VerifyAuth(ctx, cfg, app.VerifyOptions{Attempts: 3, BaseDelay: 200 * time.Millisecond})

	status := map[string]any{
		"verified": false,
		"status":   "fail",
	}
	switch app.KindOf(err) {
	case "":
		status["verified"] = true
		status["status"] = "ok"
	case app.KindUnauthorized:
		status["status"] = "unauthorized"
		status["error"] = strings.TrimSpace(err.Error())
	case app.KindTransient:
		status["status"] = "unverified"
		status["error"] = strings.TrimSpace(err.Error())
	default:
		if err != nil {
			status["error"] = strings.TrimSpace(err.Error())
		}
	}

	if *jsonOut {
		if err := printJSON(status); err != nil {
			errf("failed to render auth verify JSON: %v\n", err)
			return 1
		}
		if err != nil {
			return 1
		}
		return 0
	}
	if *plain {
		fmt.Printf("verified\t%v\n", status["verified"])
		fmt.Printf("status\t%v\n", status["status"])
		if e, ok := status["error"]; ok {
			fmt.Printf("error\t%v\n", e)
		}
		if err != nil {
			return 1
		}
		return 0
	}

	if err == nil {
		fmt.Println("auth verify: OK")
		fmt.Println("[OK] authorization: accepted by API")
		return 0
	}

	switch app.KindOf(err) {
	case app.KindUnauthorized:
		fmt.Println("auth verify: FAIL")
		fmt.Println("[FAIL] authorization: rejected by API (401 Unauthorized)")
		fmt.Println("next: pocketcastsctl auth refresh")
		return 1
	case app.KindTransient:
		fmt.Println("auth verify: WARN")
		fmt.Printf("[WARN] authorization: unable to verify now (%v)\n", err)
		fmt.Println("next: retry `pocketcastsctl auth verify`")
		return 1
	default:
		fmt.Println("auth verify: FAIL")
		fmt.Printf("[FAIL] authorization: %v\n", err)
		fmt.Println("next: pocketcastsctl auth refresh")
		return 1
	}
}

func runAuthRefresh(args []string, cfg config.Config) int {
	fs := flag.NewFlagSet("auth refresh", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	browser := fs.String("browser", cfg.Browser, `browser name`)
	browserApp := fs.String("browser-app", cfg.BrowserApp, `macOS application name (optional)`)
	openURL := fs.String("url", "https://pocketcasts.com/podcasts", "URL to open for login")
	urlContains := fs.String("url-contains", cfg.URLContains, `substring to match the Pocket Casts tab URL`)
	keyContains := fs.String("key-contains", "", "prefer tokens whose sourceKey contains this substring")
	candidatePasses := fs.Int("candidate-passes", 1, "number of token-candidate verification passes")
	syncOnly := fs.Bool("sync-only", false, "skip login/open flow; sync token from current browser session")
	noInput := fs.Bool("no-input", false, "disable interactive prompts (requires --sync-only)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: pocketcastsctl auth refresh [--browser <name>] [--browser-app <app>] [--url https://play.pocketcasts.com] [--url-contains needle] [--key-contains q] [--candidate-passes N] [--sync-only] [--no-input]")
		return 2
	}
	if *noInput && !*syncOnly {
		fmt.Fprintln(os.Stderr, "auth refresh: --no-input requires --sync-only")
		return 2
	}

	if *syncOnly {
		fmt.Fprintln(os.Stderr, "refresh step 1/2: sync and verify token from current browser session")
	} else {
		fmt.Fprintln(os.Stderr, "refresh step 1/2: open login page")
		loginArgs := []string{
			"--browser", *browser,
			"--browser-app", *browserApp,
			"--url", *openURL,
			"--url-contains", *urlContains,
		}
		if code := runAuthLogin(loginArgs, cfg); code != 0 {
			return code
		}
	}

	fmt.Fprintln(os.Stderr, "refresh step 2/2: sync and verify token")
	cfgNow, _ := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	updatedCfg, result, err := app.SyncAndVerifyAuth(ctx, cfgNow, app.SyncVerifyOptions{
		Browser:         *browser,
		BrowserApp:      *browserApp,
		URLContains:     *urlContains,
		KeyContains:     strings.TrimSpace(*keyContains),
		CandidatePasses: *candidatePasses,
		VerifyOptions: app.VerifyOptions{
			Attempts:  3,
			BaseDelay: 200 * time.Millisecond,
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "auth refresh failed: %v\n", err)
		for _, f := range result.Failures {
			fmt.Fprintf(os.Stderr, "  candidate %q: %s\n", f.SourceKey, f.Reason)
		}
		if app.KindOf(err) == app.KindUnauthorized {
			printAuthRecoveryHint()
		}
		return 1
	}
	if saveErr := config.Save(updatedCfg); saveErr != nil {
		fmt.Fprintf(os.Stderr, "auth refresh failed: failed to save config: %v\n", saveErr)
		return 1
	}
	fmt.Printf("stored %q header in: %s\n", "Authorization", config.Path())
	if strings.TrimSpace(result.SourceKey) != "" {
		fmt.Fprintf(os.Stderr, "selected token source: %s\n", strings.TrimSpace(result.SourceKey))
	}

	fmt.Println("auth refresh: complete")
	return 0
}

func isBrowserAutomationHintError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "no tab found"):
		return true
	case strings.Contains(s, "syntax error"):
		return true
	case strings.Contains(s, "expected end of line"):
		return true
	case strings.Contains(s, "not authorized to send apple events"):
		return true
	case strings.Contains(s, "not allowed assistive access"):
		return true
	case strings.Contains(s, "application isn’t running"):
		return true
	case strings.Contains(s, "application isn't running"):
		return true
	default:
		return false
	}
}

func runAuthLogin(args []string, cfg config.Config) int {
	fs := flag.NewFlagSet("auth login", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	browser := fs.String("browser", cfg.Browser, `browser name (chrome/safari/arc/dia/brave/edge or custom app name)`)
	browserApp := fs.String("browser-app", cfg.BrowserApp, `macOS application name (optional)`)
	openURL := fs.String("url", "https://pocketcasts.com/podcasts", "URL to open for login")
	urlContains := fs.String("url-contains", cfg.URLContains, `substring to match the Pocket Casts tab URL`)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}

	appName := *browserApp
	if strings.TrimSpace(appName) == "" {
		appName = defaultAppForBrowser(*browser)
	}

	// Persist the user's browser preference (auth sync will write the file).
	cfg.Browser = *browser
	cfg.BrowserApp = strings.TrimSpace(*browserApp)
	cfg.URLContains = *urlContains

	if err := openInBrowser(appName, *openURL); err != nil {
		fmt.Fprintf(os.Stderr, "failed to open browser: %v\n", err)
		return 1
	}

	fmt.Fprintln(os.Stderr, "Complete login in the browser, then press Enter...")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')

	// Reuse sync logic by invoking it directly (no extra prompts).
	return runAuth([]string{"sync", "--browser", cfg.Browser, "--browser-app", cfg.BrowserApp, "--url-contains", cfg.URLContains}, cfg)
}

func runAuthTabs(args []string, cfg config.Config) int {
	fs := flag.NewFlagSet("auth tabs", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	browser := fs.String("browser", cfg.Browser, `browser name`)
	browserApp := fs.String("browser-app", cfg.BrowserApp, `macOS application name (optional)`)
	jsonOut := fs.Bool("json", false, "output JSON")
	plain := fs.Bool("plain", false, "plain line-oriented output")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "failed to parse flags: %v\n", err)
		return 2
	}
	controller, err := browsercontrol.New(browsercontrol.Options{
		Browser:     *browser,
		BrowserApp:  *browserApp,
		URLContains: "pocketcasts", // not used for TabURLs
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid browser options: %v\n", err)
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var urls []string
	err = retryTransient(ctx, 3, 150*time.Millisecond, func() error {
		var tabErr error
		urls, tabErr = controller.TabURLs(ctx)
		return tabErr
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "auth tabs failed: %v\n", err)
		return 1
	}
	if len(urls) == 0 {
		if *jsonOut {
			fmt.Println("[]")
			return 0
		}
		if *plain {
			return 0
		}
		fmt.Println("(no tabs found)")
		return 0
	}
	if *jsonOut {
		if err := printJSON(urls); err != nil {
			errf("failed to render auth tabs JSON: %v\n", err)
			return 1
		}
		return 0
	}
	for _, u := range urls {
		fmt.Println(u)
	}
	return 0
}

func printTabHints(ctx context.Context, controller *browsercontrol.Controller) error {
	urls, err := controller.TabURLs(ctx)
	if err != nil {
		return err
	}
	if len(urls) == 0 {
		return nil
	}
	fmt.Fprintln(os.Stderr, "open tabs:")
	shown := 0
	for _, u := range urls {
		if strings.Contains(strings.ToLower(u), "pocketcasts") {
			fmt.Fprintln(os.Stderr, " -", u)
			shown++
			if shown >= 8 {
				break
			}
		}
	}
	if shown == 0 {
		for _, u := range urls {
			fmt.Fprintln(os.Stderr, " -", u)
			shown++
			if shown >= 8 {
				break
			}
		}
	}
	return nil
}

func openInBrowser(appName, url string) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return fmt.Errorf("url cannot be empty")
	}
	args := []string{}
	if strings.TrimSpace(appName) != "" {
		args = append(args, "-a", appName)
	}
	args = append(args, url)
	cmd := exec.Command("open", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func defaultAppForBrowser(browser string) string {
	switch strings.ToLower(strings.TrimSpace(browser)) {
	case "", "chrome", "googlechrome":
		return "Google Chrome"
	case "safari":
		return "Safari"
	case "arc":
		return "Arc"
	case "dia":
		return "Dia"
	case "brave", "bravebrowser":
		return "Brave Browser"
	case "edge", "microsoftedge":
		return "Microsoft Edge"
	default:
		// treat as a custom macOS app name
		return browser
	}
}

func selectBestToken(cands []browsercontrol.TokenCandidate, keyContains string) string {
	ranked := rankedTokenCandidates(cands, keyContains)
	if len(ranked) == 0 {
		return ""
	}
	bestToken := strings.TrimSpace(ranked[0].Token)
	bestToken = strings.TrimPrefix(bestToken, "Bearer ")
	bestToken = strings.TrimPrefix(bestToken, "bearer ")
	return strings.TrimSpace(bestToken)
}

func rankedTokenCandidates(cands []browsercontrol.TokenCandidate, keyContains string) []browsercontrol.TokenCandidate {
	out := make([]browsercontrol.TokenCandidate, 0, len(cands))
	for _, c := range cands {
		if strings.TrimSpace(c.Token) == "" {
			continue
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return tokenCandidateScore(out[i], keyContains) > tokenCandidateScore(out[j], keyContains)
	})
	return out
}

func tokenCandidateScore(c browsercontrol.TokenCandidate, keyContains string) int {
	keyContains = strings.ToLower(strings.TrimSpace(keyContains))
	score := 0
	k := strings.ToLower(c.SourceKey)
	if keyContains != "" {
		if strings.Contains(k, keyContains) {
			score += 1000
		} else {
			score -= 1000
		}
	}
	if strings.Contains(k, "access") {
		score += 30
	}
	if strings.Contains(k, "auth") {
		score += 20
	}
	if strings.Contains(k, "token") {
		score += 10
	}
	if strings.Contains(k, "session") {
		score += 5
	}
	if exp, ok := authutil.TokenExpFromToken(c.Token); ok {
		now := time.Now().Unix()
		if exp > now {
			score += 50
			score += int((exp - now) / 60)
		} else {
			score -= 200
		}
	}
	if len(strings.TrimSpace(c.Token)) >= 40 {
		score += 5
	}
	return score
}
