// Package app coordinates setup, sidecar lifecycle, and console state.
package app

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/jdtobe/gavel/internal/config"
	"github.com/jdtobe/gavel/internal/console"
	appruntime "github.com/jdtobe/gavel/internal/runtime"
	"github.com/jdtobe/gavel/internal/sidecar"
	"github.com/jdtobe/gavel/internal/status"
)

type Runtime interface {
	EnsureUV(context.Context) error
	EnsurePython(context.Context) error
	EnsureKev(context.Context) error
	Prefetch(context.Context, string, func(appruntime.PrefetchProgress)) error
	CheckDisk(string, uint64) error
	CheckMemory(string) error
}
type Sidecar interface {
	Start(context.Context, string, int) error
	WaitReady(context.Context, time.Duration) (sidecar.ModelInfo, error)
	Stop(context.Context) error
	URL() string
	Logs() []string
}
type Console interface {
	Start(context.Context) (string, error)
	WaitIdle(context.Context) error
}
type Deps struct {
	Paths   appruntime.Paths
	Config  config.Config
	Runtime Runtime
	Sidecar Sidecar
	Console Console
	Listen  func() (net.Listener, error)
}
type App struct {
	deps       Deps
	mu         sync.Mutex
	op         sync.Mutex
	cfg        config.Config
	state      status.Status
	consoleURL string
	lifetime   context.Context
	cancel     context.CancelFunc
	work       context.Context
	cancelWork context.CancelFunc
}

