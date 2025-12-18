package browsercontrol

import (
	"fmt"
	"strings"
)

type browser struct {
	kind    browserKind
	appName string
}

type browserKind int

const (
	kindChromium browserKind = iota
	kindSafari
)

func parseBrowser(name string, appOverride string) (browser, error) {
	nameNorm := normalize(name)
	appOverride = normalizeAppName(appOverride)

	switch nameNorm {
	case "", "chrome", "googlechrome":
		return browser{kind: kindChromium, appName: chooseApp(appOverride, "Google Chrome")}, nil
	case "chromium":
		if appOverride == "" {
			return browser{}, fmt.Errorf("browser=chromium requires --browser-app")
		}
		return browser{kind: kindChromium, appName: appOverride}, nil
	case "brave", "bravebrowser":
		return browser{kind: kindChromium, appName: chooseApp(appOverride, "Brave Browser")}, nil
	case "edge", "microsoftedge":
		return browser{kind: kindChromium, appName: chooseApp(appOverride, "Microsoft Edge")}, nil
	case "arc":
		return browser{kind: kindChromium, appName: chooseApp(appOverride, "Arc")}, nil
	case "dia":
		return browser{kind: kindChromium, appName: chooseApp(appOverride, "Dia")}, nil
	case "safari":
		return browser{kind: kindSafari, appName: chooseApp(appOverride, "Safari")}, nil
	default:
		// Treat unknown as a custom app name for Chromium-style scripting.
		if appOverride != "" {
			return browser{kind: kindChromium, appName: appOverride}, nil
		}
		if name != "" {
			return browser{kind: kindChromium, appName: name}, nil
		}
		return browser{}, fmt.Errorf("unsupported browser: %q", name)
	}
}

func (b browser) appleScript() string {
	switch b.kind {
	case kindChromium:
		return appleScriptChromium
	case kindSafari:
		return appleScriptSafari
	default:
		return appleScriptChromium
	}
}

func (b browser) appleScriptSetURL() string {
	switch b.kind {
	case kindChromium:
		return appleScriptChromiumSetURL
	case kindSafari:
		return appleScriptSafariSetURL
	default:
		return appleScriptChromiumSetURL
	}
}

func (b browser) appleScriptListURLs() string {
	switch b.kind {
	case kindChromium:
		return appleScriptChromiumListURLs
	case kindSafari:
		return appleScriptSafariListURLs
	default:
		return appleScriptChromiumListURLs
	}
}

func normalize(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == ' ' || r == '-' || r == '_' {
			continue
		}
		if 'A' <= r && r <= 'Z' {
			r += 'a' - 'A'
		}
		out = append(out, r)
	}
	return string(out)
}

func chooseApp(override, fallback string) string {
	if override != "" {
		return override
	}
	return fallback
}

func normalizeAppName(s string) string {
	return strings.TrimSpace(s)
}

const appleScriptChromium = `
using terms from application "Google Chrome"
on run argv
  set appName to item 1 of argv
  set urlNeedle to item 2 of argv
  set js to item 3 of argv
  set matched to 0
  set lastErr to ""
  set lastURL to ""

  tell application appName
    repeat with w in windows
      repeat with t in tabs of w
        try
          set u to URL of t
          if u contains urlNeedle then
            set matched to matched + 1
            set lastURL to u
            try
              try
                set active tab index of w to (index of t)
              end try
              try
                set index of w to 1
              end try
              return execute active tab of w javascript js
            on error errMsg number errNum
              set lastErr to errMsg & " (" & errNum & ")"
            end try
          end if
        end try
      end repeat
    end repeat
  end tell

  if matched > 0 then
    error "Found " & matched & " matching tab(s) but JavaScript execution failed (lastURL=" & lastURL & "): " & lastErr
  end if

  error "No tab found in " & appName & " with URL containing: " & urlNeedle
end run
end using terms from
`

const appleScriptSafari = `
on run argv
  set appName to item 1 of argv
  set urlNeedle to item 2 of argv
  set js to item 3 of argv

  tell application appName
    repeat with w in windows
      repeat with t in tabs of w
        try
          set u to URL of t
          if u contains urlNeedle then
            return do JavaScript js in t
          end if
        end try
      end repeat
    end repeat
  end tell

  error "No tab found in " & appName & " with URL containing: " & urlNeedle
end run
`

const appleScriptChromiumSetURL = `
using terms from application "Google Chrome"
on run argv
  set appName to item 1 of argv
  set urlNeedle to item 2 of argv
  set newURL to item 3 of argv

  tell application appName
    repeat with w in windows
      repeat with t in tabs of w
        try
          set u to URL of t
          if u contains urlNeedle then
            try
              set active tab index of w to (index of t)
            end try
            set URL of t to newURL
            return "ok"
          end if
        end try
      end repeat
    end repeat
  end tell

  error "No tab found in " & appName & " with URL containing: " & urlNeedle
end run
end using terms from
`

const appleScriptSafariSetURL = `
on run argv
  set appName to item 1 of argv
  set urlNeedle to item 2 of argv
  set newURL to item 3 of argv

  tell application appName
    repeat with w in windows
      repeat with t in tabs of w
        try
          set u to URL of t
          if u contains urlNeedle then
            set URL of t to newURL
            return "ok"
          end if
        end try
      end repeat
    end repeat
  end tell

  error "No tab found in " & appName & " with URL containing: " & urlNeedle
end run
`

const appleScriptChromiumListURLs = `
using terms from application "Google Chrome"
on run argv
  set appName to item 1 of argv
  set urls to {}

  tell application appName
    repeat with w in windows
      repeat with t in tabs of w
        try
          set u to URL of t
          if u is not missing value then
            copy u to end of urls
          end if
        end try
      end repeat
    end repeat
  end tell

  if (count of urls) is 0 then
    return "[]"
  end if

  set AppleScript's text item delimiters to "\",\""
  set joined to urls as text
  set AppleScript's text item delimiters to ""
  return "[\"" & joined & "\"]"
end run
end using terms from
`

const appleScriptSafariListURLs = `
on run argv
  set appName to item 1 of argv
  set urls to {}

  tell application appName
    repeat with w in windows
      repeat with t in tabs of w
        try
          set u to URL of t
          if u is not missing value then
            copy u to end of urls
          end if
        end try
      end repeat
    end repeat
  end tell

  if (count of urls) is 0 then
    return "[]"
  end if

  set AppleScript's text item delimiters to "\",\""
  set joined to urls as text
  set AppleScript's text item delimiters to ""
  return "[\"" & joined & "\"]"
end run
`
