package sidecar

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	appruntime "github.com/jdtobe/gavel/internal/runtime"
)

func TestWaitReady(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		code       int
		good       bool
	}{
		{"ready", `{"models":[{"name":"kev-latest","backend":"mlx","dtype":"bfloat16","device":"mps"}]}`, 200, true},
		{"unavailable", `loading`, 503, false}, {"malformed", `{`, 200, false}, {"empty", `{"models":[]}`, 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/models" {
					t.Errorf("path = %s", r.URL.Path)
				}
				w.WriteHeader(tc.code)
				io.WriteString(w, tc.body)
			}))
			defer srv.Close()
			m := &Manager{url: srv.URL, HTTPClient: srv.Client()}
			got, err := m.WaitReady(context.Background(), 50*time.Millisecond)
			if tc.good {
				if err != nil {
					t.Fatal(err)
				}
				if got != (ModelInfo{"kev-latest", "mlx", "bfloat16", "mps"}) {
					t.Fatalf("info = %#v", got)
				}
			} else if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func newManager(t *testing.T, crash bool) *Manager {
	t.Helper()
	p, err := appruntime.ResolvePathsAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{Paths: p, Python: "fake-python", Log: io.Discard}
	m.CommandFactory = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		mode := "serve"
		if crash {
			mode = "crash"
		}
		return exec.Command(os.Args[0], "-test.run=^TestHelperProcess$", "--", mode, "kev.serve", args[len(args)-1])
	}
	return m
}

func TestLifecycle(t *testing.T) {
	m := newManager(t, false)
	port := freePort(t)
	if err := m.Start(context.Background(), "model", port); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Stop(context.Background()) })
	if _, err := os.Stat(m.Paths.SidecarStateFile); err != nil {
		t.Fatal(err)
	}
	if _, err := m.WaitReady(context.Background(), 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(m.URL(), "http://127.0.0.1:") {
		t.Fatal(m.URL())
	}
	if err := m.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(m.Paths.SidecarStateFile); !os.IsNotExist(err) {
		t.Fatalf("state file: %v", err)
	}
	if err := syscall.Kill(m.cmd.Process.Pid, 0); err == nil {
		t.Fatal("child remains alive")
	}
	if err := m.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestStartArgumentsAndOfflineEnvironment(t *testing.T) {
	m := newManager(t, false)
	var name string
	var args []string
	factory := m.CommandFactory
	m.CommandFactory = func(ctx context.Context, n string, a ...string) *exec.Cmd {
		name = n
		args = append([]string(nil), a...)
		return factory(ctx, n, a...)
	}
	port := freePort(t)
	if err := m.Start(context.Background(), "model", port); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Stop(context.Background()) })
	if name != "fake-python" || !reflect.DeepEqual(args[:7], []string{"-m", "kev.serve", "--run", "model", "--host", "127.0.0.1", "--port"}) {
		t.Fatalf("%s %v", name, args)
	}
	for _, entry := range []string{"HF_HUB_OFFLINE=1", "TRANSFORMERS_OFFLINE=1", "HF_HUB_DISABLE_TELEMETRY=1", "PYTHONUNBUFFERED=1", "TOKENIZERS_PARALLELISM=false", "KEV_BACKEND=auto", "HF_HOME=" + m.Paths.HFHome} {
		found := false
		for _, e := range m.cmd.Env {
			if e == entry {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing %s", entry)
		}
	}
	if m.cmd.SysProcAttr == nil || !m.cmd.SysProcAttr.Setpgid {
		t.Fatal("missing process group")
	}
}

func TestUnexpectedExit(t *testing.T) {
	m := newManager(t, false)
	if err := m.Start(context.Background(), "model", freePort(t)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Stop(context.Background()) })
	// Crash only after Start has recorded the process identity. An immediately
	// exiting helper can disappear before WriteState's ps call, testing a startup
	// failure rather than WaitReady's unexpected-exit detection.
	if err := syscall.Kill(-m.cmd.Process.Pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	_, err := m.WaitReady(context.Background(), 5*time.Second)
	if err == nil || !strings.Contains(err.Error(), "exit") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(m.Paths.SidecarStateFile); err != nil {
		t.Fatalf("process state was not recorded before the crash: %v", err)
	}
}

func TestLogs(t *testing.T) {
	m := newManager(t, false)
	for i := 0; i < 1100; i++ {
		m.appendLog("\x1b[31mred\x1b[0m")
	}
	got := m.Logs()
	if len(got) != 1000 || got[0] != "red" {
		t.Fatalf("logs length %d first %q", len(got), got[0])
	}
	got[0] = "mutated"
	if m.Logs()[0] != "red" {
		t.Fatal("Logs leaked internal slice")
	}
}

func TestAcquireLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime", "lock")
	release, err := AcquireLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if release2, err := AcquireLock(path); err == nil {
		release2()
		t.Fatal("second lock succeeded")
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	release3, err := AcquireLock(path)
	if err != nil {
		t.Fatal(err)
	}
	release3()
}

func TestReapOrphan(t *testing.T) {
	m := newManager(t, false)
	if err := m.Start(context.Background(), "model", freePort(t)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Stop(context.Background()) })
	if err := ReapOrphan(m.Paths.SidecarStateFile); err != nil {
		t.Fatal(err)
	}
	select {
	case <-m.done:
	case <-time.After(5 * time.Second):
		t.Fatal("orphan survived")
	}
	if _, err := os.Stat(m.Paths.SidecarStateFile); !os.IsNotExist(err) {
		t.Fatalf("state file: %v", err)
	}
}

func TestReapOrphanDoesNotKillUnrelatedProcess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := WriteState(path, os.Getpid(), syscall.Getpgrp()); err == nil {
		t.Fatal("recorded unrelated process")
	}
}

func TestReapOrphanWaitsForExit(t *testing.T) {
	m := newManager(t, false)
	m.CommandFactory = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.Command(os.Args[0], "-test.run=^TestHelperProcess$", "--", "delayed", "kev.serve", args[len(args)-1])
	}
	if err := m.Start(context.Background(), "model", freePort(t)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Stop(context.Background()) })
	if _, err := m.WaitReady(context.Background(), 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := ReapOrphan(m.Paths.SidecarStateFile); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(m.cmd.Process.Pid, 0); err == nil {
		t.Fatal("ReapOrphan returned before the process exited")
	}
}
