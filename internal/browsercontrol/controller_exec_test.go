package browsercontrol

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupFakeOsa(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "osascript")
	script := "#!/bin/sh\n" +
		"if [ -n \"$OSASCRIPT_OUT\" ]; then\n" +
		"  printf '%s' \"$OSASCRIPT_OUT\"\n" +
		"fi\n" +
		"if [ -n \"$OSASCRIPT_ERR\" ]; then\n" +
		"  printf '%s' \"$OSASCRIPT_ERR\" >&2\n" +
		"fi\n" +
		"code=${OSASCRIPT_CODE:-0}\n" +
		"exit \"$code\"\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake osascript: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func testController() *Controller {
	return &Controller{
		browser:     browser{kind: kindChromium, appName: "Google Chrome"},
		urlContains: "pocketcasts.com",
	}
}

func TestControllerStatusAndQueueList(t *testing.T) {
	setupFakeOsa(t)
	c := testController()

	t.Run("status empty becomes unknown", func(t *testing.T) {
		t.Setenv("OSASCRIPT_OUT", `{}`)
		t.Setenv("OSASCRIPT_CODE", "0")
		st, err := c.Status(context.Background())
		if err != nil {
			t.Fatalf("Status error: %v", err)
		}
		if st.State != "unknown" {
			t.Fatalf("state = %q, want unknown", st.State)
		}
	})

	t.Run("queue list json parse", func(t *testing.T) {
		t.Setenv("OSASCRIPT_OUT", `[{"title":"Ep","href":"/ep"}]`)
		t.Setenv("OSASCRIPT_CODE", "0")
		items, err := c.QueueList(context.Background())
		if err != nil {
			t.Fatalf("QueueList error: %v", err)
		}
		if len(items) != 1 || items[0].Title != "Ep" {
			t.Fatalf("unexpected items: %+v", items)
		}
	})
}

func TestControllerDoAndErrors(t *testing.T) {
	setupFakeOsa(t)
	c := testController()

	t.Setenv("OSASCRIPT_OUT", `{"clicked":true,"clickedLabel":"Play"}`)
	t.Setenv("OSASCRIPT_CODE", "0")
	res, err := c.Do(context.Background(), ActionPlay)
	if err != nil {
		t.Fatalf("Do error: %v", err)
	}
	if !res.Clicked || res.ClickedLabel != "Play" {
		t.Fatalf("unexpected result: %+v", res)
	}

	t.Setenv("OSASCRIPT_OUT", `{"clicked":false}`)
	t.Setenv("OSASCRIPT_CODE", "0")
	_, err = c.Do(context.Background(), ActionPause)
	if err == nil || !strings.Contains(err.Error(), "no matching control found") {
		t.Fatalf("error = %v, want no matching control", err)
	}

	t.Setenv("OSASCRIPT_OUT", "")
	t.Setenv("OSASCRIPT_ERR", "boom")
	t.Setenv("OSASCRIPT_CODE", "1")
	_, err = c.Do(context.Background(), ActionNext)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error = %v, want stderr message", err)
	}
}

func TestControllerSetTabURLAndTabURLs(t *testing.T) {
	setupFakeOsa(t)
	c := testController()

	if err := c.SetTabURL(context.Background(), "   "); err == nil || !strings.Contains(err.Error(), "new URL cannot be empty") {
		t.Fatalf("error = %v, want empty URL validation", err)
	}

	t.Setenv("OSASCRIPT_OUT", "ok")
	t.Setenv("OSASCRIPT_ERR", "")
	t.Setenv("OSASCRIPT_CODE", "0")
	if err := c.SetTabURL(context.Background(), "https://play.pocketcasts.com/episode/x"); err != nil {
		t.Fatalf("SetTabURL error: %v", err)
	}

	t.Setenv("OSASCRIPT_OUT", `["https://play.pocketcasts.com"]`)
	urls, err := c.TabURLs(context.Background())
	if err != nil {
		t.Fatalf("TabURLs error: %v", err)
	}
	if len(urls) != 1 || urls[0] != "https://play.pocketcasts.com" {
		t.Fatalf("unexpected urls: %#v", urls)
	}

	t.Setenv("OSASCRIPT_OUT", `not-json`)
	_, err = c.TabURLs(context.Background())
	if err == nil || !strings.Contains(err.Error(), "unexpected JS result") {
		t.Fatalf("error = %v, want parse error", err)
	}
}
