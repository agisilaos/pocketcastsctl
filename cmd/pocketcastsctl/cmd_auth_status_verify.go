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
	"pocketcastsctl/internal/authn"
	"pocketcastsctl/internal/config"
)

func runAuthStatus(args []string, cfg config.Config) int {
	fs := flag.NewFlagSet("auth status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var outputFlags authOutputFlags
	outputFlags.register(fs)
	if ok, code := parseFlagsOrExit(fs, args); !ok {
		return code
	}
	mode, ok := outputFlags.resolveOrReport("auth status")
	if !ok {
		return 2
	}
	if ok, code := requireNoPositionalArgsOrExit(fs, "usage: pocketcastsctl auth status [--json|--plain]"); !ok {
		return code
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
	manager := newAuthManager(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	session, source, loadErr := manager.Snapshot(ctx)
	cancel()
	authHeader := loadErr == nil && strings.TrimSpace(session.AccessToken) != ""

	status := map[string]any{
		"config_path":            redactUserPath(config.Path()),
		"api_headers_count":      count,
		"authorization_present":  authHeader,
		"authorization_verified": false,
		"token_expiry_known":     false,
		"browser":                cfg.Browser,
		"url_contains":           cfg.URLContains,
		"source":                 string(source),
	}
	if session.Method != "" {
		status["method"] = session.Method
	}
	if session.Scope != "" {
		status["scope"] = session.Scope
	}
	if session.Email != "" {
		status["email"] = session.Email
	}
	if session.AccountID != "" {
		status["account_id"] = session.AccountID
	}
	missingOnly := errors.Is(loadErr, authn.ErrNotConfigured)
	if loadErr != nil && !missingOnly {
		status["error"] = loadErr.Error()
	}
	var tokenExpiryText string
	if exp := session.ExpiresAt; exp > 0 {
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

	switch mode {
	case authOutputJSON:
		if err := printJSON(status); err != nil {
			errf("failed to render auth status JSON: %v\n", err)
			return 1
		}
		return 0
	case authOutputPlain:
		keys := []string{
			"config_path",
			"api_headers_count",
			"authorization_present",
			"authorization_verified",
			"token_expiry_known",
			"source",
			"browser",
			"url_contains",
		}
		for _, k := range keys {
			fmt.Printf("%s\t%v\n", k, status[k])
		}
		for _, key := range []string{"account_id", "email", "method", "scope", "error"} {
			if value, ok := status[key]; ok {
				fmt.Printf("%s\t%v\n", key, value)
			}
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
		fmt.Printf("[OK] credential_source: %s\n", source)
		if session.Method != "" {
			fmt.Printf("[OK] method: %s\n", session.Method)
		}
		if session.Scope != "" {
			fmt.Printf("[OK] scope: %s\n", session.Scope)
		}
		if session.Email != "" {
			fmt.Printf("[OK] account: %s\n", session.Email)
		} else if session.AccountID != "" {
			fmt.Printf("[OK] account: %s\n", session.AccountID)
		}
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
		fmt.Println("      next: pocketcastsctl auth import-browser --browser dia")
		if loadErr != nil && !missingOnly {
			fmt.Printf("[WARN] credential_source: %v\n", loadErr)
		}
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
	var outputFlags authOutputFlags
	outputFlags.register(fs)
	if ok, code := parseFlagsOrExit(fs, args); !ok {
		return code
	}
	mode, ok := outputFlags.resolveOrReport("auth verify")
	if !ok {
		return 2
	}
	if ok, code := requireNoPositionalArgsOrExit(fs, "usage: pocketcastsctl auth verify [--json|--plain]"); !ok {
		return code
	}
	warnLegacyCredential(cfg)

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

	switch mode {
	case authOutputJSON:
		if err := printJSON(status); err != nil {
			errf("failed to render auth verify JSON: %v\n", err)
			return 1
		}
		if err != nil {
			return 1
		}
		return 0
	case authOutputPlain:
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
		fmt.Println("next: pocketcastsctl auth login")
		fmt.Println("next: pocketcastsctl auth import-browser --browser dia")
		return 1
	case app.KindTransient:
		fmt.Println("auth verify: WARN")
		fmt.Printf("[WARN] authorization: unable to verify now (%v)\n", err)
		fmt.Println("next: retry `pocketcastsctl auth verify`")
		return 1
	default:
		fmt.Println("auth verify: FAIL")
		fmt.Printf("[FAIL] authorization: %v\n", err)
		fmt.Println("next: pocketcastsctl auth login")
		return 1
	}
}
