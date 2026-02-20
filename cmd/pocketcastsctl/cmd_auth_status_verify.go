package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"pocketcastsctl/internal/app"
	"pocketcastsctl/internal/authutil"
	"pocketcastsctl/internal/config"
)

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
