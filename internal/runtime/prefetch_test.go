package runtime

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type blockingRunner struct {
	started chan struct{}
}

func (r *blockingRunner) Run(ctx context.Context, name string, args []string, env []string, stdout, stderr io.Writer) error {
	close(r.started)
	<-ctx.Done()
	return ctx.Err()
}

func TestParsePrefetchLine(t *testing.T) {
	data, err := os.ReadFile("testdata/prefetch.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")

	cases := []struct {
		name    string
		line    string
		want    PrefetchProgress
		wantErr string
	}{
		{
			name: "phase",
			line: lines[0],
			want: PrefetchProgress{Event: "phase", Item: "adapter"},
		},
		{
			name: "progress",
			line: lines[1],
			want: PrefetchProgress{Event: "progress", Item: "base", File: "model-00001.safetensors", Done: 100, Total: 1000},
		},
		{
			name: "done",
			line: lines[2],
			want: PrefetchProgress{Event: "done", Item: "base"},
		},
		{
			name:    "error",
			line:    lines[3],
			want:    PrefetchProgress{Event: "error", Item: "base", Message: "network down"},
			wantErr: "network down",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParsePrefetchLine(tc.line)
			if got != tc.want {
				t.Fatalf("progress = %#v, want %#v", got, tc.want)
			}
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestParsePrefetchLineRejectsMalformed(t *testing.T) {
	for _, line := range []string{"not json", `{"item":"x"}`, `{"event":"progress","done":"lots"}`} {
		if _, err := ParsePrefetchLine(line); err == nil {
			t.Fatalf("expected an error for %q", line)
		}
	}
}

func TestPrefetchStreamsProgress(t *testing.T) {
	s := newTestSetup(t)
	data, err := os.ReadFile("testdata/prefetch.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	runner := &fakeRunner{stdout: strings.Join(lines[:3], "\n") + "\n"}
	s.Runner = runner

	var events []PrefetchProgress
	model := DefaultPins().Model08B
	if err := s.Prefetch(context.Background(), model, func(p PrefetchProgress) { events = append(events, p) }); err != nil {
		t.Fatal(err)
	}

	want := []PrefetchProgress{
		{Event: "phase", Item: "adapter"},
		{Event: "progress", Item: "base", File: "model-00001.safetensors", Done: 100, Total: 1000},
		{Event: "done", Item: "base"},
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}

	scriptPath := filepath.Join(s.Paths.RuntimeDir, "prefetch.py")
	written, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(written, PrefetchScript) {
		t.Fatal("prefetch.py does not match the embedded script")
	}

	if len(runner.calls) != 1 {
		t.Fatalf("calls = %#v", runner.calls)
	}
	call := runner.calls[0]
	if call.name != s.VenvPython() {
		t.Fatalf("name = %q, want %q", call.name, s.VenvPython())
	}
	if !reflect.DeepEqual(call.args, []string{scriptPath, model}) {
		t.Fatalf("args = %#v", call.args)
	}
	for _, want := range []string{
		"HF_HOME=" + s.Paths.HFHome,
		"HF_HUB_DISABLE_TELEMETRY=1",
		"PYTHONUNBUFFERED=1",
		"TRANSFORMERS_OFFLINE=0",
	} {
		if !envHas(call.env, want) {
			t.Fatalf("missing %q in %#v", want, call.env)
		}
	}
}

func TestPrefetchReturnsHelperError(t *testing.T) {
	s := newTestSetup(t)
	s.Runner = &fakeRunner{
		stdout: `{"event":"error","item":"base","message":"network down"}` + "\n",
		stderr: "traceback tail\n",
		err:    errors.New("exit status 1"),
	}
	err := s.Prefetch(context.Background(), DefaultPins().Model08B, nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "network down") || !strings.Contains(err.Error(), "traceback tail") {
		t.Fatalf("err = %v", err)
	}
}

func TestPrefetchHonorsCancel(t *testing.T) {
	s := newTestSetup(t)
	runner := &blockingRunner{started: make(chan struct{})}
	s.Runner = runner
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-runner.started
		cancel()
	}()
	err := s.Prefetch(ctx, DefaultPins().Model08B, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}