func New(deps Deps) *App {
	if deps.Config.Model == "" {
		deps.Config = config.Default()
	}
	if deps.Listen == nil {
		deps.Listen = func() (net.Listener, error) { return net.Listen("tcp", "127.0.0.1:0") }
	}
	ctx, cancel := context.WithCancel(context.Background())
	a := &App{deps: deps, cfg: deps.Config, state: status.Status{Phase: status.PhaseSetup, Model: deps.Config.Model}, lifetime: ctx, cancel: cancel}
	a.work, a.cancelWork = context.WithCancel(context.Background())
	if a.deps.Console == nil {
		a.deps.Console = console.New(a, deps.Sidecar.URL, nil)
	}
	return a
}
func (a *App) Status() status.Status {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state.Phase == status.PhaseReady {
		if checker, ok := a.deps.Sidecar.(interface{ ExitError() (bool, error) }); ok {
			if exited, err := checker.ExitError(); exited {
				a.state.Phase = status.PhaseError
				a.state.Error = fmt.Sprintf("Kev exited: %v; re-run setup if model cache is incomplete", err)
			}
		}
	}
	return a.state
}
func (a *App) phase(p status.Phase) {
	a.mu.Lock()
	a.state.Phase = p
	a.state.Error = ""
	a.mu.Unlock()
}
func (a *App) fail(err error) error {
	a.mu.Lock()
	a.state.Phase = status.PhaseError
	a.state.Error = err.Error()
	a.mu.Unlock()
	return err
}
func (a *App) Setup(ctx context.Context, models []string, cb func(appruntime.PrefetchProgress)) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(a.work, cancel)
	defer stop()
	a.op.Lock()
	defer a.op.Unlock()
	need, err := appruntime.RequiredDisk(models)
	if err != nil {
		return a.fail(err)
	}
	a.mu.Lock()
	for _, m := range models {
		if contains(a.cfg.DownloadedModels, m) {
			size, _ := appruntime.ModelSize(m)
			need -= size
		}
	}
	a.mu.Unlock()
	if installed, ok := a.deps.Runtime.(interface{ Installed() bool }); ok && installed.Installed() {
		need -= appruntime.RuntimeDiskBytes - (1 << 30)
	}
	if err := os.MkdirAll(a.deps.Paths.RuntimeDir, 0o700); err != nil {
		return a.fail(err)
	}
	if err := a.deps.Runtime.CheckDisk(a.deps.Paths.RuntimeDir, need); err != nil {
		return a.fail(err)
	}
	for _, m := range models {
		if err := a.deps.Runtime.CheckMemory(m); err != nil {
			return a.fail(err)
		}
	}
	// Setup may also be used from Manage: stop serving before modifying runtime.
	if a.Status().Phase == status.PhaseReady {
		if err := a.drainStop(ctx); err != nil {
			return a.fail(err)
		}
	}
	a.phase(status.PhaseDownloading)
	steps := []struct {
		label string
		run   func(context.Context) error
	}{
		{"Downloading and verifying uv", a.deps.Runtime.EnsureUV},
		{"Installing Python 3.13", a.deps.Runtime.EnsurePython},
		{"Syncing pinned Kev dependencies", a.deps.Runtime.EnsureKev},
	}
	for _, step := range steps {
		if cb != nil {
			cb(appruntime.PrefetchProgress{Event: "phase", Item: "runtime", File: step.label})
		}
		if err := step.run(ctx); err != nil {
			return a.fail(err)
		}
	}
	if cb != nil {
		cb(appruntime.PrefetchProgress{Event: "done", Item: "runtime"})
	}
	for _, m := range models {
		if err := a.deps.Runtime.Prefetch(ctx, m, cb); err != nil {
			return a.fail(err)
		}
		a.mu.Lock()
		if !contains(a.cfg.DownloadedModels, m) {
			a.cfg.DownloadedModels = append(a.cfg.DownloadedModels, m)
		}
		a.mu.Unlock()
	}
	a.mu.Lock()
	model := a.cfg.Model
	if !contains(models, model) {
		model = models[0]
	}
	a.mu.Unlock()
	return a.start(ctx, model)
}
func (a *App) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(a.work, cancel)
	defer stop()
	a.op.Lock()
	defer a.op.Unlock()
	a.mu.Lock()
	m := a.cfg.Model
	a.mu.Unlock()
	return a.start(ctx, m)
}
func (a *App) start(ctx context.Context, model string) error {
	if err := ctx.Err(); err != nil {
		return a.fail(err)
	}
	a.phase(status.PhaseStarting)
	l, err := a.deps.Listen()
	if err != nil {
		return a.fail(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	if err := a.deps.Sidecar.Start(a.lifetime, model, port); err != nil {
		return a.fail(err)
	}
	info, err := a.deps.Sidecar.WaitReady(ctx, 10*time.Minute)
	if err != nil {
		a.deps.Sidecar.Stop(context.Background())
		return a.fail(err)
	}
	a.mu.Lock()
	next := a.cfg
	next.Model = model
	a.mu.Unlock()
	if err := config.Save(a.deps.Paths.ConfigFile, next); err != nil {
		a.deps.Sidecar.Stop(context.Background())
		return a.fail(err)
	}
	a.mu.Lock()
	a.cfg = next
	a.state = status.Status{Phase: status.PhaseReady, Model: model, Backend: info.Backend, Dtype: info.Dtype, SidecarURL: a.deps.Sidecar.URL()}
	a.mu.Unlock()
	return nil
}

const restartGrace = 30 * time.Second

func (a *App) drainStop(ctx context.Context) error {
	a.phase(status.PhaseStopping)
	grace, cancel := context.WithTimeout(ctx, restartGrace)
	defer cancel()
	if err := a.deps.Console.WaitIdle(grace); err != nil {
		if canceller, ok := a.deps.Console.(interface{ CancelInflight() }); ok {
			canceller.CancelInflight()
		}
	}
	return a.deps.Sidecar.Stop(ctx)
}
func (a *App) Restart(ctx context.Context) error {
	a.op.Lock()
	defer a.op.Unlock()
	if err := a.drainStop(ctx); err != nil {
		return a.fail(err)
	}
	a.mu.Lock()
	model := a.cfg.Model
	a.mu.Unlock()
	return a.start(ctx, model)
}
func (a *App) SwitchModel(ctx context.Context, model string) error {
	a.op.Lock()
	defer a.op.Unlock()
	a.mu.Lock()
	cached := contains(a.cfg.DownloadedModels, model)
	a.mu.Unlock()
	if !cached {
		return a.fail(fmt.Errorf("model not cached; re-run setup"))
	}
	if err := a.deps.Runtime.CheckMemory(model); err != nil {
		return a.fail(err)
	}
	if err := a.drainStop(ctx); err != nil {
		return a.fail(err)
	}
	return a.start(ctx, model)
}
func (a *App) Models() []status.ModelState {
	a.mu.Lock()
	defer a.mu.Unlock()
	var models []status.ModelState
	for _, m := range []string{appruntime.Model08B, appruntime.Model4B} {
		models = append(models, status.ModelState{Name: m, Cached: contains(a.cfg.DownloadedModels, m), Active: m == a.cfg.Model})
	}
	return models
}
func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func (a *App) Logs() []string { return a.deps.Sidecar.Logs() }
func (a *App) StartConsole(ctx context.Context) (string, error) {
	a.op.Lock()
	defer a.op.Unlock()
	a.mu.Lock()
	url := a.consoleURL
	a.mu.Unlock()
	if url != "" {
		return url, nil
	}
	url, err := a.deps.Console.Start(ctx)
	if err != nil {
		return "", err
	}
	a.mu.Lock()
	a.consoleURL = url
	a.mu.Unlock()
	return url, nil
}
func (a *App) ConsoleURL() string { a.mu.Lock(); defer a.mu.Unlock(); return a.consoleURL }
func (a *App) OpenConsole() error { return exec.Command("open", a.ConsoleURL()).Run() }
func (a *App) Stop(ctx context.Context) error {
	a.cancelWork()
	a.op.Lock()
	defer a.op.Unlock()
	a.phase(status.PhaseStopping)
	err := a.deps.Sidecar.Stop(ctx)
	a.cancel()
	return err
}
