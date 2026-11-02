package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	appruntime "github.com/jdtobe/gavel/internal/runtime"
)

func (m Model) launcherKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.screen == "logs" {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	if msg.String() == "l" {
		m.previous = m.screen
		m.screen = "logs"
		m.viewport.SetContent(strings.Join(m.deps.App.Logs(), "\n"))
		return m, nil
	}
	if m.screen == "ready" {
		switch msg.String() {
		case "o":
			return m, func() tea.Msg {
				var err error
				if m.deps.OpenBrowser != nil {
					err = m.deps.OpenBrowser(m.deps.App.ConsoleURL())
				} else {
					err = m.deps.App.OpenConsole()
				}
				if err != nil {
					return logLineMsg("Browser could not open; use " + m.deps.App.ConsoleURL())
				}
				return nil
			}
		case "m":
			m.screen = "manage"
			for _, model := range m.deps.App.Models() {
				m.cached[model.Name] = model.Cached
			}
			return m, nil
		case "r":
			return m.serverAction(m.deps.App.Restart)
		}
	}
	if m.screen == "manage" {
		switch msg.String() {
		case "down", "up", "tab":
			m.focus = 1 - m.focus
		case " ":
			id := []string{appruntime.Model08B, appruntime.Model4B}[m.focus]
			need, _ := appruntime.ModelMinMemory(id)
			if !m.cached[id] && m.memory >= need {
				m.selected[id] = !m.selected[id]
			}
		case "enter":
			id := []string{appruntime.Model08B, appruntime.Model4B}[m.focus]
			if m.cached[id] {
				return m.serverAction(func(ctx context.Context) error { return m.deps.App.SwitchModel(ctx, id) })
			}
			if m.canDownload() {
				return m.beginSetup()
			}
		}
	}
	return m, nil
}
func (m Model) serverAction(action func(context.Context) error) (Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.screen = "progress"
	m.notice = "restarting"
	m.generation++
	generation := m.generation
	return m, func() tea.Msg {
		err := action(ctx)
		if err != nil {
			return serverErrMsg{err, generation}
		}
		return serverReadyMsg{generation}
	}
}
func (m Model) launcherView() string {
	if m.screen == "logs" {
		return "Logs\n" + m.viewport.View() + "\n[esc] Back\n"
	}
	if m.screen == "manage" {
		text := "Manage models\n"
		for i, id := range []string{appruntime.Model08B, appruntime.Model4B} {
			cursor := " "
			if i == m.focus {
				cursor = ">"
			}
			size, _ := appruntime.ModelSize(id)
			text += fmt.Sprintf("%s %s %.2f GB · cached=%t · active=%t · selected=%t\n", cursor, id, float64(size)/1e9, m.cached[id], m.deps.App.Status().Model == id, m.selected[id])
		}
		return text + "\n[space] Select additional models · [enter] Switch/download · [esc] Back\n"
	}
	s := m.deps.App.Status()
	return fmt.Sprintf("%s · %s · %s · %s\n\n[o] Open console in browser\n[m] Manage models\n[r] Restart Kev server\n[l] Show logs\n[q] Quit\n\n%s\n%s\n", s.Model, s.Backend, s.SidecarURL, s.Phase, m.deps.App.ConsoleURL(), m.notice)
}
