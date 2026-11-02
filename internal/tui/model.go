// Package tui implements Gavel's setup and launcher screens.
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	appruntime "github.com/jdtobe/gavel/internal/runtime"
	"github.com/jdtobe/gavel/internal/status"
)

type App interface {
	Status() status.Status
	Setup(context.Context, []string, func(appruntime.PrefetchProgress)) error
	Start(context.Context) error
	Restart(context.Context) error
	SwitchModel(context.Context, string) error
	Models() []status.ModelState
	Logs() []string
	StartConsole(context.Context) (string, error)
	Stop(context.Context) error
	ConsoleURL() string
	OpenConsole() error
}
type Deps struct {
	App              App
	OpenBrowser      func(string) error
	MemoryBytes      func() (uint64, error)
	DiskFree         func(string) (uint64, error)
	DiskPath         string
	AutoStart        bool
	RuntimeInstalled bool
}
type Model struct {
	deps         Deps
	screen       string
	selected     map[string]bool
	focus        int
	memory, free uint64
	err, notice  string
	rows         map[string]appruntime.PrefetchProgress
	cancel       context.CancelFunc
	events       chan tea.Msg
	generation   int
	bar          progress.Model
	cached       map[string]bool
	previous     string
	viewport     viewport.Model
}
type setupProgressMsg struct {
	p          appruntime.PrefetchProgress
	generation int
}
type setupDoneMsg struct {
	err        error
	generation int
}
type serverReadyMsg struct{ generation int }
type serverErrMsg struct {
	err        error
	generation int
}
type logLineMsg string
type tickMsg time.Time

