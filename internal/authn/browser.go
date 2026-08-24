package authn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/steipete/sweetcookie"
)

const pocketCastsCookieURL = "https://pocketcasts.com/"
const browserCredentialTimeout = 20 * time.Second

type BrowserCandidate struct {
	Browser string
	Profile string
	Session Session
}

type BrowserReader interface {
	Profiles(string) ([]string, error)
	Read(context.Context, string, string) ([]string, []string, error)
}

type SweetCookieReader struct {
	get func(context.Context, sweetcookie.Options) (sweetcookie.Result, error)
}

func NewSweetCookieReader() *SweetCookieReader {
	return &SweetCookieReader{get: sweetcookie.Get}
}

func SupportedBrowser(browser string) bool {
	switch strings.ToLower(strings.TrimSpace(browser)) {
	case "chrome", "dia", "safari":
		return true
	default:
		return false
	}
}

func (r *SweetCookieReader) Profiles(browser string) ([]string, error) {
	browser = strings.ToLower(strings.TrimSpace(browser))
	if !SupportedBrowser(browser) {
		return nil, fmt.Errorf("unsupported browser %q (choose chrome, dia, or safari)", browser)
	}
	if browser == "safari" {
		return []string{""}, nil
	}
	root, err := browserProfileRoot(browser)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(root, "Local State"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{"Default"}, nil
		}
		return nil, fmt.Errorf("read %s profile catalog: %w", browser, err)
	}
	var state struct {
		Profile struct {
			InfoCache map[string]json.RawMessage `json:"info_cache"`
		} `json:"profile"`
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("parse %s profile catalog: %w", browser, err)
	}
	profiles := make([]string, 0, len(state.Profile.InfoCache))
	for profile := range state.Profile.InfoCache {
		profile = strings.TrimSpace(profile)
		if profile != "" {
			profiles = append(profiles, profile)
		}
	}
	if len(profiles) == 0 {
		profiles = append(profiles, "Default")
	}
	sort.Strings(profiles)
	return profiles, nil
}

func (r *SweetCookieReader) Read(ctx context.Context, browser, profile string) ([]string, []string, error) {
	browser = strings.ToLower(strings.TrimSpace(browser))
	source, err := sweetCookieBrowser(browser)
	if err != nil {
		return nil, nil, err
	}
	if browser == "safari" && strings.TrimSpace(profile) != "" {
		return nil, nil, errors.New("Safari does not support --profile")
	}
	opts := sweetcookie.Options{
		URL:      pocketCastsCookieURL,
		Names:    []string{"auth"},
		Browsers: []sweetcookie.Browser{source},
		Mode:     sweetcookie.ModeFirst,
		Timeout:  browserCredentialTimeout,
	}
	if profile = strings.TrimSpace(profile); profile != "" {
		opts.Profiles = map[sweetcookie.Browser]string{source: profile}
	}
	result, err := r.get(ctx, opts)
	if err != nil {
		return nil, nil, err
	}
	values := make([]string, 0, len(result.Cookies))
	for _, cookie := range result.Cookies {
		if cookie.Name == "auth" && strings.TrimSpace(cookie.Value) != "" {
			values = append(values, cookie.Value)
		}
	}
	return values, result.Warnings, nil
}

func BrowserCandidates(ctx context.Context, reader BrowserReader, browser, profile string, now time.Time) ([]BrowserCandidate, []string, error) {
	browser = strings.ToLower(strings.TrimSpace(browser))
	if !SupportedBrowser(browser) {
		return nil, nil, fmt.Errorf("unsupported browser %q (choose chrome, dia, or safari)", browser)
	}
	profiles := []string{strings.TrimSpace(profile)}
	if profiles[0] == "" {
		var err error
		profiles, err = reader.Profiles(browser)
		if err != nil {
			return nil, nil, err
		}
	}

	var candidates []BrowserCandidate
	var warnings []string
	for _, candidateProfile := range profiles {
		values, readWarnings, err := reader.Read(ctx, browser, candidateProfile)
		warnings = append(warnings, readWarnings...)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s profile %q: %v", browser, BrowserProfileName(candidateProfile), err))
			continue
		}
		for _, value := range values {
			session, err := DecodeBrowserCookie(value, now)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s profile %q: auth cookie could not be decoded", browser, BrowserProfileName(candidateProfile)))
				continue
			}
			session.Method = "browser-" + browser
			session.Scope = ScopeWebPlayer
			candidates = append(candidates, BrowserCandidate{Browser: browser, Profile: candidateProfile, Session: session})
		}
	}
	return candidates, warnings, nil
}

func ValidBrowserCandidates(ctx context.Context, api *API, candidates []BrowserCandidate) ([]BrowserCandidate, []string) {
	valid := make([]BrowserCandidate, 0, len(candidates))
	failed := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		validated, err := api.Validate(ctx, candidate.Session)
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s profile %q: session rejected", candidate.Browser, BrowserProfileName(candidate.Profile)))
			continue
		}
		candidate.Session = validated
		valid = append(valid, candidate)
	}
	return valid, failed
}

func DecodeBrowserCookie(raw string, now time.Time) (Session, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Session{}, errors.New("auth cookie is empty")
	}
	decoded, err := url.PathUnescape(raw)
	if err != nil {
		return Session{}, fmt.Errorf("URL-decode auth cookie: %w", err)
	}
	return decodeTokenResponse([]byte(decoded), now)
}

// BrowserRecoveryHint turns dependency diagnostics into stable, actionable UX
// without exposing profile paths or raw helper output.
func BrowserRecoveryHint(browser string, warnings []string) string {
	browser = strings.ToLower(strings.TrimSpace(browser))
	joined := strings.ToLower(strings.Join(warnings, "\n"))
	switch {
	case browser == "safari" && strings.Contains(joined, "operation not permitted"):
		return "grant Full Disk Access to your terminal in System Settings > Privacy & Security, then retry"
	case strings.Contains(joined, "profile") && strings.Contains(joined, "not found"):
		return fmt.Sprintf("install %s, sign into pocketcasts.com, and retry (use --profile when needed)", BrowserName(browser))
	case strings.Contains(joined, "cookie store not found"):
		return fmt.Sprintf("sign into pocketcasts.com in %s, then retry", BrowserName(browser))
	case strings.Contains(joined, "keychain"):
		return fmt.Sprintf("allow Keychain access for %s when macOS prompts, then retry", BrowserName(browser))
	default:
		return fmt.Sprintf("sign into pocketcasts.com in %s, then retry", BrowserName(browser))
	}
}

func browserProfileRoot(browser string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch browser {
	case "chrome":
		return filepath.Join(home, "Library", "Application Support", "Google", "Chrome"), nil
	case "dia":
		return filepath.Join(home, "Library", "Application Support", "Dia", "User Data"), nil
	default:
		return "", fmt.Errorf("browser %q has no Chromium profile root", browser)
	}
}

func sweetCookieBrowser(browser string) (sweetcookie.Browser, error) {
	switch browser {
	case "chrome":
		return sweetcookie.BrowserChrome, nil
	case "dia":
		return sweetcookie.BrowserDia, nil
	case "safari":
		return sweetcookie.BrowserSafari, nil
	default:
		return "", fmt.Errorf("unsupported browser %q (choose chrome, dia, or safari)", browser)
	}
}

func BrowserProfileName(profile string) string {
	if strings.TrimSpace(profile) == "" {
		return "Default"
	}
	return profile
}

func BrowserName(browser string) string {
	switch browser {
	case "chrome":
		return "Chrome"
	case "dia":
		return "Dia"
	case "safari":
		return "Safari"
	default:
		return browser
	}
}
