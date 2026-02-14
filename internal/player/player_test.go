package player

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPauseResumeStopRequireActivePID(t *testing.T) {
	tests := []struct {
		name string
		fn   func(int) error
	}{
		{name: "pause", fn: Pause},
		{name: "resume", fn: Resume},
		{name: "stop", fn: Stop},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn(0)
			if err == nil || !strings.Contains(err.Error(), "no active playback") {
				t.Fatalf("error = %v, want no active playback", err)
			}
		})
	}
}

func TestAliveWithInvalidPID(t *testing.T) {
	if Alive(0) {
		t.Fatalf("Alive(0) = true, want false")
	}
	if Alive(-1) {
		t.Fatalf("Alive(-1) = true, want false")
	}
}

func TestDownloadToFileSuccessMP3(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != "pcctl-test" {
			t.Fatalf("User-Agent = %q, want pcctl-test", got)
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("abc123"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	path, err := downloadToFile(context.Background(), srv.URL, dir, "pcctl-test")
	if err != nil {
		t.Fatalf("downloadToFile error = %v", err)
	}
	if filepath.Ext(path) != ".mp3" {
		t.Fatalf("ext = %s, want .mp3", filepath.Ext(path))
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(b) != "abc123" {
		t.Fatalf("file content = %q, want abc123", string(b))
	}
}

func TestDownloadToFileChoosesM4AExtension(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/x-m4a")
		_, _ = w.Write([]byte("m4a-bytes"))
	}))
	defer srv.Close()

	path, err := downloadToFile(context.Background(), srv.URL, t.TempDir(), "")
	if err != nil {
		t.Fatalf("downloadToFile error = %v", err)
	}
	if filepath.Ext(path) != ".m4a" {
		t.Fatalf("ext = %s, want .m4a", filepath.Ext(path))
	}
}

func TestDownloadToFileHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := downloadToFile(context.Background(), srv.URL, t.TempDir(), "")
	if err == nil {
		t.Fatalf("expected HTTP error")
	}
	if !strings.Contains(err.Error(), "http 401") {
		t.Fatalf("error = %v, want http 401", err)
	}
}
