package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/jdtobe/gavel/internal/status"
)

func readyModel() Model {
	m := testModel(64<<30, 1<<40)
	m.deps.App.(*fakeApp).state = status.Status{Phase: status.PhaseReady, Model: "m", Backend: "mlx"}
	m.screen = "ready"
	return m
}
func TestReadyOOpensBrowser(t *testing.T) {
	m := readyModel()
	var opened string
	m.deps.OpenBrowser = func(url string) error { opened = url; return nil }
	_, cmd := key(m, "o")
	if cmd == nil {
		t.Fatal("no browser command")
	}
	cmd()
	if opened == "" {
		t.Fatal("browser not opened")
	}
}

func TestBrowserOpenFailureShowsFallbackURL(t *testing.T) {
	m := readyModel()
	m.deps.OpenBrowser = func(string) error { return errors.New("open failed") }
	next, cmd := key(m, "o")
	updated, _ := next.Update(cmd())
	next = updated.(Model)
	if next.screen != "ready" {
		t.Fatal("browser failure left Ready")
	}
	if view := next.View(); !strings.Contains(view, "Browser could not open") || !strings.Contains(view, m.deps.App.ConsoleURL()) {
		t.Fatalf("missing visible fallback: %s", view)
	}
}
func TestReadyScreens(t *testing.T) {
	for _, k := range []string{"m", "l"} {
		m := readyModel()
		next, _ := key(m, k)
		if next.screen == "ready" {
			t.Fatal("did not change screen")
		}
		next, _ = key(next, "esc")
		if next.screen != "ready" {
			t.Fatal(next.screen)
		}
	}
	m := readyModel()
	next, cmd := key(m, "r")
	if cmd == nil || !strings.Contains(next.View(), "restarting") {
		t.Fatal(next.View())
	}
	updated, _ := next.Update(cmd())
	next = updated.(Model)
	if next.screen != "ready" {
		t.Fatal(next.screen)
	}
	m = readyModel()
	changed, _ := m.Update(serverErrMsg{err: errors.New("crashed")})
	if changed.(Model).screen != "error" {
		t.Fatal("missing error screen")
	}
}
func TestManageCachedSwitchWaits(t *testing.T) {
	m := readyModel()
	m.screen = "manage"
	m.cached = map[string]bool{"jaredpalmer/kev-0.8b@v1.0": true}
	next, cmd := key(m, "enter")
	if next.screen != "progress" || cmd == nil {
		t.Fatal("switch not started")
	}
	updated, _ := next.Update(cmd())
	if updated.(Model).screen != "ready" {
		t.Fatal("switch not finished")
	}
}
