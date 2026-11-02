package app

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jdtobe/gavel/internal/config"
	"github.com/jdtobe/gavel/internal/console"
	appruntime "github.com/jdtobe/gavel/internal/runtime"
	"github.com/jdtobe/gavel/internal/sidecar"
	"github.com/jdtobe/gavel/internal/status"
	"net/http/httptest"
	"strings"
)

type fakeRuntime struct {
	calls []string
	err   error
}

func (f *fakeRuntime) EnsureUV(context.Context) error { f.calls = append(f.calls, "uv"); return f.err }
func (f *fakeRuntime) EnsurePython(context.Context) error {
	f.calls = append(f.calls, "python")
	return f.err
}
func (f *fakeRuntime) EnsureKev(context.Context) error {
	f.calls = append(f.calls, "kev")
	return f.err
}
func (f *fakeRuntime) Prefetch(ctx context.Context, m string, cb func(appruntime.PrefetchProgress)) error {
	f.calls = append(f.calls, "prefetch:"+m)
	if cb != nil {
		cb(appruntime.PrefetchProgress{Event: "done", Item: m})
	}
	return f.err
}
func (f *fakeRuntime) CheckDisk(string, uint64) error {
	f.calls = append(f.calls, "disk")
	return f.err
}
func (f *fakeRuntime) CheckMemory(string) error { f.calls = append(f.calls, "memory"); return f.err }

type fakeSidecar struct {
	err     error
	starts  []string
	stops   int
	onReady func()
}

func (s *fakeSidecar) Start(ctx context.Context, m string, p int) error {
	s.starts = append(s.starts, m)
	return s.err
}
func (s *fakeSidecar) WaitReady(context.Context, time.Duration) (sidecar.ModelInfo, error) {
	if s.onReady != nil {
		s.onReady()
	}
	return sidecar.ModelInfo{Name: "kev-latest", Backend: "mlx", Dtype: "bfloat16"}, s.err
}
func (s *fakeSidecar) Stop(context.Context) error { s.stops++; return nil }
func (s *fakeSidecar) URL() string                { return "http://127.0.0.1:1234" }
func (s *fakeSidecar) Logs() []string             { return []string{"log"} }

type fakeConsole struct {
	starts int
	wait   func(context.Context) error
}

func (c *fakeConsole) Start(context.Context) (string, error) {
	c.starts++
	return "http://127.0.0.1:4321", nil
}
func (c *fakeConsole) WaitIdle(ctx context.Context) error {
	if c.wait != nil {
		return c.wait(ctx)
	}
	return nil
}
func newTestApp(t *testing.T) *App {
	t.Helper()
	p, _ := appruntime.ResolvePathsAt(t.TempDir())
	return New(Deps{Paths: p, Config: config.Default(), Runtime: &fakeRuntime{}, Sidecar: &fakeSidecar{}, Console: &fakeConsole{}})
}

