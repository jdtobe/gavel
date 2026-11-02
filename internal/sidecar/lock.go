package sidecar

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

func AcquireLock(path string) (func() error, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("Gavel is already running: %w", err)
	}
	var once sync.Once
	var closeErr error
	return func() error { once.Do(func() { closeErr = f.Close() }); return closeErr }, nil
}

type processState struct {
	PID      int    `json:"pid"`
	PGID     int    `json:"pgid"`
	Identity string `json:"identity"`
}

func processIdentity(pid int) (string, error) {
	output, err := exec.Command("/bin/ps", "-p", fmt.Sprint(pid), "-o", "lstart=", "-o", "command=").Output()
	return strings.TrimSpace(string(output)), err
}

// WriteState records PID, group and start-time/command identity to avoid PID reuse.
func WriteState(path string, pid, pgid int) error {
	identity, err := processIdentity(pid)
	if err != nil {
		return err
	}
	if pid <= 1 || pgid != pid || !strings.Contains(identity, "kev.serve") {
		return fmt.Errorf("refusing to record unrelated process %d", pid)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(processState{pid, pgid, identity})
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// ReapOrphan never signals a PID whose identity or process group has changed.
func ReapOrphan(path string) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var state processState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("read sidecar state: %w", err)
	}
	if state.PID <= 1 || state.PGID != state.PID || state.Identity == "" {
		return fmt.Errorf("invalid sidecar state")
	}
	identity, err := processIdentity(state.PID)
	if err != nil {
		return removeState(path)
	}
	pgid, err := syscall.Getpgid(state.PID)
	if err != nil || pgid != state.PGID || identity != state.Identity || !strings.Contains(identity, "kev.serve") {
		return removeState(path)
	}
	if err := syscall.Kill(-pgid, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
		return err
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		current, err := processIdentity(state.PID)
		if err != nil || current != state.Identity {
			return removeState(path)
		}
		time.Sleep(25 * time.Millisecond)
	}
	// Recheck identity before escalation; the PID may have been reused.
	current, err := processIdentity(state.PID)
	if err == nil && current == state.Identity {
		if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
			return err
		}
	}
	return removeState(path)
}

func removeState(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
