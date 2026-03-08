//go:build e2e
// +build e2e

package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const e2eRegistryPIDFile = ".e2e-registry.pid"

// startRegistryServer builds the registry binary, starts the server with env.configPath,
// waits until the registry responds, and returns a cleanup function that stops the process
// and removes the binary. Callers should defer the returned cleanup.
// If a PID file from a previous run exists, only that process is killed (so we only ever
// kill our own leftover server), then the file is removed before starting the new one.
func startRegistryServer(t *testing.T, env *e2eEnv) (cleanup func()) {
	t.Helper()

	pidPath := filepath.Join(env.rootDir, e2eRegistryPIDFile)
	if b, err := os.ReadFile(pidPath); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && pid > 0 {
			if err := syscall.Kill(pid, 0); err == nil {
				_ = syscall.Kill(pid, syscall.SIGKILL)
				time.Sleep(500 * time.Millisecond)
			}
		}
		os.Remove(pidPath)
	}

	binaryPath := filepath.Join(env.rootDir, "openspmregistry.e2e")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "main.go")
	buildCmd.Dir = env.rootDir
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("build server: %v", err)
	}

	cmd := exec.Command(binaryPath, "-config", env.configPath, "-v")
	cmd.Dir = env.rootDir
	var serverLog bytes.Buffer
	cmd.Stdout = &serverLog
	cmd.Stderr = &serverLog
	if err := cmd.Start(); err != nil {
		os.Remove(binaryPath)
		t.Fatalf("start server: %v", err)
	}
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(cmd.Process.Pid)), 0644); err != nil {
		cmd.Process.Kill()
		os.Remove(binaryPath)
		t.Fatalf("write pid file: %v", err)
	}

	// If the process exits within 2s, fail with server log so we see why (e.g. config error, Fatal).
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
		t.Fatalf("registry process exited before ready. Server log:\n%s", serverLog.String())
	case <-time.After(2 * time.Second):
		// Process still running, continue to waitForRegistry
	}

	waitForRegistry(t, env)

	return func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		os.Remove(pidPath)
		os.Remove(binaryPath)
		if t.Failed() {
			t.Logf("--- Server log ---\n%s", serverLog.String())
		}
	}
}
