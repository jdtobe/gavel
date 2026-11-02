package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/jdtobe/gavel/internal/app"
	appruntime "github.com/jdtobe/gavel/internal/runtime"
	"github.com/jdtobe/gavel/internal/version"
)

func TestRunVersion(t *testing.T) {
	var out bytes.Buffer
	if err := runWith([]string{"--version"}, runOptions{out: &out, goos: "linux", goarch: "amd64"}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "gavel "+version.Version {
		t.Fatal(out.String())
	}
}
func TestUnsupportedPlatform(t *testing.T) {
	err := runWith(nil, runOptions{out: &bytes.Buffer{}, goos: "linux", goarch: "amd64"})
	if err == nil || !strings.Contains(err.Error(), "Apple Silicon") {
		t.Fatal(err)
	}
}
func TestRunHonorsGavelHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GAVEL_HOME", home)
	launched := false
	err := runWith(nil, runOptions{out: &bytes.Buffer{}, goos: "darwin", goarch: "arm64", launch: func(ctx context.Context, a *app.App, s *appruntime.Setup, auto bool) error {
		launched = true
		if s.Paths.Home != home {
			t.Fatal(s.Paths.Home)
		}
		if a.ConsoleURL() == "" {
			t.Fatal("console not started")
		}
		if auto {
			t.Fatal("first run autostarted")
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !launched {
		t.Fatal("TUI not launched")
	}
}
