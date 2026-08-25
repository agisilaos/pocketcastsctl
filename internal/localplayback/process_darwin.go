//go:build darwin

package localplayback

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

	"golang.org/x/sys/unix"
)

const (
	// Values from Darwin's sys/proc.h p_stat field.
	darwinProcessStopped = 4
	darwinProcessZombie  = 5
)

type darwinProcessBackend struct{}

func (darwinProcessBackend) Prepare(ctx context.Context, request StartRequest, runtime runtimeOptions) (preparedPlayback, error) {
	urlString := strings.TrimSpace(request.URL)
	if urlString == "" {
		return preparedPlayback{}, errors.New("missing audio URL")
	}

	if executable, _ := exec.LookPath("mpv"); executable != "" {
		args := []string{"--no-video", "--force-window=no", "--quiet"}
		if request.StartAt > 0 {
			args = append(args, fmt.Sprintf("--start=%d", request.StartAt))
		}
		args = append(args, urlString)
		return preparedPlayback{
			executable:         executable,
			args:               args,
			player:             "mpv",
			startOffsetApplied: request.StartAt > 0,
		}, nil
	}

	executable, err := exec.LookPath("afplay")
	if err != nil {
		return preparedPlayback{}, errors.New("no supported player found (install mpv or ensure afplay exists)")
	}
	cacheFile, err := downloadAudio(ctx, urlString, runtime.cacheDir, runtime.userAgent)
	if err != nil {
		return preparedPlayback{}, err
	}
	return preparedPlayback{
		executable: executable,
		args:       []string{cacheFile},
		player:     "afplay",
		cacheFile:  cacheFile,
	}, nil
}

func (backend darwinProcessBackend) Launch(prepared preparedPlayback) (launchedPlayback, error) {
	if strings.TrimSpace(prepared.executable) == "" {
		return launchedPlayback{}, errors.New("missing player executable")
	}
	cmd := exec.Command(prepared.executable, prepared.args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return launchedPlayback{}, err
	}

	identity, err := inspectDarwinPID(cmd.Process.Pid)
	if err != nil {
		captureErr := fmt.Errorf("capture player identity: %w", err)
		if killErr := cmd.Process.Kill(); killErr != nil {
			return launchedPlayback{}, errors.Join(captureErr, fmt.Errorf("rollback unidentified player: %w", killErr))
		}
		_ = cmd.Wait() // Reap the child; a killed process is expected to return an exit error.
		return launchedPlayback{}, captureErr
	}
	go func() {
		_ = cmd.Wait()
	}()
	return launchedPlayback{identity: identity}, nil
}

func (darwinProcessBackend) Inspect(identity processIdentity) (processObservation, error) {
	current, status, err := inspectDarwinProcess(identity.PID)
	if err != nil {
		if errors.Is(err, unix.ESRCH) || darwinProcessAbsent(identity.PID) {
			return processObservation{}, nil
		}
		return processObservation{}, err
	}
	if status == darwinProcessZombie {
		return processObservation{}, nil
	}
	matches := current == identity
	return processObservation{
		Exists:  true,
		Matches: matches,
		Paused:  matches && status == darwinProcessStopped,
	}, nil
}

func darwinProcessAbsent(pid int) bool {
	err := unix.Kill(pid, 0)
	return errors.Is(err, unix.ESRCH)
}

func (backend darwinProcessBackend) Signal(identity processIdentity, signal processSignal) error {
	observation, err := backend.Inspect(identity)
	if err != nil {
		return err
	}
	if !observation.Exists {
		return errProcessGone
	}
	if !observation.Matches {
		return errIdentityMismatch
	}

	var unixSignal syscall.Signal
	switch signal {
	case signalPause:
		unixSignal = syscall.SIGSTOP
	case signalResume:
		unixSignal = syscall.SIGCONT
	case signalTerminate:
		unixSignal = syscall.SIGTERM
	case signalKill:
		unixSignal = syscall.SIGKILL
	default:
		return errors.New("unknown local playback signal")
	}
	if err := unix.Kill(identity.PID, unixSignal); err != nil {
		if errors.Is(err, unix.ESRCH) {
			return errProcessGone
		}
		return err
	}
	return nil
}

func inspectDarwinPID(pid int) (processIdentity, error) {
	identity, _, err := inspectDarwinProcess(pid)
	return identity, err
}

func inspectDarwinProcess(pid int) (processIdentity, int8, error) {
	if pid <= 0 {
		return processIdentity{}, 0, unix.ESRCH
	}
	info, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return processIdentity{}, 0, err
	}
	if info == nil || int(info.Proc.P_pid) != pid {
		return processIdentity{}, 0, unix.ESRCH
	}
	birth := int64(info.Proc.P_starttime.Sec)*1_000_000 + int64(info.Proc.P_starttime.Usec)
	if birth <= 0 {
		return processIdentity{}, 0, errors.New("kernel returned an invalid process birth time")
	}
	return processIdentity{PID: pid, BirthUnixMicros: birth}, info.Proc.P_stat, nil
}

func downloadAudio(ctx context.Context, urlString, cacheDir, userAgent string) (string, error) {
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return "", err
	}
	cacheInfo, err := os.Lstat(cacheDir)
	if err != nil {
		return "", err
	}
	if !cacheInfo.IsDir() || cacheInfo.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("local playback cache directory is not a real directory")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlString, nil)
	if err != nil {
		return "", err
	}
	if userAgent = strings.TrimSpace(userAgent); userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	client := &http.Client{Timeout: 60 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return "", fmt.Errorf("download failed: http %d: %s", response.StatusCode, string(body))
	}

	extension := ".mp3"
	if strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "m4a") {
		extension = ".m4a"
	}
	file, err := os.CreateTemp(cacheDir, "pocketcastsctl-*"+extension)
	if err != nil {
		return "", err
	}
	path := file.Name()
	keep := false
	defer func() {
		if !keep {
			_ = file.Close()
			_ = os.Remove(path)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return "", err
	}
	if _, err := io.Copy(file, response.Body); err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	keep = true
	return path, nil
}

func defaultProcessBackend() processBackend {
	return darwinProcessBackend{}
}

func defaultLifecycleLock(path string) lifecycleLock {
	return &darwinFileLock{path: path, pollInterval: 10 * time.Millisecond}
}

func defaultCacheDir() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pocketcastsctl"), nil
}
