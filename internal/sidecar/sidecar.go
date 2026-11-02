// Package sidecar manages the loopback-only Kev child process.
package sidecar

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	appruntime "github.com/jdtobe/gavel/internal/runtime"
)

// Manager owns exactly one sidecar. Lifecycle calls must be serialized.
type Manager struct {
	Paths          appruntime.Paths
	Python         string
	CommandFactory func(context.Context, string, ...string) *exec.Cmd
	HTTPClient     *http.Client
	Log            io.Writer
	mu             sync.Mutex
	cmd            *exec.Cmd
	done           chan struct{}
	exitErr        error
	url            string
	logs           []string
}

func (m *Manager) URL() string { m.mu.Lock(); defer m.mu.Unlock(); return m.url }
func (m *Manager) Logs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.logs...)
}

var ansi = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07]*(?:\x07))`)

func (m *Manager) appendLog(line string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	line = ansi.ReplaceAllString(line, "")
	if len(m.logs) == 1000 {
		copy(m.logs, m.logs[1:])
		m.logs = m.logs[:999]
	}
	m.logs = append(m.logs, line)
	if m.Log != nil {
		fmt.Fprintln(m.Log, line)
	}
}

// Start starts Kev in a separate process group with offline model resolution.
func (m *Manager) Start(ctx context.Context, model string, port int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	if m.done != nil {
		select {
		case <-m.done:
		default:
			m.mu.Unlock()
			return fmt.Errorf("sidecar already running")
		}
	}
	m.mu.Unlock()
	if err := ReapOrphan(m.Paths.SidecarStateFile); err != nil {
		return err
	}
	factory := m.CommandFactory
	if factory == nil {
		factory = exec.CommandContext
	}
	cmd := factory(ctx, m.Python, "-m", "kev.serve", "--run", model, "--host", "127.0.0.1", "--port", strconv.Itoa(port))
	cmd.Env = offlineEnv(m.Paths.HFHome)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// os/exec's default cancellation kills only the parent. Kill the group.
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	// A custom factory may not use CommandContext, which rejects Cancel.
	if m.CommandFactory != nil {
		cmd.Cancel = nil
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	errs, err := cmd.StderrPipe()
	if err != nil {
		out.Close()
		return err
	}
	if err := cmd.Start(); err != nil {
		out.Close()
		errs.Close()
		return err
	}
	done := make(chan struct{})
	m.mu.Lock()
	m.cmd = cmd
	m.done = done
	m.exitErr = nil
	m.url = "http://127.0.0.1:" + strconv.Itoa(port)
	m.mu.Unlock()
	var wg sync.WaitGroup
	for _, r := range []io.Reader{out, errs} {
		wg.Add(1)
		go func(r io.Reader) {
			defer wg.Done()
			scanner := bufio.NewScanner(r)
			scanner.Buffer(make([]byte, 4096), 1<<20)
			for scanner.Scan() {
				m.appendLog(scanner.Text())
			}
		}(r)
	}
	go func() {
		wg.Wait()
		err := cmd.Wait()
		m.mu.Lock()
		m.exitErr = err
		m.mu.Unlock()
		close(done)
	}()
	if err := WriteState(m.Paths.SidecarStateFile, cmd.Process.Pid, cmd.Process.Pid); err != nil {
		m.Stop(context.Background())
		return fmt.Errorf("record sidecar: %w", err)
	}
	return nil
}

// ExitError reports whether the child has exited, and its exit status.
func (m *Manager) ExitError() (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.done == nil {
		return false, nil
	}
	select {
	case <-m.done:
		return true, m.exitErr
	default:
		return false, nil
	}
}

// Stop terminates the entire process group and reaps the child.
func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	cmd, done := m.cmd, m.done
	m.mu.Unlock()
	if cmd == nil || done == nil {
		return nil
	}
	select {
	case <-done:
		return removeState(m.Paths.SidecarStateFile)
	default:
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
		return err
	}
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
			return err
		}
		<-done
	case <-ctx.Done():
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
	}
	return removeState(m.Paths.SidecarStateFile)
}

func offlineEnv(home string) []string {
	values := map[string]string{"PYTHONUNBUFFERED": "1", "HF_HOME": home, "HF_HUB_OFFLINE": "1", "TRANSFORMERS_OFFLINE": "1", "HF_HUB_DISABLE_TELEMETRY": "1", "TOKENIZERS_PARALLELISM": "false", "KEV_BACKEND": "auto"}
	var env []string
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if _, override := values[key]; !override {
			env = append(env, entry)
		}
	}
	for key, value := range values {
		env = append(env, key+"="+value)
	}
	return env
}