func TestSetupTransitionsToReady(t *testing.T) {
	a := newTestApp(t)
	if a.Status().Phase != status.PhaseSetup {
		t.Fatal(a.Status())
	}
	s := a.deps.Sidecar.(*fakeSidecar)
	s.onReady = func() {
		if a.Status().Phase != status.PhaseStarting {
			t.Error(a.Status())
		}
		if _, err := os.Stat(a.deps.Paths.ConfigFile); !os.IsNotExist(err) {
			t.Error("config written before ready")
		}
	}
	if err := a.Setup(context.Background(), []string{appruntime.Model08B}, func(p appruntime.PrefetchProgress) {
		if a.Status().Phase != status.PhaseDownloading {
			t.Error(a.Status())
		}
	}); err != nil {
		t.Fatal(err)
	}
	if a.Status().Phase != status.PhaseReady {
		t.Fatal(a.Status())
	}
	got, _ := config.Load(a.deps.Paths.ConfigFile)
	if got.Model != appruntime.Model08B || len(got.DownloadedModels) != 1 {
		t.Fatal(got)
	}
	want := []string{"disk", "memory", "uv", "python", "kev", "prefetch:" + appruntime.Model08B}
	if !reflect.DeepEqual(a.deps.Runtime.(*fakeRuntime).calls, want) {
		t.Fatal(a.deps.Runtime.(*fakeRuntime).calls)
	}
}
func TestStartTransitionsToReady(t *testing.T) {
	a := newTestApp(t)
	if err := a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if a.Status().Phase != status.PhaseReady {
		t.Fatal(a.Status())
	}
	if len(a.deps.Runtime.(*fakeRuntime).calls) != 0 {
		t.Fatal("ordinary launch ran setup")
	}
}
func TestSetupError(t *testing.T) {
	a := newTestApp(t)
	a.deps.Runtime.(*fakeRuntime).err = errors.New("network")
	if err := a.Setup(context.Background(), []string{appruntime.Model08B}, nil); err == nil {
		t.Fatal("missing error")
	}
	if a.Status().Phase != status.PhaseError {
		t.Fatal(a.Status())
	}
}
func TestFailedStartDoesNotWriteConfig(t *testing.T) {
	a := newTestApp(t)
	a.deps.Sidecar.(*fakeSidecar).err = errors.New("failed")
	if err := a.Start(context.Background()); err == nil {
		t.Fatal("missing error")
	}
	if _, err := os.Stat(a.deps.Paths.ConfigFile); !os.IsNotExist(err) {
		t.Fatal("config written")
	}
}
func TestSwitchModelAndFailure(t *testing.T) {
	a := newTestApp(t)
	a.cfg.DownloadedModels = []string{appruntime.Model08B, appruntime.Model4B}
	if err := a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := a.SwitchModel(context.Background(), appruntime.Model4B); err != nil {
		t.Fatal(err)
	}
	c, _ := config.Load(a.deps.Paths.ConfigFile)
	if c.Model != appruntime.Model4B {
		t.Fatal(c)
	}
	a.deps.Sidecar.(*fakeSidecar).err = errors.New("load failed")
	if err := a.SwitchModel(context.Background(), appruntime.Model08B); err == nil {
		t.Fatal("missing error")
	}
	c, _ = config.Load(a.deps.Paths.ConfigFile)
	if c.Model != appruntime.Model4B {
		t.Fatal("active config changed")
	}
}
func TestModelsLogsConsoleStop(t *testing.T) {
	a := newTestApp(t)
	a.cfg.DownloadedModels = []string{appruntime.Model08B}
	if got := a.Models(); len(got) != 2 || !got[0].Cached || !got[0].Active || got[1].Cached {
		t.Fatal(got)
	}
	if !reflect.DeepEqual(a.Logs(), []string{"log"}) {
		t.Fatal(a.Logs())
	}
	for range 2 {
		if _, err := a.StartConsole(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if a.deps.Console.(*fakeConsole).starts != 1 || a.ConsoleURL() == "" {
		t.Fatal("console duplicated")
	}
	if err := a.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if a.Status().Phase != status.PhaseStopping {
		t.Fatal(a.Status())
	}
}
func TestRestartDrainsBeforeStop(t *testing.T) {
	a := newTestApp(t)
	a.Start(context.Background())
	s := a.deps.Sidecar.(*fakeSidecar)
	c := a.deps.Console.(*fakeConsole)
	c.wait = func(ctx context.Context) error {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 30*time.Second {
			t.Error("missing restart deadline")
		}
		if s.stops != 0 {
			t.Error("stopped before drain")
		}
		proxy := console.New(a, s.URL, nil)
		rr := httptest.NewRecorder()
		proxy.Handler().ServeHTTP(rr, httptest.NewRequest("POST", "/api/systemone", strings.NewReader(`{}`)))
		if rr.Code != 503 {
			t.Errorf("got %d during restart", rr.Code)
		}
		return context.DeadlineExceeded
	}
	if err := a.Restart(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.stops != 1 || len(s.starts) != 2 || a.Status().Phase != status.PhaseReady {
		t.Fatal("restart incomplete")
	}
}
