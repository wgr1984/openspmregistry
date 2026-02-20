//go:build e2e
// +build e2e

package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// startRegistryServer builds the registry binary, starts the server with env.configPath,
// waits until the registry responds, and returns a cleanup function that stops the process
// and removes the binary. Callers should defer the returned cleanup.
// If lsof is available, it frees port 8082 before starting.
func startRegistryServer(t *testing.T, env *e2eEnv) (cleanup func()) {
	t.Helper()

	if _, err := exec.LookPath("lsof"); err == nil {
		exec.Command("sh", "-c", "lsof -ti :8082 | xargs kill -9 2>/dev/null || true").Run()
		time.Sleep(time.Second)
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
		os.Remove(binaryPath)
		if t.Failed() {
			t.Logf("--- Server log ---\n%s", serverLog.String())
		}
	}
}
