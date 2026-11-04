package tui

import (
	"context"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	appruntime "github.com/jdtobe/gavel/internal/runtime"
	"github.com/jdtobe/gavel/internal/status"
)

type fakeApp struct {
	state     status.Status
	setup     []string
	cancelled bool
}

func (a *fakeApp) Status() status.Status { return a.state }
func (a *fakeApp) Setup(ctx context.Context, models []string, cb func(appruntime.PrefetchProgress)) error {
	a.setup = models
	return nil
}
func (a *fakeApp) Start(context.Context) error               { return nil }
func (a *fakeApp) Restart(context.Context) error             { return nil }
func (a *fakeApp) SwitchModel(context.Context, string) error { return nil }
func (a *fakeApp) Models() []status.ModelState {
	return []status.ModelState{{Name: appruntime.Model08B}, {Name: appruntime.Model4B}}
}
func (a *fakeApp) Logs() []string                               { return []string{"log"} }
func (a *fakeApp) StartConsole(context.Context) (string, error) { return a.ConsoleURL(), nil }
func (a *fakeApp) Stop(context.Context) error                   { return nil }
func (a *fakeApp) ConsoleURL() string                           { return "http://127.0.0.1:1234" }
func (a *fakeApp) OpenConsole() error                           { return nil }
func testModel(memory, free uint64) Model {
	return New(Deps{App: &fakeApp{}, MemoryBytes: func() (uint64, error) { return memory, nil }, DiskFree: func(string) (uint64, error) { return free, nil }})
}
func key(m Model, k string) (Model, tea.Cmd) {
	var msg tea.KeyMsg
	switch k {
	case "space":
		msg = tea.KeyMsg{Type: tea.KeySpace}
	case "down":
		msg = tea.KeyMsg{Type: tea.KeyDown}
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
	}
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}
func TestSpaceTogglesModel(t *testing.T) {
	m := testModel(64<<30, 1<<40)
	if !m.selected[appruntime.Model08B] || m.selected[appruntime.Model4B] {
		t.Fatal(m.selected)
	}
	m, _ = key(m, "space")
	if m.selected[appruntime.Model08B] {
		t.Fatal("not deselected")
	}
	m, _ = key(m, "down")
	m, _ = key(m, "space")
	if !m.selected[appruntime.Model4B] {
		t.Fatal("4B not selected")
	}
}
func TestSetupGuards(t *testing.T) {
	m := testModel(16<<30, 1<<40)
	m, _ = key(m, "down")
	m, _ = key(m, "space")
	if m.selected[appruntime.Model4B] {
		t.Fatal("selected unsupported model")
	}
	m = testModel(64<<30, 1)
	_, cmd := key(m, "enter")
	if cmd != nil {
		t.Fatal("download allowed without disk")
	}
	if !strings.Contains(m.View(), "Disk free") {
		t.Fatal(m.View())
	}
}

func TestCachedRecoveryNeedsOnlyHeadroom(t *testing.T) {
	m := testModel(64<<30, 1<<30)
	m.cached[appruntime.Model08B] = true
	m.deps.RuntimeInstalled = true
	if !m.canDownload() {
		t.Fatal("cached recovery blocked by full download accounting")
	}
}
func TestCachedSetupOffersStart(t *testing.T) {
	m := testModel(64<<30, 1<<40)
	m.cached[appruntime.Model08B] = true
	m.deps.RuntimeInstalled = true
	view := m.View()
	if !strings.Contains(view, "[enter] Start") || strings.Contains(view, "[enter] Download & start") {
		t.Fatalf("cached model still asks to download: %s", view)
	}
	if !strings.Contains(view, "(cached)") || !strings.Contains(view, "Downloaded; ready to start.") {
		t.Fatalf("cached state not visible: %s", view)
	}
}
func TestAutoStartSkipsSetupScreen(t *testing.T) {
	m := New(Deps{App: &fakeApp{}, AutoStart: true})
	if m.screen != "progress" {
		t.Fatalf("auto start showed %q instead of progress", m.screen)
	}
	m.cached[appruntime.Model08B] = true
	view := m.View()
	if !strings.Contains(view, "Starting Kev") || strings.Contains(view, "First run") {
		t.Fatalf("auto start still looks like setup: %s", view)
	}
}
func TestCachedProgressIsNotADownload(t *testing.T) {
	m := testModel(64<<30, 1<<40)
	m.cached[appruntime.Model08B] = true
	m.screen = "progress"
	view := m.View()
	if !strings.Contains(view, "Starting Kev") || strings.Contains(view, "Downloading") {
		t.Fatalf("cached start looks like a download: %s", view)
	}
	if !strings.Contains(view, "Model cached") {
		t.Fatalf("missing cached note: %s", view)
	}
}
func TestEnterStartsSelectedModels(t *testing.T) {
	m := testModel(64<<30, 1<<40)
	next, cmd := key(m, "enter")
	if cmd == nil || next.screen != "progress" {
		t.Fatal("no setup command")
	}
	// The returned batch contains a setup worker and progress consumer.
	batch := cmd().(tea.BatchMsg)
	for _, c := range batch {
		msg := c()
		if _, ok := msg.(setupDoneMsg); ok {
			break
		}
	}
	if got := m.deps.App.(*fakeApp).setup; !reflect.DeepEqual(got, []string{appruntime.Model08B}) {
		t.Fatal(got)
	}
}
func TestCancelDownload(t *testing.T) {
	m := testModel(64<<30, 1<<40)
	m, _ = key(m, "enter")
	cancelled := false
	m.cancel = func() { cancelled = true }
	m, _ = key(m, "esc")
	if !cancelled || m.screen != "setup" {
		t.Fatal("cancel failed")
	}
}
func TestProgressAndError(t *testing.T) {
	m := testModel(64<<30, 1<<40)
	p := appruntime.PrefetchProgress{Event: "progress", Item: "base", Done: 100, Total: 200, File: "weights"}
	updated, _ := m.Update(setupProgressMsg{p: p})
	m = updated.(Model)
	if m.rows["base"].Done != 100 {
		t.Fatal(m.rows)
	}
	m.screen = "error"
	m.err = "failure"
	for _, action := range []string{"[r]", "[m]", "[l]", "[q]"} {
		if !strings.Contains(m.View(), action) {
			t.Fatal(m.View())
		}
	}
	_, cmd := key(m, "q")
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("not quit")
	}
}
