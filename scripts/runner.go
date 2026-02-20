package scripts

import "os/exec"

// RunCommand executes a command in dir and returns combined stdout/stderr.
func RunCommand(dir string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
