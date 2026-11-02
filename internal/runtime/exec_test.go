package runtime

import (
	"context"
	"io"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

type pidWriter struct{ ready chan int }

func (w pidWriter) Write(p []byte) (int, error) {
	pid, err := strconv.Atoi(strings.TrimSpace(string(p)))
	if err == nil {
		w.ready <- pid
	}
	return len(p), nil
}
func TestExecRunnerCancelsProcessGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	pids := make(chan int, 1)
	// The shell remains the parent; sleep must also be terminated on cancel.
	go func() {
		done <- (ExecRunner{}).Run(ctx, "/bin/sh", []string{"-c", "sleep 60 & echo $!; wait"}, os.Environ(), pidWriter{pids}, io.Discard)
	}()
	var pid int
	select {
	case pid = <-pids:
	case <-time.After(5 * time.Second):
		t.Fatal("child did not start")
	}
	defer syscall.Kill(pid, syscall.SIGKILL)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runner did not cancel")
	}
	// A zombie has stopped and will be reaped by the OS; only a live child fails.
	for i := 0; i < 50; i++ {
		if err := syscall.Kill(pid, 0); err != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("installer child survived context cancellation")
}
