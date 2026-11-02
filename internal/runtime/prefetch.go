package runtime

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// PrefetchScript downloads model artifacts without loading a model.
//
//go:embed prefetch.py
var PrefetchScript []byte

// PrefetchProgress is one event from the download helper.
type PrefetchProgress struct {
	Event   string `json:"event"`
	Item    string `json:"item"`
	File    string `json:"file,omitempty"`
	Message string `json:"message,omitempty"`
	Done    int64  `json:"done,omitempty"`
	Total   int64  `json:"total,omitempty"`
}

// ParsePrefetchLine decodes a helper event, preserving error events as data.
func ParsePrefetchLine(line string) (PrefetchProgress, error) {
	var p PrefetchProgress
	if err := json.Unmarshal([]byte(line), &p); err != nil {
		return p, err
	}
	switch p.Event {
	case "phase", "progress", "done":
		return p, nil
	case "error":
		return p, fmt.Errorf("prefetch %s: %s", p.Item, p.Message)
	default:
		return p, fmt.Errorf("unknown prefetch event %q", p.Event)
	}
}

// Prefetch streams download events while keeping stderr bounded.
func (s *Setup) Prefetch(ctx context.Context, model string, onProgress func(PrefetchProgress)) error {
	if _, err := ModelSize(model); err != nil {
		return err
	}
	if err := os.MkdirAll(s.Paths.RuntimeDir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(s.Paths.RuntimeDir, "prefetch.py")
	previous, _ := os.ReadFile(path)
	if !bytes.Equal(previous, PrefetchScript) {
		if err := os.WriteFile(path, PrefetchScript, 0o600); err != nil {
			return err
		}
	}
	r, w := io.Pipe()
	var stderr tailWriter
	result := make(chan error, 1)
	go func() {
		err := s.Runner.Run(ctx, s.VenvPython(), []string{path, model}, envFor(
			"HF_HOME", s.Paths.HFHome, "HF_HUB_DISABLE_TELEMETRY", "1",
			"PYTHONUNBUFFERED", "1", "HF_HUB_OFFLINE", "0", "TRANSFORMERS_OFFLINE", "0",
		), w, &stderr)
		w.Close()
		result <- err
	}()
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	var helperErr error
	for scanner.Scan() {
		p, err := ParsePrefetchLine(scanner.Text())
		if p.Event == "error" {
			helperErr = err
		}
		if err != nil && p.Event != "error" {
			if s.Log != nil {
				fmt.Fprintln(s.Log, scanner.Text())
			}
			continue
		}
		if onProgress != nil {
			onProgress(p)
		}
	}
	scanErr := scanner.Err()
	r.Close()
	runErr := <-result
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if runErr != nil || helperErr != nil {
		return fmt.Errorf("prefetch %s: %v; %v; %s", model, runErr, helperErr, strings.TrimSpace(string(stderr.data)))
	}
	return scanErr
}

type tailWriter struct{ data []byte }

func (t *tailWriter) Write(p []byte) (int, error) {
	t.data = append(t.data, p...)
	if len(t.data) > 16<<10 {
		t.data = t.data[len(t.data)-(16<<10):]
	}
	return len(p), nil
}
