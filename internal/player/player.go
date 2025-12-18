package player

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type StartOptions struct {
	URL       string
	Title     string
	CacheDir  string
	UserAgent string
}

type Started struct {
	PID     int
	Command []string
}

func Start(ctx context.Context, opts StartOptions) (Started, error) {
	urlStr := strings.TrimSpace(opts.URL)
	if urlStr == "" {
		return Started{}, errors.New("missing audio URL")
	}

	if mpv, _ := exec.LookPath("mpv"); mpv != "" {
		cmd := exec.CommandContext(ctx, mpv, "--no-video", "--force-window=no", "--quiet", urlStr)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			return Started{}, err
		}
		return Started{PID: cmd.Process.Pid, Command: cmd.Args}, nil
	}

	// Fallback: download and use afplay (present on macOS).
	afplay, err := exec.LookPath("afplay")
	if err != nil {
		return Started{}, errors.New("no supported player found (install mpv or ensure afplay exists)")
	}

	cacheDir := strings.TrimSpace(opts.CacheDir)
	if cacheDir == "" {
		cacheDir = os.TempDir()
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return Started{}, err
	}

	filePath, err := downloadToFile(ctx, urlStr, cacheDir, opts.UserAgent)
	if err != nil {
		return Started{}, err
	}

	cmd := exec.CommandContext(ctx, afplay, filePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return Started{}, err
	}
	return Started{PID: cmd.Process.Pid, Command: cmd.Args}, nil
}

func Pause(pid int) error  { return signal(pid, syscall.SIGSTOP) }
func Resume(pid int) error { return signal(pid, syscall.SIGCONT) }
func Stop(pid int) error   { return signal(pid, syscall.SIGTERM) }

func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 checks existence.
	if err := p.Signal(syscall.Signal(0)); err != nil {
		return false
	}
	return true
}

func signal(pid int, sig syscall.Signal) error {
	if pid <= 0 {
		return errors.New("no active playback")
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Signal(sig)
}

func downloadToFile(ctx context.Context, urlStr, dir, userAgent string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(userAgent) != "" {
		req.Header.Set("User-Agent", userAgent)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("download failed: http %d: %s", resp.StatusCode, string(b))
	}

	ext := ".mp3"
	if ct := resp.Header.Get("Content-Type"); strings.Contains(strings.ToLower(ct), "m4a") {
		ext = ".m4a"
	}
	name := fmt.Sprintf("pocketcastsctl-%d%s", time.Now().UnixNano(), ext)
	path := filepath.Join(dir, name)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}