func New(deps Deps) Model {
	m := Model{deps: deps, screen: "setup", selected: map[string]bool{appruntime.Model08B: true}, rows: make(map[string]appruntime.PrefetchProgress), bar: progress.New(progress.WithDefaultGradient(), progress.WithWidth(26))}
	m.cached = make(map[string]bool)
	m.viewport = viewport.New(90, 20)
	for _, model := range deps.App.Models() {
		m.cached[model.Name] = model.Cached
	}
	var err error
	if deps.MemoryBytes != nil {
		m.memory, err = deps.MemoryBytes()
		if err != nil {
			m.err = err.Error()
			m.screen = "error"
		}
	}
	if deps.DiskFree != nil {
		m.free, err = deps.DiskFree(deps.DiskPath)
		if err != nil {
			m.err = err.Error()
			m.screen = "error"
		}
	}
	if deps.App.Status().Phase == status.PhaseReady {
		m.screen = "ready"
	}
	return m
}
func tick() tea.Cmd { return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }) }
func (m Model) Init() tea.Cmd {
	if m.deps.AutoStart {
		return tea.Batch(func() tea.Msg {
			if err := m.deps.App.Start(context.Background()); err != nil {
				return serverErrMsg{err: err}
			}
			return serverReadyMsg{}
		}, tick())
	}
	return tick()
}
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		s := m.deps.App.Status()
		if s.Phase == status.PhaseError && m.screen != "setup" {
			m.screen = "error"
			m.err = s.Error
		}
		return m, tick()
	case setupProgressMsg:
		if msg.generation != m.generation {
			return m, nil
		}
		m.rows[msg.p.Item] = msg.p
		return m, waitProgress(m.events)
	case setupDoneMsg:
		if msg.generation != m.generation {
			return m, nil
		}
		m.cancel = nil
		if msg.err != nil {
			m.err = msg.err.Error()
			m.screen = "error"
		} else {
			m.screen = "ready"
		}
		return m, nil
	case serverReadyMsg:
		if msg.generation == m.generation {
			m.screen = "ready"
			m.cancel = nil
		}
		return m, nil
	case serverErrMsg:
		if msg.generation == m.generation {
			m.screen = "error"
			m.err = msg.err.Error()
		}
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "esc" && (m.screen == "manage" || m.screen == "logs") {
			if m.screen == "logs" {
				m.screen = m.previous
			} else {
				m.screen = "ready"
			}
			return m, nil
		}
		switch msg.String() {
		case "q", "ctrl+c":
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		case "esc":
			if m.cancel != nil {
				m.cancel()
				m.cancel = nil
				m.generation++
			}
			m.screen = "setup"
			m.notice = "cancelled"
			return m, nil
		}
		if m.screen == "error" {
			switch msg.String() {
			case "m":
				m.screen = "manage"
				return m, nil
			case "r":
				return m.beginSetup()
			case "l":
				m.previous = m.screen
				m.screen = "logs"
				m.viewport.SetContent(strings.Join(m.deps.App.Logs(), "\n"))
				return m, nil
			}
			return m, nil
		}
		if m.screen == "ready" || m.screen == "manage" || m.screen == "logs" {
			return m.launcherKey(msg)
		}
		if m.screen == "progress" && msg.String() == "l" {
			m.previous = m.screen
			m.screen = "logs"
			m.viewport.SetContent(strings.Join(m.deps.App.Logs(), "\n"))
			return m, nil
		}
		if m.screen == "setup" {
			switch msg.String() {
			case "down", "tab", "up", "shift+tab":
				m.focus = 1 - m.focus
			case " ":
				model := []string{appruntime.Model08B, appruntime.Model4B}[m.focus]
				need, _ := appruntime.ModelMinMemory(model)
				if m.memory >= need {
					m.selected[model] = !m.selected[model]
				}
			case "enter":
				if m.canDownload() {
					return m.beginSetup()
				}
			}
		}
	}
	return m, nil
}
func (m Model) chosen() []string {
	var models []string
	for _, id := range []string{appruntime.Model08B, appruntime.Model4B} {
		if m.selected[id] {
			models = append(models, id)
		}
	}
	return models
}
func (m Model) canDownload() bool {
	models := m.chosen()
	required, err := appruntime.RequiredDisk(models)
	if err == nil && m.deps.RuntimeInstalled {
		required -= appruntime.RuntimeDiskBytes - (1 << 30)
	}
	for _, id := range models {
		if m.cached[id] {
			size, _ := appruntime.ModelSize(id)
			required -= size
		}
	}
	if err != nil || required > m.free {
		return false
	}
	for _, id := range models {
		need, _ := appruntime.ModelMinMemory(id)
		if m.memory < need {
			return false
		}
	}
	return true
}
func (m Model) beginSetup() (Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.generation++
	generation := m.generation
	m.screen = "progress"
	m.err = ""
	m.events = make(chan tea.Msg, 32)
	events := m.events
	models := m.chosen()
	app := m.deps.App
	worker := func() tea.Msg {
		defer close(events)
		err := app.Setup(ctx, models, func(p appruntime.PrefetchProgress) {
			select {
			case events <- setupProgressMsg{p, generation}:
			case <-ctx.Done():
			}
		})
		return setupDoneMsg{err, generation}
	}
	return m, tea.Batch(worker, waitProgress(events))
}
func waitProgress(events chan tea.Msg) tea.Cmd {
	if events == nil {
		return nil
	}
	return func() tea.Msg {
		msg, ok := <-events
		if !ok {
			return nil
		}
		return msg
	}
}
func (m Model) View() string {
	header := lipgloss.NewStyle().Bold(true).Render("Gavel — Kev playground") + "\n\n"
	switch m.screen {
	case "setup":
		var b strings.Builder
		b.WriteString(header + "First run: choose what to install\n\nRuntime\n  [x] Kev runtime (Python 3.13 + Kev + MLX)\n\nModels\n")
		for i, id := range []string{appruntime.Model08B, appruntime.Model4B} {
			mark := " "
			if m.selected[id] {
				mark = "x"
			}
			cursor := " "
			if m.focus == i {
				cursor = ">"
			}
			size, _ := appruntime.ModelSize(id)
			need, _ := appruntime.ModelMinMemory(id)
			reason := ""
			if m.memory < need {
				reason = fmt.Sprintf(" (requires %d GiB RAM)", need>>30)
			}
			fmt.Fprintf(&b, "%s [%s] %s ~%.2f GB%s\n", cursor, mark, id, float64(size)/1e9, reason)
		}
		fmt.Fprintf(&b, "\nRAM: %d GiB · Disk free: %.1f GB\n", m.memory>>30, float64(m.free)/1e9)
		if need, err := appruntime.RequiredDisk(m.chosen()); err == nil {
			fmt.Fprintf(&b, "Required: %.1f GB\n", float64(need)/1e9)
		}
		b.WriteString("\n[enter] Download & start   [q] Quit\n" + m.notice)
		if !m.canDownload() {
			b.WriteString("\nDownload disabled: select a supported model and ensure sufficient disk space.")
		}
		return b.String() + "\nConsole: " + m.deps.App.ConsoleURL() + "\n"
	case "progress":
		var b strings.Builder
		b.WriteString(header + "Downloading / Starting Kev… " + m.notice + "\n")
		for _, item := range []string{"runtime", "adapter", "base", "model"} {
			p := m.rows[item]
			fraction := 0.0
			if p.Total > 0 {
				fraction = float64(p.Done) / float64(p.Total)
			}
			if p.Event == "done" {
				fraction = 1
			}
			fmt.Fprintf(&b, "%s %s %s\n", item, m.bar.ViewAs(fraction), p.File)
		}
		return b.String() + "\n[esc] cancel · [q] Quit\nConsole: " + m.deps.App.ConsoleURL() + "\n"
	case "error":
		logs := m.deps.App.Logs()
		if len(logs) > 20 {
			logs = logs[len(logs)-20:]
		}
		return header + m.err + "\n" + strings.Join(logs, "\n") + "\n[r] Retry  [m] Models/setup  [l] Logs  [q] Quit\n" + m.notice
	default:
		return header + m.launcherView()
	}
}
